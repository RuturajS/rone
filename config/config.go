package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultConfig returns a Config with sane defaults.
func DefaultConfig() *Config {
	return &Config{
		Telegram: TelegramConfig{Enabled: false},
		Discord:  DiscordConfig{Enabled: false},
		Slack:    SlackConfig{Enabled: false},
		Ollama: OllamaConfig{
			Endpoint:   "http://localhost:11434",
			Model:      "llama3.2",
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		},
		Scheduler: SchedulerConfig{
			Interval: 10 * time.Second,
		},
		RateLimit: RateLimitConfig{
			MessagesPerMinute: 20,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		Tools: ToolsConfig{
			Enabled:         false,
			RequireApproval: true, // Safety first
			Timeout:         30 * time.Second,
		},
	}
}

// Load reads a YAML config file and applies environment variable overrides.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyEnvOverrides(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// applyEnvOverrides overrides config values with environment variables if set.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("RONE_TELEGRAM_TOKEN"); v != "" {
		cfg.Telegram.Token = v
	}
	if v := os.Getenv("RONE_TELEGRAM_CHAT_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Telegram.ChatID = id
		}
	}
	if v := os.Getenv("RONE_DISCORD_TOKEN"); v != "" {
		cfg.Discord.Token = v
	}
	if v := os.Getenv("RONE_SLACK_TOKEN"); v != "" {
		cfg.Slack.Token = v
	}
	if v := os.Getenv("RONE_SLACK_APP_TOKEN"); v != "" {
		cfg.Slack.AppToken = v
	}
	if v := os.Getenv("RONE_OLLAMA_MODEL"); v != "" {
		cfg.Ollama.Model = v
	}
	if v := os.Getenv("RONE_OLLAMA_ENDPOINT"); v != "" {
		cfg.Ollama.Endpoint = v
	}
	if v := os.Getenv("RONE_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("RONE_SCHEDULER_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Scheduler.Interval = d
		}
	}
	if v := os.Getenv("RONE_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.MessagesPerMinute = n
		}
	}
	if v := os.Getenv("RONE_TOOLS_ENABLED"); v != "" {
		cfg.Tools.Enabled = v == "true"
	}
	if v := os.Getenv("RONE_TOOLS_REQUIRE_APPROVAL"); v != "" {
		cfg.Tools.RequireApproval = v == "true"
	}
	if v := os.Getenv("RONE_TOOLS_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Tools.Timeout = d
		}
	}
}

// validate checks required fields.
func validate(cfg *Config) error {
	if cfg.Telegram.Enabled && cfg.Telegram.Token == "" {
		return fmt.Errorf("telegram enabled but token is empty (set RONE_TELEGRAM_TOKEN)")
	}
	if cfg.Telegram.Enabled && cfg.Telegram.ChatID == 0 {
		return fmt.Errorf("telegram enabled but chat_id is empty (set RONE_TELEGRAM_CHAT_ID)")
	}
	if cfg.Discord.Enabled && cfg.Discord.Token == "" {
		return fmt.Errorf("discord enabled but token is empty (set RONE_DISCORD_TOKEN)")
	}
	if cfg.Slack.Enabled && cfg.Slack.Token == "" {
		return fmt.Errorf("slack enabled but token is empty (set RONE_SLACK_TOKEN)")
	}
	if cfg.Slack.Enabled && cfg.Slack.AppToken == "" {
		return fmt.Errorf("slack enabled but app_token is empty (set RONE_SLACK_APP_TOKEN)")
	}
	if cfg.Ollama.Endpoint == "" {
		return fmt.Errorf("ollama endpoint is required")
	}
	if cfg.Ollama.Model == "" {
		return fmt.Errorf("ollama model is required")
	}
	if cfg.Scheduler.Interval < 1*time.Second {
		return fmt.Errorf("scheduler interval must be >= 1s")
	}
	return nil
}
