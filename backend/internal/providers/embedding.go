package providers

import "context"

// EmbeddingRequest is the provider-agnostic embedding request.
type EmbeddingRequest struct {
	Input []string // normalized from string | []string
	Model string
	User  string // optional
}

// EmbeddingData holds one embedding vector with its index.
type EmbeddingData struct {
	Object    string    `json:"object"`    // "embedding"
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// EmbeddingUsage holds token usage for an embedding request.
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbeddingResponse is the OpenAI-compatible embedding response.
type EmbeddingResponse struct {
	Object string          `json:"object"` // "list"
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`
}

// EmbeddingProvider is the interface every embedding backend must implement.
type EmbeddingProvider interface {
	CreateEmbedding(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)
}
