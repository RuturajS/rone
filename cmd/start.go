/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/RuturajS/rone/config"
	"github.com/RuturajS/rone/daemon"
	"github.com/RuturajS/rone/internal/logger"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the ROne daemon",
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	cfgPath := configPath()
	slog.Info("loading config", "path", cfgPath)

	// Always reload config from disk on every start/build
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Override log level from flag
	if logLevel != "" {
		cfg.Log.Level = logLevel
	}

	// Initialize logger
	logger.Init(cfg.Log.Level, cfg.Log.Format)

	slog.Info("==========================================")
	slog.Info("  ROne v" + appVersion)
	slog.Info("  Author: Ruturaj Sharbidre")
	slog.Info("  GitHub: github.com/RuturajS")
	slog.Info("==========================================")
	slog.Info("config loaded",
		"path", cfgPath,
		"log_level", cfg.Log.Level,
		"telegram_enabled", cfg.Telegram.Enabled,
		"discord_enabled", cfg.Discord.Enabled,
		"slack_enabled", cfg.Slack.Enabled,
		"ollama_endpoint", cfg.Ollama.Endpoint,
		"ollama_model", cfg.Ollama.Model,
		"scheduler_interval", cfg.Scheduler.Interval,
	)

	// Write PID file
	pidPath := pidFilePath()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		slog.Warn("failed to write pid file", "path", pidPath, "error", err)
	} else {
		slog.Debug("pid file written", "path", pidPath)
	}
	defer os.Remove(pidPath)

	// Create daemon (includes health checks)
	d, err := daemon.New(cfg)
	if err != nil {
		return fmt.Errorf("init daemon: %w", err)
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for shutdown signals in a goroutine
	go daemon.WaitForShutdown(cancel)

	// Run daemon (blocks until context cancelled)
	return d.Run(ctx)
}

// pidFilePath returns the platform-appropriate PID file path.
func pidFilePath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "rone.pid")
	}
	return "/tmp/rone.pid"
}

