/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package ollama

// --- Ollama (Local) Types ---

type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system,omitempty"`
	Stream bool   `json:"stream"`
}

// GenerateResponse is the response from /api/generate (non-streaming).
type GenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// --- OpenAI Compatible (Cloud) Types ---

// ChatCompletionRequest is the payload sent to /chat/completions.
type ChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []ChatCompletionMessage `json:"messages"`
}

// ChatCompletionMessage is a single message in a chat completion.
type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse is the simplified response from /chat/completions.
type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// --- Common Types ---

// TagsResponse is the response from /api/tags (model list).
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// ModelInfo represents a single model in the Ollama instance.
type ModelInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}
