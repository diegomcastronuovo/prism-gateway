package httpapi

import (
	"context"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// embeddingAccumulator collects token/char counts across all internal embedding
// calls (tool routing, semantic cache, semantic routing) made during a single
// request. A single log entry is written via flush() when the request completes.
//
// This replaces the addEmb anonymous closure that previously captured local
// variables from ChatCompletions via defer. Using an explicit struct makes the
// captured state visible, testable, and parameter-safe across function calls.
type embeddingAccumulator struct {
	tokens int
	chars  int
	model  *config.ModelConfig
}

// Add records token and character counts from a single embedding call.
// The first model encountered is retained as the representative model for the
// log entry (matching the previous closure behaviour where embAccModel is set
// once and never overwritten).
func (a *embeddingAccumulator) Add(tokens, chars int, model *config.ModelConfig) {
	a.tokens += tokens
	a.chars += chars
	if a.model == nil {
		a.model = model
	}
}

// embLogFn is the function signature used by flush to write the accumulated
// embedding log row. Matches logInternalEmbeddingAsync on both Handlers and
// Orchestrator.
type embLogFn func(ctx context.Context, requestID string, tenant *config.TenantConfig, model *config.ModelConfig, tokens, chars int)

// flush writes the accumulated embedding usage as a single async log row.
// No-op when no embedding calls were made (model == nil).
// requestID should be the client-visible ID (resp.ID) when available so that
// the embedding entry shares the same request_id as the LLM entry in FinOps.
func (a *embeddingAccumulator) flush(ctx context.Context, logFn embLogFn, requestID string, tenant *config.TenantConfig) {
	if a.model == nil {
		return // no embedding calls made
	}
	logFn(ctx, requestID, tenant, a.model, a.tokens, a.chars)
}
