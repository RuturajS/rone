/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
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

	// Health status
	ollamaConnected bool
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
		cfg.Ollama.CloudEndpoint,
		cfg.Ollama.CloudModel,
		cfg.Ollama.CloudAPIKey,
		cfg.Ollama.Mode,
		cfg.Ollama.Timeout,
		cfg.Ollama.MaxRetries,
	)

	slog.Info("checking ollama connectivity...", "mode", cfg.Ollama.Mode, "model", ollamaClient.GetModel())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	tags, err := ollamaClient.Ping(ctx)
	cancel()

	var ollamaConnected bool
	if err != nil {
		slog.Warn("ollama: NOT reachable — AI responses may fail", "error", err)
		ollamaConnected = false
	} else {
		if cfg.Ollama.Mode == "local" {
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
		} else {
			slog.Info("ollama cloud: reachable", "model", cfg.Ollama.CloudModel)
		}
		ollamaConnected = true
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
		cfg:             cfg,
		db:              db,
		ollama:          ollamaClient,
		adapters:        adapterMap,
		scheduler:       sched,
		pendingCmds:     make(map[string]string),
		ollamaConnected: ollamaConnected,
	}, nil
}

// Run starts all components and blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	slog.Info("==========================================")
	slog.Info("  ROne daemon starting")
	slog.Info("  GitHub: github.com/RuturajS")
	slog.Info("  Authored by Ruturaj Sharbidre")
	slog.Info("==========================================")
	slog.Info("config summary",
		"adapters", len(d.adapters),
		"mode", d.ollama.GetMode(),
		"ollama_model", d.ollama.GetModel(),
		"tools_enabled", d.cfg.Tools.Enabled,
		"require_approval", d.cfg.Tools.RequireApproval,
	)

	handler := d.makeHandler(ctx)
	for _, a := range d.adapters {
		a.SetHandler(handler)
	}

	var wg sync.WaitGroup

	// Provide a small delay for adapters to connect before sending greeting
	go func() {
		time.Sleep(2 * time.Second)
		d.notifyOnline()
	}()

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

	// Shutdown phase
	d.notifyOffline()

	slog.Info("daemon shutting down")
	if err := d.db.Close(); err != nil {
		slog.Error("close database", "error", err)
	}

	slog.Info("daemon stopped")
	return nil
}

// notifyOnline sends a greeting message to active adapters.
func (d *Daemon) notifyOnline() {
	greeting := fmt.Sprintf("🌟 *%s, ROne is Online!* 🌟\n\nI'm now monitoring your messages and ready to assist. 🚀", getGreeting())
	funny := "\n\n_P.S. I promised not to overthink, but I’ve already indexed the meaning of life. (It’s 42, but with better formatting.)_ 😉"
	
	msg := greeting + funny
	
	if !d.ollamaConnected {
		msg += "\n\n⚠️ *Alert:* Local Ollama instance is NOT reachable! AI features will be limited until it's back online. Please check your local Ollama service. 🛠️"
	}

	d.broadcast(msg)
}

func getGreeting() string {
	hour := time.Now().Hour()
	switch {
	case hour >= 5 && hour < 12:
		return "Good Morning"
	case hour >= 12 && hour < 17:
		return "Good Afternoon"
	case hour >= 17 && hour < 21:
		return "Good Evening"
	default:
		return "Good Night"
	}
}

// notifyOffline sends a shutdown message.
func (d *Daemon) notifyOffline() {
	farewell := "😴 *ROne is going offline now.* 😴\n\nI'm shutting down for maintenance or a quick nap. See you soon! 👋\n\n_System going quiet... Over and out!_ 📡"
	d.broadcast(farewell)
}

