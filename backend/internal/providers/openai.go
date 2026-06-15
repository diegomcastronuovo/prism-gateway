package providers

import (
	"context"
	"fmt"
	"net/http"
)

// OpenAI implements the Provider interface for OpenAI-compatible APIs.
type OpenAI struct {
	openAICompat
}

func NewOpenAI(baseURL, apiKey string) *OpenAI {
	return &OpenAI{openAICompat: newOpenAICompat(baseURL, apiKey)}
}

// SetHTTPClient allows overriding the default HTTP client (for testing).
func (o *OpenAI) SetHTTPClient(c *http.Client) {
	o.openAICompat.setHTTPClient(c)
}

func (o *OpenAI) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return o.openAICompat.chatCompletion(ctx, req)
}

func (o *OpenAI) ChatCompletionStream(ctx context.Context, req ChatRequest) (*StreamResponse, error) {
	return o.openAICompat.chatCompletionStream(ctx, req)
}

// UpstreamError represents a non-200 response from the upstream provider.
type UpstreamError struct {
	StatusCode int
	Body       string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream returned status %d: %s", e.StatusCode, e.Body)
}

// IsRetryable returns true for errors that should trigger fallback (5xx, 429, 404, timeouts).
// 404 is retryable because it may indicate model/endpoint mismatch that could succeed with another provider.
// Other 4xx errors (400, 401, 403) are non-retryable as they indicate client/auth/config errors.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if ue, ok := err.(*UpstreamError); ok {
		// Retryable: 5xx (server errors), 429 (rate limit), 404 (model not found/endpoint mismatch)
		if ue.StatusCode >= 500 || ue.StatusCode == 429 || ue.StatusCode == 404 {
			return true
		}
		// Non-retryable: all other 4xx (client errors like 400, 401, 403)
		return false
	}
	// Network errors, timeouts, context deadline exceeded are retryable
	return true
}
