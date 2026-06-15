package providers

import (
	"context"
	"net/http"
)

// XAI implements the Provider interface for xAI's Grok API (OpenAI-compatible).
type XAI struct {
	openAICompat
}

func NewXAI(baseURL, apiKey string) *XAI {
	return &XAI{openAICompat: newOpenAICompat(baseURL, apiKey)}
}

// SetHTTPClient allows overriding the default HTTP client (for testing).
func (x *XAI) SetHTTPClient(c *http.Client) {
	x.openAICompat.setHTTPClient(c)
}

func (x *XAI) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return x.openAICompat.chatCompletion(ctx, req)
}

func (x *XAI) ChatCompletionStream(ctx context.Context, req ChatRequest) (*StreamResponse, error) {
	// TODO: Implement full SSE streaming passthrough for xAI.
	return nil, ErrStreamingNotSupported
}
