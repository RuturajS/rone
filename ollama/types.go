package ollama

// GenerateRequest is the payload sent to /api/generate.
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// GenerateResponse is the response from /api/generate (non-streaming).
type GenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// TagsResponse is the response from /api/tags (model list).
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// ModelInfo represents a single model in the Ollama instance.
type ModelInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}
