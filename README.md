# ROne

Cross-platform, single-binary CLI daemon for multi-channel messaging with local LLM inference and task scheduling.

## Overview

ROne is a lightweight background process that connects to Telegram, Discord, and Slack channels, classifies incoming message intent via a local Ollama instance, and manages scheduled tasks through an in-memory SQLite database. It requires no cloud backend, no external database server, and no REST API — a single static binary handles everything.

## Features

- **Multi-channel adapters** — Telegram (long polling), Discord (WebSocket gateway), Slack (Socket Mode)
- **Local LLM inference** — Intent classification and conversational response generation via Ollama (`/api/generate`)
- **In-memory SQLite** — Schema with `channels`, `messages`, `tasks`, `execution_logs` tables; prepared statements only, no ORM
- **Task scheduler** — Configurable poll interval, supports one-time and recurring (5-field cron) execution
- **Execution audit trail** — Every task run is logged with start/finish timestamps, status, result, and error
- **Typing indicators** — Telegram and Discord send typing status while the LLM generates a response
- **Graceful shutdown** — SIGINT/SIGTERM handling with context-based cancellation, adapter drain, and DB cleanup
- **Environment variable overrides** — All sensitive config fields (tokens, chat IDs) can be set via env vars
- **Cross-platform** — Builds for Linux and Windows from a single codebase (pure Go SQLite, no CGO required)



## Prerequisites

- Go 1.22+
- Ollama running locally (`http://localhost:11434`) with a pulled model
- Bot tokens for whichever platforms you want to enable

## Quick Start

```bash
git clone <repo-url> && cd rone

# Copy and configure
cp config.example.yaml config.yaml
# Edit config.yaml — set tokens, chat_id, model name

# Resolve dependencies
go mod tidy

# Validate schema
go run . migrate

# Test Ollama endpoint
go run . test-ollama

# Run in foreground with debug logging
go run . start --foreground --log-level debug
```

### Using environment variables instead of config

```bash
export RONE_TELEGRAM_TOKEN="your-bot-token"
export RONE_TELEGRAM_CHAT_ID="123456789"
export RONE_OLLAMA_MODEL="qwen3b-smallctx:latest"
go run . start --foreground
```

## CLI Reference

| Command              | Description                                         |
|----------------------|-----------------------------------------------------|
| `rone start`         | Launch daemon. Use `--foreground` to run in terminal |
| `rone stop`          | Send SIGTERM to running daemon via PID file          |
| `rone status`        | Check if daemon process is alive                     |
| `rone reload-config` | Validate config file syntax and required fields      |
| `rone test-ollama`   | Ping Ollama, list models, verify configured model    |
| `rone migrate`       | Run schema DDL against temp DB, verify table creation|

### Global Flags

| Flag            | Default        | Description                       |
|-----------------|----------------|-----------------------------------|
| `--config, -c`  | `config.yaml`  | Path to configuration file        |
| `--log-level`   | (from config)  | Override log level                 |
| `--foreground`  | `false`        | Run in foreground, don't daemonize |

## Configuration

All config is in YAML. Copy `config.example.yaml` to `config.yaml`. Environment variables take precedence over file values.

| Environment Variable       | Config Path            | Type     |
|----------------------------|------------------------|----------|
| `RONE_TELEGRAM_TOKEN`      | `telegram.token`       | string   |
| `RONE_TELEGRAM_CHAT_ID`    | `telegram.chat_id`     | int64    |
| `RONE_DISCORD_TOKEN`       | `discord.token`        | string   |
| `RONE_SLACK_TOKEN`         | `slack.token`          | string   |
| `RONE_SLACK_APP_TOKEN`     | `slack.app_token`      | string   |
| `RONE_OLLAMA_MODEL`        | `ollama.model`         | string   |
| `RONE_OLLAMA_ENDPOINT`     | `ollama.endpoint`      | string   |
| `RONE_LOG_LEVEL`           | `log.level`            | string   |
| `RONE_SCHEDULER_INTERVAL`  | `scheduler.interval`   | duration |
| `RONE_RATE_LIMIT`          | `rate_limit.messages_per_minute` | int |

### Telegram Debug Mode

Set `telegram.debug: true` in config to log every incoming message with full content, chat ID, sender, and message ID. Useful for verifying the bot is receiving messages and the `chat_id` filter is correct.

## Message Flow

```
Incoming message (Telegram/Discord/Slack)
  -> Adapter receives, sends typing indicator
  -> Upsert channel in SQLite
  -> Classify intent via Ollama (/api/generate)
  -> Store message in messages table
  -> Branch:
       "conversation" -> Generate response via Ollama -> Send reply
       "task"         -> Insert into tasks table -> Send acknowledgment
  -> Mark message as responded

Scheduler (runs every N seconds):
  -> SELECT tasks WHERE scheduled_time <= now AND status = 'pending'
  -> Execute each task, log result in execution_logs
  -> Recurring: compute next_run via cron parser, update task
  -> One-time: mark status = 'done'
  -> Send result back to originating channel
```

## SQLite Schema

Four tables, all in-memory (`file:memdb1?mode=memory&cache=shared`):

- `channels` — registered platform/channel pairs (unique constraint on platform+channel_id)
- `messages` — raw ingested messages with intent classification
- `tasks` — extracted actionable items with scheduling metadata
- `execution_logs` — audit trail for each task execution

Indexes: `idx_tasks_scheduled_time`, `idx_tasks_status`

## Docker

You can run ROne as a lightweight container.

### Build the image
```bash
docker build -t rone:latest .
```

### Run the container
Mount your `config.yaml` as a volume:
```bash
docker run -d \
  --name rone \
  -v $(pwd)/config.yaml:/app/config.yaml \
  rone:latest
```

Or use environment variables:
```bash
docker run -d \
  --name rone \
  -e RONE_TELEGRAM_TOKEN="your-token" \
  -e RONE_TELEGRAM_CHAT_ID="your-id" \
  -e RONE_OLLAMA_ENDPOINT="http://host.docker.internal:11434" \
  rone:latest
```
*Note: Use `host.docker.internal` on Windows/Mac to reach Ollama running on your host.*

## Building

```bash
# Current platform
go build -ldflags "-s -w" -o rone .

# Cross-compile for Linux (from any OS)
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/rone-linux-amd64 .

# Cross-compile for Windows
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o bin/rone-windows-amd64.exe .
```

Uses `modernc.org/sqlite` (pure Go) — no CGO required, cross-compilation works out of the box.

## Dependencies

| Package                              | Purpose                        |
|--------------------------------------|--------------------------------|
| `github.com/spf13/cobra`            | CLI framework                  |
| `gopkg.in/yaml.v3`                  | Config parsing                 |
| `modernc.org/sqlite`                | Pure Go SQLite driver          |
| `github.com/go-telegram-bot-api/telegram-bot-api/v5` | Telegram adapter |
| `github.com/bwmarrin/discordgo`     | Discord adapter                |
| `github.com/slack-go/slack`         | Slack adapter                  |

## License

MIT