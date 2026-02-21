package config

import "time"

// Config is the root configuration struct loaded from YAML.
type Config struct {
	Telegram  TelegramConfig  `yaml:"telegram"`
	Discord   DiscordConfig   `yaml:"discord"`
	Slack     SlackConfig     `yaml:"slack"`
	Ollama    OllamaConfig    `yaml:"ollama"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Log       LogConfig       `yaml:"log"`
}

type TelegramConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
	ChatID  int64  `yaml:"chat_id"`
	Debug   bool   `yaml:"debug"` // log full incoming messages
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
