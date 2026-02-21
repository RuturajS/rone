# ROne Docker Documentation

This guide covers building, running, and managing ROne using Docker and Docker Compose.

## Prerequisites

- Docker installed on your system.
- Docker Compose (optional, for easier management).
- Ollama running on the host machine (default: `http://localhost:11434`).

## Building the Image

Build the ROne image from the project root:

```bash
docker build -t rone:latest .
```

## Running with Docker Compose (Recommended)

1. Create or update your `config.yaml`.
2. Start the service:

```bash
docker-compose up -d
```

3. View logs:

```bash
docker-compose logs -f rone
```

## Running with Docker CLI

### Basic Run (Using config.yaml)
```bash
docker run -d \
  --name rone \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  --extra-hosts=host.docker.internal:host-gateway \
  rone:latest
```

### Advanced Run (Using Environment Variables)
```bash
docker run -d \
  --name rone \
  -e RONE_TELEGRAM_TOKEN="your_token" \
  -e RONE_TELEGRAM_CHAT_ID="your_chat_id" \
  -e RONE_OLLAMA_ENDPOINT="http://host.docker.internal:11434" \
  -e RONE_TOOLS_ENABLED="true" \
  -e RONE_LOG_LEVEL="debug" \
  --extra-hosts=host.docker.internal:host-gateway \
  --restart unless-stopped \
  rone:latest
```

## Docker Management Commands

| Action | Command |
|--------|---------|
| Start Container | `docker start rone` |
| Stop Container | `docker stop rone` |
| Restart Container | `docker restart rone` |
| View Logs | `docker logs -f rone` |
| Execute Shell Inside | `docker exec -it rone bash` |
| Remove Container | `docker rm -f rone` |
| Remove Image | `docker rmi rone:latest` |
| Check Resource Usage | `docker stats rone` |

## Configuration Properties (Environment Variables)

All configuration options can be overridden using environment variables:

| Variable | Description |
|----------|-------------|
| `RONE_TELEGRAM_TOKEN` | Telegram Bot API Token |
| `RONE_TELEGRAM_CHAT_ID` | Allowed Telegram Chat ID |
| `RONE_DISCORD_TOKEN` | Discord Bot Token |
| `RONE_SLACK_TOKEN` | Slack Bot Token |
| `RONE_SLACK_APP_TOKEN` | Slack App-level Token (Socket Mode) |
| `RONE_OLLAMA_ENDPOINT` | Ollama API URL (use `http://host.docker.internal:11434`) |
| `RONE_OLLAMA_MODEL` | Ollama Model name |
| `RONE_TOOLS_ENABLED` | Enable/Disable terminal command execution ("true"/"false") |
| `RONE_TOOLS_TIMEOUT` | Max duration for command execution (e.g., "30s") |
| `RONE_LOG_LEVEL` | Logging level (debug, info, warn, error) |

## Networking Notes

### Accessing Ollama on Host
When running inside Docker, `localhost` refers to the container itself. To access Ollama running on your host machine:
- **Windows/Mac**: Use `http://host.docker.internal:11434`.
- **Linux**: Use `http://host.docker.internal:11434` and ensure you pass `--extra-hosts=host.docker.internal:host-gateway` to the `docker run` command or use the provided `docker-compose.yml`.

## Troubleshooting

### Container keeps restarting
Check logs to see if there is a configuration error:
`docker logs rone`

### Tools feature not working
Ensure the container has the necessary tools installed. The default `Dockerfile` includes `bash`, `curl`, `iproute2`, and `procps`. If you need more tools, you must add them to the `apk add` list in the `Dockerfile` and rebuild.

### Permission Denied
By default, the container runs as a non-root user (`rone`). If a command requires root privileges, it will fail unless you modify the `Dockerfile` to run as root or use `sudo` (not installed by default for security).
