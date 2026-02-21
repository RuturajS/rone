package config

import "time"

// Config is the root configuration struct loaded from YAML.
type Config struct {
	Telegram  TelegramConfig  `yaml:"telegram"`
	Discord   DiscordConfig   `yaml:"discord"`
	Slack     SlackConfig     `yaml:"slack"`
	Ollama    OllamaConfig    `yaml:"ollama"`
	Tools     ToolsConfig     `yaml:"tools"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Log       LogConfig       `yaml:"log"`
}

type TelegramConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
	ChatID  int64  `yaml:"chat_id"`
	Debug   bool   `yaml:"debug"`
}

type DiscordConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
}

type SlackConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Token    string `yaml:"token"`
	AppToken string `yaml:"app_token"`
}

type OllamaConfig struct {
	Endpoint   string        `yaml:"endpoint"`
	Model      string        `yaml:"model"`
	Timeout    time.Duration `yaml:"timeout"`
	MaxRetries int           `yaml:"max_retries"`
}

type ToolsConfig struct {
	Enabled         bool          `yaml:"enabled"`          // allow LLM to execute system commands
	RequireApproval bool          `yaml:"require_approval"` // ask user for permission before running CMD
	Timeout         time.Duration `yaml:"timeout"`          // max execution time per command
}

type SchedulerConfig struct {
	Interval time.Duration `yaml:"interval"`
}

type RateLimitConfig struct {
	MessagesPerMinute int `yaml:"messages_per_minute"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}
