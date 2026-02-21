---
description: How to build and run the ROne project using WSL (Go installed on WSL, not Windows)
---

# Build & Run ROne via WSL

## Prerequisites
- Go is installed inside WSL (not Windows)
- The project lives on the Windows filesystem at `C:\Users\rutur\OneDrive\Desktop\Ai-Projects\ROne\rone`
- WSL can access it via `/mnt/c/Users/rutur/OneDrive/Desktop/Ai-Projects/ROne/rone`

## Steps

// turbo-all

1. Open WSL and navigate to the project:
```bash
wsl -e bash -c "cd '/mnt/c/Users/rutur/OneDrive/Desktop/Ai-Projects/ROne/rone' && pwd"
```

2. Initialize module and download dependencies:
```bash
wsl -e bash -c "cd '/mnt/c/Users/rutur/OneDrive/Desktop/Ai-Projects/ROne/rone' && go mod tidy"
```

3. Verify schema migration:
```bash
wsl -e bash -c "cd '/mnt/c/Users/rutur/OneDrive/Desktop/Ai-Projects/ROne/rone' && go run . migrate"
```

4. Test Ollama connectivity (requires Ollama running on localhost:11434):
```bash
wsl -e bash -c "cd '/mnt/c/Users/rutur/OneDrive/Desktop/Ai-Projects/ROne/rone' && go run . test-ollama"
```

5. Build for Linux:
```bash
wsl -e bash -c "cd '/mnt/c/Users/rutur/OneDrive/Desktop/Ai-Projects/ROne/rone' && GOOS=linux GOARCH=amd64 go build -ldflags '-s -w' -o bin/rone-linux-amd64 ."
```

6. Build for Windows:
```bash
wsl -e bash -c "cd '/mnt/c/Users/rutur/OneDrive/Desktop/Ai-Projects/ROne/rone' && GOOS=windows GOARCH=amd64 go build -ldflags '-s -w' -o bin/rone-windows-amd64.exe ."
```

7. Run in foreground (dev mode):
```bash
wsl -e bash -c "cd '/mnt/c/Users/rutur/OneDrive/Desktop/Ai-Projects/ROne/rone' && go run . start --foreground --log-level debug"
```

## Setting Tokens via Environment Variables
```bash
export RONE_TELEGRAM_TOKEN="your-bot-token"
export RONE_TELEGRAM_CHAT_ID="your-chat-id"
export RONE_DISCORD_TOKEN="your-bot-token"
```
