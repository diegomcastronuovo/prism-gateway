package httpapi

import (
	"context"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// Orchestrator executes the complete routing + provider pipeline from a ParsedRequest.
type Orchestrator struct {
	h *Handlers
}

// OrchestratorInput is everything the Orchestration Core needs to process a
// request. Separates the ParsedRequest from infrastructure context.
type OrchestratorInput struct {
	Req    ParsedRequest
	Tenant *config.TenantConfig

	W http.ResponseWriter

	// R is for context propagation and tracing ONLY.
	// The core MUST NOT read R.Header or R.Body directly — use Req.Headers instead.
	R *http.Request

	// Sink is only populated when Req.Stream == true.
	// Nil for non-streaming requests; the result is returned via OrchestratorOutput.Response.
	Sink ResponseSink

	// ChatProviderFor returns a Provider for the given model config.
	// Injected from Handlers to avoid circular dependency.
	ChatProviderFor func(ctx context.Context, modelCfg *config.ModelConfig) (providers.Provider, bool)

	// EmbeddingProviderFor returns an EmbeddingProvider for the given provider name.
	// Injected from Handlers to avoid circular dependency.
	EmbeddingProviderFor func(ctx context.Context, provider string) (providers.EmbeddingProvider, bool)
}

// OrchestratorOutput is returned by Orchestrator.Run for non-streaming requests.
// For streaming requests the core writes to OrchestratorInput.Sink and returns
// OrchestratorOutput with Response == nil.
type OrchestratorOutput struct {
	// Response is non-nil for successful non-streaming requests.
	// Nil when the request was streaming (core wrote to Sink) or when Err != nil.
	Response *CanonicalResponse

	// Err is non-nil when the request failed at any stage.
	Err error
}

// Run executes the full pipeline for a single chat completions request.
// Implementation lives in orchestrator_run.go.
func (o *Orchestrator) Run(ctx context.Context, in OrchestratorInput) OrchestratorOutput {
	return orchestratorRun(ctx, o, in)
}
