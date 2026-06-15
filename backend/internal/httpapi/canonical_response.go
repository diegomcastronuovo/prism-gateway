package httpapi

import (
	"encoding/json"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// CanonicalResponse is what the Orchestration Core returns to the endpoint
// handler after completing a non-streaming request. The handler formats it
// into its wire format (ChatCompletionResponse, etc.).
//
// CanonicalResponse MUST NOT contain any net/http types.
type CanonicalResponse struct {
	// Provider response fields, already normalised.
	ID           string
	Object       string
	Created      int64
	Model        string // model that actually answered
	Content      string // full assistant text
	FinishReason string
	Usage        providers.Usage
	Choices      []providers.ChatChoice

	// RawProviderResponse is the unmodified JSON body returned by the upstream
	// provider. Nil on semantic cache hits, streaming responses, and error paths.
	// Future API-style serializers use this for lossless response reconstruction.
	RawProviderResponse json.RawMessage

	// RawPassthroughBody is non-nil only when the provider signals that the raw
	// bytes should be written verbatim to the client (e.g. Responses API). It
	// carries the same bytes as RawProviderResponse but its presence is the
	// explicit signal to skip re-serialisation in the endpoint handler.
	RawPassthroughBody json.RawMessage

	// Routing metadata for response headers.
	SelectedModel string
	Provider      string
	RouteGroup    string
	Latency       time.Duration

	// Additional headers that the core wants to set (e.g. X-Semantic-Cache).
	// The endpoint handler copies these to http.ResponseWriter before writing the body.
	ExtraHeaders map[string]string
}

// canonicalToChatCompletionResponse converts a CanonicalResponse to the
// OpenAI-compatible ChatCompletionResponse wire format.
func canonicalToChatCompletionResponse(cr *CanonicalResponse) ChatCompletionResponse {
	resp := ChatCompletionResponse{
		ID:      cr.ID,
		Object:  cr.Object,
		Created: cr.Created,
		Model:   cr.Model,
		Usage: Usage{
			PromptTokens:     cr.Usage.PromptTokens,
			CompletionTokens: cr.Usage.CompletionTokens,
			TotalTokens:      cr.Usage.TotalTokens,
		},
	}
	for _, c := range cr.Choices {
		resp.Choices = append(resp.Choices, ChatChoice{
			Index:        c.Index,
			Message:      mapProviderMessage(c.Message),
			FinishReason: c.FinishReason,
		})
	}
	return resp
}
