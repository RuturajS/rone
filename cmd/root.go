/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	logLevel   string
	foreground bool
	appVersion string = "dev"
)

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:   "rone",
	Short: "ROne — ultra-lightweight CLI daemon for multi-channel AI messaging",
	Long: `ROne is a single-binary, cross-platform daemon that listens to
Telegram, Discord, and Slack messages, classifies intent via a local
Ollama instance, and schedules tasks using an in-memory SQLite database.`,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "config.yaml", "path to config file")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "override log level (debug|info|warn|error)")
	rootCmd.PersistentFlags().BoolVar(&foreground, "foreground", false, "run in foreground (don't daemonize)")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// SetVersion sets the application version from ldflags.
func SetVersion(v string) {
	appVersion = v
	rootCmd.Version = v
}

// configPath returns the resolved config file path.
func configPath() string {
	if cfgFile != "" {
		return cfgFile
	}
	return "config.yaml"
}

// exitOnError prints an error and exits.
func exitOnError(msg string, err error) {
	fmt.Fprintf(os.Stderr, "error: %s: %v\n", msg, err)
	os.Exit(1)
}

