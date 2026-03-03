/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

var ErrOllamaUnavailable = errors.New("ollama: service unavailable after retries")

// classifyPrompt — very strict: only explicit reminders/scheduled actions are "task".
const classifyPrompt = `Classify this message as "task" or "conversation".

TASK means the user EXPLICITLY asks to:
- Set a reminder ("remind me to X at Y")
- Schedule something ("schedule X for tomorrow")
- Create a to-do ("add task: X")
- Set a timer ("in 30 minutes do X")

CONVERSATION means EVERYTHING else:
- Questions ("what is X?", "how to X?")
- Commands ("run X", "check X", "show me X")
- Greetings ("hi", "hello")
- General chat
- Requests for information
- Asking to do something NOW (not scheduled)

Default to "conversation" if unsure.
Reply with ONE word only: conversation or task

Message: %s`

// toolPrompt — system prompt for tool-aware generation.
var toolPrompt = fmt.Sprintf(`You are ROne, a helpful assistant running on a %s system.
You have access to the local terminal to check the status of THIS specific machine.

RULES FOR USING CMD:
1. ONLY use CMD for questions about the host machine's state (disk, ram, files, processes, network config, hardware).
2. NEVER use CMD for general knowledge, definitions, coding help, or explanations.
3. If the user asks "What is X?", provide a text explanation. Do NOT run a command.

Examples of when to use CMD:
- "what's my disk space" -> CMD: df -h
- "show running processes" -> CMD: ps aux
- "what's my IP" -> CMD: %s
- "list files here" -> CMD: ls -la
- "system uptime" -> CMD: uptime

Examples of when NOT to use CMD (Respond with TEXT only):
- "what is docker?" -> "Docker is a platform for developing, shipping, and running applications in containers..."
- "how to write a loop in Go?" -> "In Go, you use the for keyword..."
- "who is the president?" -> [Provide factual answer]

IMPORTANT: If using CMD, respond with ONLY the CMD line. If not using CMD, respond with a helpful conversational message.`, runtime.GOOS, ipCommand())

// summarizePrompt — used after command execution to produce a clean summary.
const summarizePrompt = `The user asked: "%s"

I ran this command on the system:
$ %s

Output:
%s

Provide a clear, concise summary of the result. Be direct and factual. If there's an error, explain what it means.`

const failSafeMessage = "ROne: AI is temporarily unavailable. Your message has been logged."

// Client is a minimal HTTP client for the Ollama API.
type Client struct {
	endpoint string
	model    string
	apiKey   string
	mode     string // "local" or "cloud"

	// Provider settings
	localEndpoint string
	localModel    string
	cloudEndpoint string
	cloudModel    string
	cloudAPIKey   string

	timeout    time.Duration
	maxRetries int
	http       *http.Client
	mu         sync.RWMutex
}

// NewClient creates a new Ollama client.
func NewClient(localEndpoint, localModel, cloudEndpoint, cloudModel, cloudAPIKey, mode string, timeout time.Duration, maxRetries int) *Client {
	c := &Client{
		endpoint:      localEndpoint,
		model:         localModel,
		apiKey:        "",
		mode:          mode,
		localEndpoint: localEndpoint,
		localModel:    localModel,
		cloudEndpoint: strings.TrimRight(cloudEndpoint, "/"),
		cloudModel:    cloudModel,
		cloudAPIKey:   strings.TrimSpace(cloudAPIKey),
		timeout:       timeout,
		maxRetries:    maxRetries,
		http:          &http.Client{},
	}

	if mode == "cloud" {
		c.endpoint = c.cloudEndpoint
		c.model = c.cloudModel
		c.apiKey = c.cloudAPIKey
	} else {
		c.endpoint = strings.TrimRight(localEndpoint, "/")
	}

	return c
}

// SetModel changes the model used for future requests.
func (c *Client) SetModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = model
	if c.mode == "cloud" {
		c.cloudModel = model
	} else {
		c.localModel = model
	}
}

// GetModel returns the currently configured model.
func (c *Client) GetModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

// GetMode returns the current operation mode.
func (c *Client) GetMode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode
}

// SwitchMode toggles between local and cloud providers.
func (c *Client) SwitchMode(mode string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if mode == "cloud" {
		if c.cloudEndpoint == "" {
			return errors.New("cloud endpoint not configured")
		}
		c.mode = "cloud"
		c.endpoint = c.cloudEndpoint
		c.model = c.cloudModel
		c.apiKey = c.cloudAPIKey
	} else {
		c.mode = "local"
		c.endpoint = strings.TrimRight(c.localEndpoint, "/")
		c.model = c.localModel
		c.apiKey = ""
	}
	slog.Info("ollama provider switched", "mode", c.mode, "endpoint", c.endpoint, "model", c.model)
	return nil
}

