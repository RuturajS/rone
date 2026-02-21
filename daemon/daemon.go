package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/RuturajS/rone/adapters"
	"github.com/RuturajS/rone/config"
	"github.com/RuturajS/rone/database"
	"github.com/RuturajS/rone/ollama"
	"github.com/RuturajS/rone/scheduler"
)

// Daemon is the main process orchestrator — wires all components together.
type Daemon struct {
	cfg       *config.Config
	db        *database.DB
	ollama    *ollama.Client
	adapters  map[string]adapters.Adapter
	scheduler *scheduler.Scheduler
}

// New creates a new Daemon from the given config.
func New(cfg *config.Config) (*Daemon, error) {
	slog.Info("initializing sqlite in-memory database...")
	db, err := database.Open()
	if err != nil {
		return nil, err
	}
	slog.Info("database: OK")

	ollamaClient := ollama.NewClient(
		cfg.Ollama.Endpoint,
		cfg.Ollama.Model,
		cfg.Ollama.Timeout,
		cfg.Ollama.MaxRetries,
	)

	slog.Info("checking ollama connectivity...", "endpoint", cfg.Ollama.Endpoint, "model", cfg.Ollama.Model)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	tags, err := ollamaClient.Ping(ctx)
	cancel()
	if err != nil {
		slog.Warn("ollama: NOT reachable — AI responses will fail", "error", err)
	} else {
		modelFound := false
		for _, m := range tags.Models {
			if m.Name == cfg.Ollama.Model {
				modelFound = true
				break
			}
		}
		if modelFound {
			slog.Info("ollama: OK", "model", cfg.Ollama.Model, "available_models", len(tags.Models))
		} else {
			slog.Warn("ollama: reachable but configured model NOT found",
				"model", cfg.Ollama.Model,
				"available_models", len(tags.Models),
			)
		}
	}

	adapterMap := make(map[string]adapters.Adapter)
	if cfg.Telegram.Enabled {
		slog.Info("telegram: enabled", "chat_id", cfg.Telegram.ChatID, "debug", cfg.Telegram.Debug)
		adapterMap["telegram"] = adapters.NewTelegramAdapter(cfg.Telegram.Token, cfg.Telegram.ChatID, cfg.Telegram.Debug)
	} else {
		slog.Info("telegram: disabled")
	}
	if cfg.Discord.Enabled {
		slog.Info("discord: enabled")
		adapterMap["discord"] = adapters.NewDiscordAdapter(cfg.Discord.Token)
	} else {
		slog.Info("discord: disabled")
	}
	if cfg.Slack.Enabled {
		slog.Info("slack: enabled")
		adapterMap["slack"] = adapters.NewSlackAdapter(cfg.Slack.Token, cfg.Slack.AppToken)
	} else {
		slog.Info("slack: disabled")
	}

	if len(adapterMap) == 0 {
		slog.Warn("no adapters enabled — daemon will run but won't receive any messages")
	}

	sched := scheduler.New(cfg.Scheduler.Interval, db, adapterMap, ollamaClient)

	return &Daemon{
		cfg:       cfg,
		db:        db,
		ollama:    ollamaClient,
		adapters:  adapterMap,
		scheduler: sched,
	}, nil
}

// Run starts all components and blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	slog.Info("==========================================")
	slog.Info("  ROne daemon starting")
	slog.Info("==========================================")
	slog.Info("config summary",
		"adapters", len(d.adapters),
		"ollama_model", d.cfg.Ollama.Model,
		"ollama_endpoint", d.cfg.Ollama.Endpoint,
		"scheduler_interval", d.cfg.Scheduler.Interval,
		"log_level", d.cfg.Log.Level,
	)

	handler := d.makeHandler(ctx)
	for _, a := range d.adapters {
		a.SetHandler(handler)
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		d.scheduler.Run(ctx)
	}()

	for name, a := range d.adapters {
		wg.Add(1)
		go func(n string, adapter adapters.Adapter) {
			defer wg.Done()
			slog.Info("starting adapter...", "adapter", n)
			if err := adapter.Start(ctx); err != nil {
				slog.Error("adapter failed", "adapter", n, "error", err)
			}
		}(name, a)
	}

	slog.Info("daemon running — press Ctrl+C to stop")

	wg.Wait()

	slog.Info("daemon shutting down")
	if err := d.db.Close(); err != nil {
		slog.Error("close database", "error", err)
	}

	slog.Info("daemon stopped")
	return nil
}

// makeHandler returns the message handler callback.
// Almost all messages are treated as conversation (direct LLM chat).
// Only explicit "remind me" / "schedule X" type messages create tasks.
func (d *Daemon) makeHandler(ctx context.Context) adapters.MessageHandler {
	return func(msg adapters.IncomingMessage) {
		slog.Info(">> message received",
			"platform", msg.Platform,
			"sender", msg.Sender,
			"channel", msg.ChannelID,
			"content", msg.Content,
		)

		if msg.Content == "" {
			slog.Debug("empty message, skipping")
			return
		}

		adapter, ok := d.adapters[msg.Platform]
		if !ok {
			slog.Error("no adapter for platform", "platform", msg.Platform)
			return
		}

		// Send typing indicator immediately
		adapter.SendTyping(msg.ChannelID)

		// Register channel
		channelDBID, err := d.db.UpsertChannel(msg.Platform, msg.ChannelID, "")
		if err != nil {
			slog.Error("upsert channel failed", "error", err)
			return
		}

		// Classify intent — biased heavily toward conversation
		slog.Debug("classifying message via ollama...")
		intent, err := d.ollama.Classify(ctx, msg.Content)
		if err != nil {
			slog.Warn("classify failed, defaulting to conversation", "error", err)
			intent = "conversation"
		}
		slog.Info("intent classified", "intent", intent, "content", msg.Content)

		// Store message in DB
		msgID, err := d.db.InsertMessage(channelDBID, msg.Sender, msg.Content, intent)
		if err != nil {
			slog.Error("store message failed", "error", err)
			return
		}

		switch intent {
		case "task":
			// Only for explicit scheduling requests.
			// Create the task ONCE and acknowledge. Scheduler picks it up later.
			now := time.Now().UTC().Format(time.RFC3339)
			taskID, err := d.db.InsertTask(&msgID, channelDBID, msg.Content, "once", nil, now, nil)
			if err != nil {
				slog.Error("insert task failed", "error", err)
				return
			}
			slog.Info("task created", "task_id", taskID)

			ack := fmt.Sprintf("Task #%d recorded. Will be executed shortly.", taskID)
			if err := adapter.Send(msg.ChannelID, ack); err != nil {
				slog.Error("send task ack failed", "error", err)
			}
			_ = d.db.MarkResponded(msgID)
			// DONE — no further processing. Scheduler handles execution.

		default:
			// Conversation — send to Ollama and reply directly.
			adapter.SendTyping(msg.ChannelID)

			slog.Info("generating response via ollama...", "model", d.cfg.Ollama.Model)
			response, err := d.ollama.Generate(ctx, msg.Content)
			if err != nil {
				slog.Error("generate response failed", "error", err)
				response = ollama.FailSafeMessage()
			}
			slog.Info("response generated", "length", len(response))

			if err := adapter.Send(msg.ChannelID, response); err != nil {
				slog.Error("send response failed", "error", err)
			} else {
				slog.Info("<< reply sent", "platform", msg.Platform, "channel", msg.ChannelID)
			}
			_ = d.db.MarkResponded(msgID)
		}
	}
}
