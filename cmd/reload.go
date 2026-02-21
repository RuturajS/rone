/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package cmd

import (
	"fmt"

	"github.com/RuturajS/rone/config"
	"github.com/RuturajS/rone/internal/logger"
	"github.com/spf13/cobra"
)

var reloadCmd = &cobra.Command{
	Use:   "reload-config",
	Short: "Reload the configuration of the running daemon",
	RunE:  runReload,
}

func init() {
	rootCmd.AddCommand(reloadCmd)
}

func runReload(cmd *cobra.Command, args []string) error {
	// Validate the config file first
	_, err := config.Load(configPath())
	if err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	logger.Init("info", "text")

	// On Linux, send SIGHUP to the daemon. On Windows, we'd use a named pipe.
	// For now, we validate and instruct the user.
	pidPath := pidFilePath()
	fmt.Printf("config validated successfully\n")
	fmt.Printf("to reload the running daemon, send SIGHUP to the process listed in %s\n", pidPath)
	fmt.Printf("  linux:   kill -HUP $(cat %s)\n", pidPath)
	fmt.Printf("  windows: restart the daemon with 'rone stop && rone start'\n")
	return nil
}

