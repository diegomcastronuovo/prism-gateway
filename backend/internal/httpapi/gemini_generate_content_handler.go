package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── Wire output types ─────────────────────────────────────────────────────────

// GeminiGenerateContentResponse is the top-level response for the generateContent endpoint.
type GeminiGenerateContentResponse struct {
	Candidates    []GeminiCandidate    `json:"candidates"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string               `json:"modelVersion,omitempty"`
}

// GeminiCandidate is one response candidate returned by the model.
type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
	Index        int           `json:"index"`
}

// GeminiUsageMetadata holds token counts in the Gemini API field naming scheme.
type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// ── Serializer ────────────────────────────────────────────────────────────────

// finishReasonToGemini maps a provider finish reason to the Gemini FinishReason enum string.
func finishReasonToGemini(reason string) string {
	switch reason {
	case "stop", "":
		return "STOP"
	case "length":
		return "MAX_TOKENS"
	default:
		return "OTHER"
	}
}

// canonicalToGeminiResponse converts a CanonicalResponse to the Gemini generateContent
// wire format. When Choices is empty, candidates is an empty (non-nil) slice.
func canonicalToGeminiResponse(cr *CanonicalResponse) GeminiGenerateContentResponse {
	candidates := make([]GeminiCandidate, 0, len(cr.Choices))
	for _, c := range cr.Choices {
		candidates = append(candidates, GeminiCandidate{
			Content: GeminiContent{
				Role:  "model",
				Parts: []GeminiPart{{Text: c.Message.Content}},
			},
			FinishReason: finishReasonToGemini(c.FinishReason),
			Index:        c.Index,
		})
	}

	return GeminiGenerateContentResponse{
		Candidates: candidates,
		UsageMetadata: &GeminiUsageMetadata{
			PromptTokenCount:     cr.Usage.PromptTokens,
			CandidatesTokenCount: cr.Usage.CompletionTokens,
			TotalTokenCount:      cr.Usage.TotalTokens,
		},
	}
}

// ── Handler ───────────────────────────────────────────────────────────────────

// GeminiGenerateContent handles both:
//   - POST /v1/models/{model}:generateContent   (non-streaming)
//   - POST /v1/models/{model}:streamGenerateContent (streaming)
//
// The streaming flag is derived from the route path suffix, not the request body.
func (h *Handlers) GeminiGenerateContent(w http.ResponseWriter, r *http.Request) {
	tenant := auth.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication_error")
		return
	}

	stream := strings.HasSuffix(r.URL.Path, ":streamGenerateContent")

	parsed, err := parseGeminiGenerateContentRequest(r, stream)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error(), "invalid_request_error")
		return
	}

	// Ensure the orchestrator is wired (tests that construct Handlers directly may not wire it).
	if h.orchestrator == nil {
		h.orchestrator = &Orchestrator{h: h}
	}

	var sink ResponseSink
	var wrappedW http.ResponseWriter = w
	if parsed.Stream {
		gw := newGeminiGenerateContentWriter(w, parsed.BodyModel)
		sink = gw
		wrappedW = gw
	}

	out := h.orchestrator.Run(r.Context(), OrchestratorInput{
		Req:    parsed,
		Tenant: tenant,
		W:      wrappedW,
		R:      r,
		Sink:   sink,
		ChatProviderFor: func(ctx context.Context, modelCfg *config.ModelConfig) (providers.Provider, bool) {
			return h.providerForChatModel(ctx, modelCfg)
		},
		EmbeddingProviderFor: func(ctx context.Context, provider string) (providers.EmbeddingProvider, bool) {
			return h.embeddingProviderFor(ctx, provider)
		},
	})

	// Non-streaming success path: the orchestrator returned a CanonicalResponse.
	// Copy extra headers, then write the JSON body.
	if out.Response != nil {
		for k, v := range out.Response.ExtraHeaders {
			w.Header().Set(k, v)
		}
		writeJSON(w, http.StatusOK, canonicalToGeminiResponse(out.Response))
	}
}
