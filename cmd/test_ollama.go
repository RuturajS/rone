package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/RuturajS/rone/config"
	"github.com/RuturajS/rone/ollama"
	"github.com/spf13/cobra"
)

var testOllamaCmd = &cobra.Command{
	Use:   "test-ollama",
	Short: "Test connectivity to the Ollama endpoint",
	RunE:  runTestOllama,
}

func init() {
	rootCmd.AddCommand(testOllamaCmd)
}

func runTestOllama(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client := ollama.NewClient(cfg.Ollama.Endpoint, cfg.Ollama.Model, cfg.Ollama.Timeout, cfg.Ollama.MaxRetries)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("pinging ollama at %s ...\n", cfg.Ollama.Endpoint)

	tags, err := client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}

	fmt.Printf("✅ ollama is reachable\n")
	fmt.Printf("available models (%d):\n", len(tags.Models))
	for _, m := range tags.Models {
		fmt.Printf("  - %s (%.1f MB)\n", m.Name, float64(m.Size)/(1024*1024))
	}

	// Check if configured model is available
	found := false
	for _, m := range tags.Models {
		if m.Name == cfg.Ollama.Model {
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("⚠ configured model '%s' not found in available models\n", cfg.Ollama.Model)
	} else {
		fmt.Printf("✅ configured model '%s' is available\n", cfg.Ollama.Model)
	}

	return nil
}