// Ping checks if Ollama is reachable and lists available models.
func (c *Client) Ping(ctx context.Context) (*TagsResponse, error) {
	c.mu.RLock()
	currentMode := c.mode
	currentEndpoint := c.endpoint
	currentAPIKey := c.apiKey
	c.mu.RUnlock()

	// Cloud mode might not support /api/tags, so we do a simple check
	url := currentEndpoint + "/api/tags"
	if currentMode == "cloud" {
		// Just try to hit the base or /models if it's OpenAI-like, 
		// but since we just need to know if it's "OK", we can try a simple request
		// For now, let's just return empty tags for cloud to signal "reachable" 
		// if a HEAD request works, or just assume it's OK if we can reach it.
		// Actually, let's try /models which is common for OpenAI.
		url = currentEndpoint + "/models"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if currentAPIKey != "" {
		auth := currentAPIKey
		if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			auth = "Bearer " + auth
		}
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ROne-Assistant/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	if currentMode == "cloud" {
		// Return dummy tags so Daemon.New doesn't fail
		return &TagsResponse{Models: []ModelInfo{{Name: c.model}}}, nil
	}

	var tags TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decode tags: %w", err)
	}
	return &tags, nil
}

// Classify determines if a message is "conversation" or "task".
func (c *Client) Classify(ctx context.Context, content string) (string, error) {
	prompt := fmt.Sprintf(classifyPrompt, content)
	resp, err := c.generateWithSystem(ctx, prompt, "")
	if err != nil {
		return "", err
	}

	result := strings.TrimSpace(strings.ToLower(resp))
	if strings.Contains(result, "task") && !strings.Contains(result, "conversation") {
		return "task", nil
	}
	slog.Debug("classify result", "raw", resp, "parsed", "conversation")
	return "conversation", nil
}

// GenerateWithTools sends the user message with the tool-aware system prompt.
func (c *Client) GenerateWithTools(ctx context.Context, userMessage string) (string, error) {
	return c.generateWithSystem(ctx, userMessage, toolPrompt)
}

// Summarize sends the command output back to the LLM for a clean human-readable summary.
func (c *Client) Summarize(ctx context.Context, userQuestion, command, output string) (string, error) {
	prompt := fmt.Sprintf(summarizePrompt, userQuestion, command, output)
	return c.generateWithSystem(ctx, prompt, "")
}

// Generate produces a plain conversational response (no tool awareness).
func (c *Client) Generate(ctx context.Context, content string) (string, error) {
	return c.generateWithSystem(ctx, content, "")
}

// ParseToolResponse checks if the LLM response contains a CMD: directive.
func ParseToolResponse(response string) (string, bool) {
	lines := strings.SplitN(response, "\n", 2)
	firstLine := strings.TrimSpace(lines[0])

	if strings.HasPrefix(strings.ToUpper(firstLine), "CMD:") {
		cmd := strings.TrimSpace(firstLine[4:])
		if cmd != "" {
			return cmd, true
		}
	}
	return "", false
}

// FailSafeMessage returns the fallback message when Ollama is unavailable.
func FailSafeMessage() string {
	return failSafeMessage
}

// generate sends a prompt to Ollama with retry logic and an optional system prompt.
func (c *Client) generateWithSystem(parentCtx context.Context, prompt string, system string) (string, error) {
	c.mu.RLock()
	currentModel := c.model
	currentEndpoint := c.endpoint
	currentAPIKey := c.apiKey
	currentMode := c.mode
	c.mu.RUnlock()

	var payload []byte
	var url string
	var err error

	url = currentEndpoint + "/api/generate"
	body := GenerateRequest{
		Model:  currentModel,
		Prompt: prompt,
		System: system,
		Stream: false,
	}
	payload, err = json.Marshal(body)

	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(parentCtx, c.timeout)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			cancel()
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "ROne-Assistant/1.0")
		if currentAPIKey != "" {
			auth := currentAPIKey
			if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				auth = "Bearer " + auth
			}
			req.Header.Set("Authorization", auth)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			cancel()
			slog.Warn("ollama request failed", "attempt", attempt+1, "error", err)
			if attempt < c.maxRetries {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return "", ErrOllamaUnavailable
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()

		if err != nil {
			slog.Warn("ollama read body failed", "attempt", attempt+1, "error", err)
			if attempt < c.maxRetries {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return "", ErrOllamaUnavailable
		}

		if resp.StatusCode != http.StatusOK {
			slog.Warn("ollama non-200", "status", resp.StatusCode, "body", string(data))
			if resp.StatusCode == http.StatusUnauthorized {
				slog.Error("ollama: 401 Unauthorized. Access Denied.", 
					"mode", currentMode, 
					"endpoint", url,
					"key_len", len(currentAPIKey),
					"hint", "Ensure your RONE_OLLAMA_CLOUD_KEY is correct. We sent exactly what was in the config. If your provider requires a 'Bearer ' prefix, add it manually in config.yaml.")
			}
			if attempt < c.maxRetries {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
		}

		var genResp GenerateResponse
		if err := json.Unmarshal(data, &genResp); err != nil {
			return "", fmt.Errorf("decode response: %w", err)
		}
		return genResp.Response, nil
	}

	return "", ErrOllamaUnavailable
}

// ipCommand returns the platform-appropriate IP command.
func ipCommand() string {
	if runtime.GOOS == "windows" {
		return "ipconfig"
	}
	return "ip addr"
}