// broadcast sends a message to the primary channel of each adapter.
func (d *Daemon) broadcast(msg string) {
	// Telegram
	if adapter, ok := d.adapters["telegram"]; ok && d.cfg.Telegram.ChatID != 0 {
		_ = adapter.Send(strconv.FormatInt(d.cfg.Telegram.ChatID, 10), msg)
	}

	// Discord
	if adapter, ok := d.adapters["discord"]; ok && d.cfg.Discord.ChannelID != "" {
		_ = adapter.Send(d.cfg.Discord.ChannelID, msg)
	}

	// Slack
	if adapter, ok := d.adapters["slack"]; ok && d.cfg.Slack.ChannelID != "" {
		_ = adapter.Send(d.cfg.Slack.ChannelID, msg)
	}
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

		// Handle internal control commands (e.g., "rone help", "rone toggle approval")
		if d.handleInternalCommand(adapter, msg) {
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

// handleInternalCommand processes commands specifically for controlling ROne.
func (d *Daemon) handleInternalCommand(adapter adapters.Adapter, msg adapters.IncomingMessage) bool {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return false
	}

	// Standardize input
	lcContent := strings.ToLower(content)
	parts := strings.Fields(lcContent)
	if len(parts) == 0 {
		return false
	}

	var cmd string
	var isExplicitRone bool

	// 1. Check for "rone ..." or "/rone ..." prefix
	if strings.HasPrefix(parts[0], "rone") || strings.HasPrefix(parts[0], "/rone") {
		isExplicitRone = true
		if len(parts) == 1 {
			// Just typing "rone" or "/rone" -> help
			cmd = "help"
		} else {
			cmd = parts[1]
		}
	} else if strings.HasPrefix(parts[0], "/") {
		// 2. Check for direct "/cmd" like "/status" or "/ping"
		cmd = strings.TrimPrefix(parts[0], "/")
		// Handle Telegram "/cmd@botname" suffix
		if atIdx := strings.Index(cmd, "@"); atIdx != -1 {
			cmd = cmd[:atIdx]
		}
	} else {
		// Not a command
		return false
	}

	// Switch on specific commands
	switch cmd {
	case "help", "rone":
		help := "🤖 *ROne Control Center*\n\n" +
			"Available options:\n" +
			"• `rone status` (or `/status`) - Show system & bot status\n" +
			"• `rone mode <local|cloud>` (or `/mode`) - Switch AI provider\n" +
			"• `rone approval <on|off>` (or `/approval`) - Toggle command safety\n" +
			"• `rone tools <on|off>` (or `/tools`) - Toggle terminal access\n" +
			"• `rone model <name>` (or `/model`) - Change LLM model\n" +
			"• `rone ping` (or `/ping`) - Check if I'm awake\n\n" +
			"Commands work with or without the `rone` prefix!"
		_ = adapter.Send(msg.ChannelID, help)
		return true

	case "ping":
		responses := []string{
			"Pong! 🏓 (I was actually mapping the stars, but for you, I'll pause.)",
			"I'm awake! 🤖 (Mostly... just finishing my digital coffee.)",
			"Still here! 🌐 My circuits are buzzing with excitement.",
			"Ping received. 📡 I'm 100% functional and 110% ready to be helpful.",
		}
		idx := time.Now().UnixNano() % int64(len(responses))
		_ = adapter.Send(msg.ChannelID, responses[idx])
		return true

	case "mode":
		if len(parts) < (map[bool]int{true: 3, false: 2}[isExplicitRone]) {
			_ = adapter.Send(msg.ChannelID, fmt.Sprintf("🧠 *Current Mode:* `%s`", d.ollama.GetMode()))
			return true
		}
		newMode := parts[len(parts)-1]
		if newMode != "local" && newMode != "cloud" {
			_ = adapter.Send(msg.ChannelID, "❓ Unknown mode. Use `local` or `cloud`.")
			return true
		}
		if err := d.ollama.SwitchMode(newMode); err != nil {
			_ = adapter.Send(msg.ChannelID, "❌ Failed to switch mode: "+err.Error())
			return true
		}
		_ = adapter.Send(msg.ChannelID, fmt.Sprintf("✅ *AI Mode Switched to:* `%s`", newMode))
		return true

	case "status":
		status := fmt.Sprintf("📊 *Status*\n\n• *Mode:* %s\n• *Model:* %s\n• *Approval Mode:* %v\n• *Tools Enabled:* %v\n• *Adapters:* %d active\n• *Models in Config:* %d", 
			d.ollama.GetMode(), d.ollama.GetModel(), d.cfg.Tools.RequireApproval, d.cfg.Tools.Enabled, len(d.adapters), len(d.cfg.Ollama.Models))
		_ = adapter.Send(msg.ChannelID, status)
		return true
		
	case "approval":
		if len(parts) < (map[bool]int{true: 3, false: 2}[isExplicitRone]) {
			_ = adapter.Send(msg.ChannelID, "❓ Specify `on` or `off`. Example: `/approval off`")
			return true
		}
		mode := parts[len(parts)-1]
		if mode == "on" {
			d.cfg.Tools.RequireApproval = true
			_ = adapter.Send(msg.ChannelID, "✅ *Approval Mode:* ON. I will now ask for permission before running terminal commands.")
		} else if mode == "off" {
			d.cfg.Tools.RequireApproval = false
			_ = adapter.Send(msg.ChannelID, "⚠️ *Approval Mode:* OFF. I will now execute terminal commands automatically.")
		} else {
			_ = adapter.Send(msg.ChannelID, "❓ Unknown mode. Use `on` or `off`.")
		}
		return true

	case "tools":
		if len(parts) < (map[bool]int{true: 3, false: 2}[isExplicitRone]) {
			_ = adapter.Send(msg.ChannelID, "❓ Specify `on` or `off`. Example: `/tools off`")
			return true
		}
		mode := parts[len(parts)-1]
		if mode == "on" {
			d.cfg.Tools.Enabled = true
			_ = adapter.Send(msg.ChannelID, "✅ *Tools:* Enabled. I can now run terminal commands.")
		} else if mode == "off" {
			d.cfg.Tools.Enabled = false
			_ = adapter.Send(msg.ChannelID, "🚫 *Tools:* Disabled. I will only engage in conversation.")
		}
		return true

	case "model":
		if len(parts) < (map[bool]int{true: 3, false: 2}[isExplicitRone]) {
			msgText := fmt.Sprintf("🧠 *Current Mode:* `%s`\n🧠 *Current LLM:* `%s`", d.ollama.GetMode(), d.ollama.GetModel())
			if len(d.cfg.Ollama.Models) > 0 {
				msgText += "\n\n*Available from config:*\n"
				for _, m := range d.cfg.Ollama.Models {
					msgText += fmt.Sprintf("• `%s`\n", m)
				}
				msgText += "\nUse `rone model <name>` to switch."
			}
			_ = adapter.Send(msg.ChannelID, msgText)
			return true
		}
		newModel := parts[len(parts)-1]
		if d.ollama.GetMode() == "local" {
			found := false
			for _, m := range d.cfg.Ollama.Models {
				if m == newModel {
					found = true
					break
				}
			}
			if !found {
				_ = adapter.Send(msg.ChannelID, fmt.Sprintf("❌ Model `%s` is NOT in the allowed list for local mode.", newModel))
				return true
			}
		}
		d.ollama.SetModel(newModel)
		_ = adapter.Send(msg.ChannelID, fmt.Sprintf("✅ *Model switched to:* `%s`", newModel))
		return true

	default:
		// If it started with "rone " or "/rone ", and we don't know the subcommand, show error.
		if isExplicitRone {
			_ = adapter.Send(msg.ChannelID, "❓ Unknown command. Type `rone` for help.")
			return true
		}
		// If it's just a random "/something", don't intercept it (let AI handle it)
		return false
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

	return false
}

// handleConversation processes a conversational message.
func (d *Daemon) handleConversation(ctx context.Context, adapter adapters.Adapter, msg adapters.IncomingMessage, msgID int64) {
	adapter.SendTyping(msg.ChannelID)

	var response string
	var err error

	if d.cfg.Tools.Enabled {
		slog.Info("generating response (tools enabled)...", "mode", d.ollama.GetMode(), "model", d.ollama.GetModel())
		response, err = d.ollama.GenerateWithTools(ctx, msg.Content)
	} else {
		slog.Info("generating response (tools disabled)...", "mode", d.ollama.GetMode(), "model", d.ollama.GetModel())
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
