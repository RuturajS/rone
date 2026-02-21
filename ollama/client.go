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
	"strings"
	"time"
)

var ErrOllamaUnavailable = errors.New("ollama: service unavailable after retries")

// classifyPrompt — very strict: only explicit reminders/scheduled actions are "task".
// Everything else defaults to conversation.
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

const failSafeMessage = "ROne: AI is temporarily unavailable. Your message has been logged."

// Client is a minimal HTTP client for the Ollama API.
type Client struct {
	endpoint   string
	model      string
	timeout    time.Duration
	maxRetries int
	http       *http.Client
}

// NewClient creates a new Ollama client.
func NewClient(endpoint, model string, timeout time.Duration, maxRetries int) *Client {
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		model:      model,
		timeout:    timeout,
		maxRetries: maxRetries,
		http:       &http.Client{},
	}
}

// Ping checks if Ollama is reachable and lists available models.
func (c *Client) Ping(ctx context.Context) (*TagsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var tags TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decode tags: %w", err)
	}
	return &tags, nil
}

// Classify determines if a message is "conversation" or "task".
// Only returns "task" for very explicit scheduling/reminder requests.
func (c *Client) Classify(ctx context.Context, content string) (string, error) {
	prompt := fmt.Sprintf(classifyPrompt, content)
	resp, err := c.generate(ctx, prompt)
	if err != nil {
		return "", err
	}

	// Extract just the classification word from the response
	result := strings.TrimSpace(strings.ToLower(resp))

	// Some models return extra text — extract the keyword
	if strings.Contains(result, "task") && !strings.Contains(result, "conversation") {
		return "task", nil
	}
	// Default to conversation for anything ambiguous
	slog.Debug("classify result", "raw", resp, "parsed", "conversation")
	return "conversation", nil
}

// Generate produces a conversational response.
func (c *Client) Generate(ctx context.Context, content string) (string, error) {
	return c.generate(ctx, content)
}

// FailSafeMessage returns the fallback message when Ollama is unavailable.
func FailSafeMessage() string {
	return failSafeMessage
}

// generate sends a prompt to Ollama with retry logic.
func (c *Client) generate(parentCtx context.Context, prompt string) (string, error) {
	body := GenerateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(parentCtx, c.timeout)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/api/generate", bytes.NewReader(payload))
		if err != nil {
			cancel()
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

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
