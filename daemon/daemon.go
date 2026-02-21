package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/RuturajS/rone/adapters"
	"github.com/RuturajS/rone/config"
	"github.com/RuturajS/rone/database"
	"github.com/RuturajS/rone/executor"
	"github.com/RuturajS/rone/ollama"
	"github.com/RuturajS/rone/scheduler"
)

// Daemon is the main process orchestrator.
type Daemon struct {
	cfg       *config.Config
	db        *database.DB
	ollama    *ollama.Client
	adapters  map[string]adapters.Adapter
	scheduler *scheduler.Scheduler

	// State for pending commands awaiting user approval
	// Key: platform:channelID, Value: command to execute
	pendingCmds map[string]string
	mu          sync.RWMutex
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
		cfg:         cfg,
		db:          db,
		ollama:      ollamaClient,
		adapters:    adapterMap,
		scheduler:   sched,
		pendingCmds: make(map[string]string),
	}, nil
}

// Run starts all components and blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	slog.Info("==========================================")
	slog.Info("  ROne daemon starting")
	slog.Info("  Author: Ruturaj Sharbidre")
	slog.Info("  GitHub: github.com/RuturajS")
	slog.Info("==========================================")
	slog.Info("config summary",
		"adapters", len(d.adapters),
		"ollama_model", d.cfg.Ollama.Model,
		"ollama_endpoint", d.cfg.Ollama.Endpoint,
		"scheduler_interval", d.cfg.Scheduler.Interval,
		"log_level", d.cfg.Log.Level,
		"tools_enabled", d.cfg.Tools.Enabled,
		"require_approval", d.cfg.Tools.RequireApproval,
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
func (d *Daemon) makeHandler(ctx context.Context) adapters.MessageHandler {
	return func(msg adapters.IncomingMessage) {
		slog.Info(">> message received",
			"platform", msg.Platform,
			"sender", msg.Sender,
			"channel", msg.ChannelID,
			"content", msg.Content,
		)

		if msg.Content == "" {
			return
		}

		adapter, ok := d.adapters[msg.Platform]
		if !ok {
			slog.Error("no adapter for platform", "platform", msg.Platform)
			return
		}

		// Handle pending approval if exists
		if d.handlePendingApproval(ctx, adapter, msg) {
			return
		}

		adapter.SendTyping(msg.ChannelID)

		channelDBID, err := d.db.UpsertChannel(msg.Platform, msg.ChannelID, "")
		if err != nil {
			slog.Error("upsert channel failed", "error", err)
			return
		}

		// Classify
		intent, err := d.ollama.Classify(ctx, msg.Content)
		if err != nil {
			slog.Warn("classify failed, defaulting to conversation", "error", err)
			intent = "conversation"
		}
		slog.Info("intent classified", "intent", intent)

		// Store message
		msgID, err := d.db.InsertMessage(channelDBID, msg.Sender, msg.Content, intent)
		if err != nil {
			slog.Error("store message failed", "error", err)
			return
		}

		switch intent {
		case "task":
			now := time.Now().UTC().Format(time.RFC3339)
			taskID, err := d.db.InsertTask(&msgID, channelDBID, msg.Content, "once", nil, now, nil)
			if err != nil {
				slog.Error("insert task failed", "error", err)
				return
			}
			slog.Info("task created", "task_id", taskID)

			ack := fmt.Sprintf("Task #%d recorded. Will be executed shortly.", taskID)
			_ = adapter.Send(msg.ChannelID, ack)
			_ = d.db.MarkResponded(msgID)

		default:
			// Conversation path — with tool support
			d.handleConversation(ctx, adapter, msg, msgID)
		}
	}
}

// handlePendingApproval checks if a message is an approval for a pending command.
func (d *Daemon) handlePendingApproval(ctx context.Context, adapter adapters.Adapter, msg adapters.IncomingMessage) bool {
	key := fmt.Sprintf("%s:%s", msg.Platform, msg.ChannelID)

	d.mu.RLock()
	cmd, exists := d.pendingCmds[key]
	d.mu.RUnlock()

	if !exists {
		return false
	}

	content := strings.ToLower(strings.TrimSpace(msg.Content))
	if content == "yes" || content == "y" || strings.Contains(content, "proceed") {
		// Approved! Clear the state and execute.
		d.mu.Lock()
		delete(d.pendingCmds, key)
		d.mu.Unlock()

		slog.Info("command approved by user", "platform", msg.Platform, "channel", msg.ChannelID, "cmd", cmd)
		
		// Map the message ID back if we had it? 
		// For simplicity, we just execute and reply here.
		// We'll use a dummy ID or just not link it back to the 'yes' message for now.
		d.executeAndReply(ctx, adapter, msg, 0, cmd)
		return true
	} else if content == "no" || content == "n" || strings.Contains(content, "cancel") {
		// Cancelled.
		d.mu.Lock()
		delete(d.pendingCmds, key)
		d.mu.Unlock()

		slog.Info("command cancelled by user", "platform", msg.Platform, "channel", msg.ChannelID)
		_ = adapter.Send(msg.ChannelID, "❌ Command execution cancelled.")
		return true
	}

	// It wasn't a clear yes/no, so we stay in pending state and return false (process normally)
	// Actually, we should probably ignore other messages while waiting? 
	// Let's settle for allowing the user to ignore it by talking about something else.
	return false
}

// handleConversation processes a conversational message.
func (d *Daemon) handleConversation(ctx context.Context, adapter adapters.Adapter, msg adapters.IncomingMessage, msgID int64) {
	adapter.SendTyping(msg.ChannelID)

	var response string
	var err error

	if d.cfg.Tools.Enabled {
		slog.Info("generating response (tools enabled)...", "model", d.cfg.Ollama.Model)
		response, err = d.ollama.GenerateWithTools(ctx, msg.Content)
	} else {
		slog.Info("generating response (tools disabled)...", "model", d.cfg.Ollama.Model)
		response, err = d.ollama.Generate(ctx, msg.Content)
	}

	if err != nil {
		slog.Error("generate response failed", "error", err)
		_ = adapter.Send(msg.ChannelID, ollama.FailSafeMessage())
		_ = d.db.MarkResponded(msgID)
		return
	}

	// Check if the LLM wants to execute a command
	if d.cfg.Tools.Enabled {
		if cmd, isCmd := ollama.ParseToolResponse(response); isCmd {
			if d.cfg.Tools.RequireApproval {
				// Store in pending state and ask user
				key := fmt.Sprintf("%s:%s", msg.Platform, msg.ChannelID)
				d.mu.Lock()
				d.pendingCmds[key] = cmd
				d.mu.Unlock()

				slog.Info("tool: pending approval required", "command", cmd)
				
				// Show the user the plan
				planMsg := fmt.Sprintf("⚠️ *Plan:* I intend to execute the following command on this system:\n\n`%s`\n\nReply with *'yes'* to proceed or *'no'* to cancel.", cmd)
				if err := adapter.Send(msg.ChannelID, planMsg); err != nil {
					slog.Error("send approval request failed", "error", err)
				}
				_ = d.db.MarkResponded(msgID)
				return
			} else {
				// Show plan but execute immediately
				planMsg := fmt.Sprintf("🛠 *Executing:* `%s`...", cmd)
				_ = adapter.Send(msg.ChannelID, planMsg)
				d.executeAndReply(ctx, adapter, msg, msgID, cmd)
				return
			}
		}
	}

	// No command — send the LLM text response directly
	slog.Info("response generated (text)", "length", len(response))
	if err := adapter.Send(msg.ChannelID, response); err != nil {
		slog.Error("send response failed", "error", err)
	} else {
		slog.Info("<< reply sent", "platform", msg.Platform)
	}
	_ = d.db.MarkResponded(msgID)
}

// executeAndReply runs a command, sends output to LLM for summary, replies to user.
func (d *Daemon) executeAndReply(ctx context.Context, adapter adapters.Adapter, msg adapters.IncomingMessage, msgID int64, command string) {
	slog.Info("tool: executing command", "command", command)

	adapter.SendTyping(msg.ChannelID)

	timeout := d.cfg.Tools.Timeout
	if timeout == 0 {
		timeout = executor.DefaultTimeout
	}
	result := executor.Run(ctx, command, timeout)

	slog.Info("tool: command completed",
		"command", command,
		"exit_code", result.ExitCode,
		"output_bytes", len(result.Output),
		"duration", result.Duration,
	)

	adapter.SendTyping(msg.ChannelID)

	cmdOutput := result.Output
	if result.Error != "" && result.ExitCode != 0 {
		cmdOutput += "\nError: " + result.Error
	}

	summary, err := d.ollama.Summarize(ctx, msg.Content, command, cmdOutput)
	if err != nil {
		slog.Warn("tool: summarize failed, sending raw output", "error", err)
		summary = executor.FormatResult(result)
	}

	slog.Info("tool: summary generated", "length", len(summary))

	if err := adapter.Send(msg.ChannelID, summary); err != nil {
		slog.Error("send tool response failed", "error", err)
	} else {
		slog.Info("<< tool reply sent", "platform", msg.Platform, "command", command)
	}
	if msgID != 0 {
		_ = d.db.MarkResponded(msgID)
	}
}
