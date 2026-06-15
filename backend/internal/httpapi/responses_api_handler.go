package httpapi

import (
	"context"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── Wire output types ─────────────────────────────────────────────────────────

// ResponsesAPIResponse is the top-level response for POST /v1/responses.
type ResponsesAPIResponse struct {
	ID        string                  `json:"id"`
	Object    string                  `json:"object"`
	CreatedAt int64                   `json:"created_at"`
	Model     string                  `json:"model"`
	Status    string                  `json:"status"`
	Output    []ResponsesAPIOutputItem `json:"output"`
	Usage     ResponsesAPIUsage       `json:"usage"`
}

// ResponsesAPIOutputItem is one element of the output array.
type ResponsesAPIOutputItem struct {
	Type    string                    `json:"type"`
	ID      string                    `json:"id"`
	Role    string                    `json:"role"`
	Status  string                    `json:"status"`
	Content []ResponsesAPIContentBlock `json:"content"`
}

// ResponsesAPIContentBlock is one content element inside an output item.
type ResponsesAPIContentBlock struct {
	Type        string                     `json:"type"`
	Text        string                     `json:"text,omitempty"`
	Annotations []ResponsesAPIAnnotation   `json:"annotations"`
}

// ResponsesAPIAnnotation is a placeholder type for future annotation support.
// The spec requires annotations to be an empty array when absent.
type ResponsesAPIAnnotation struct{}

// ResponsesAPIUsage holds token counts using Responses API field names.
type ResponsesAPIUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ── Serializer ────────────────────────────────────────────────────────────────

// finishReasonToStatus maps a provider finish reason to a Responses API status string.
// "stop" and "" (streaming intermediate chunks arrive with empty finish_reason) → "completed".
// Any other non-empty value (e.g. "length", "content_filter") → "incomplete".
func finishReasonToStatus(reason string) string {
	if reason == "stop" || reason == "" {
		return "completed"
	}
	return "incomplete"
}

// canonicalToResponsesAPIResponse converts a CanonicalResponse to the Responses API
// wire format. It always falls back to Choices (even when RawProviderResponse is nil).
func canonicalToResponsesAPIResponse(cr *CanonicalResponse) ResponsesAPIResponse {
	var outputItems []ResponsesAPIOutputItem
	for _, c := range cr.Choices {
		text := c.Message.Content
		status := finishReasonToStatus(c.FinishReason)
		outputItems = append(outputItems, ResponsesAPIOutputItem{
			Type:   "message",
			ID:     "msg_" + cr.ID,
			Role:   "assistant",
			Status: status,
			Content: []ResponsesAPIContentBlock{{
				Type:        "output_text",
				Text:        text,
				Annotations: []ResponsesAPIAnnotation{},
			}},
		})
	}

	// Ensure output is never nil (empty slice instead).
	if outputItems == nil {
		outputItems = []ResponsesAPIOutputItem{}
	}

	overallStatus := "completed"
	if len(cr.Choices) > 0 {
		overallStatus = finishReasonToStatus(cr.Choices[0].FinishReason)
	}

	return ResponsesAPIResponse{
		ID:        cr.ID,
		Object:    "response",
		CreatedAt: cr.Created,
		Model:     cr.Model,
		Status:    overallStatus,
		Output:    outputItems,
		Usage: ResponsesAPIUsage{
			InputTokens:  cr.Usage.PromptTokens,
			OutputTokens: cr.Usage.CompletionTokens,
			TotalTokens:  cr.Usage.TotalTokens,
		},
	}
}

// ── Handler ───────────────────────────────────────────────────────────────────

// ResponsesAPI handles POST /v1/responses (OpenAI Responses API).
// It follows the same call structure as ChatCompletions.
func (h *Handlers) ResponsesAPI(w http.ResponseWriter, r *http.Request) {
	tenant := auth.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication_error")
		return
	}

	parsed, err := parseResponsesAPIRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	// Ensure the orchestrator is wired (tests that construct Handlers directly may not wire it).
	if h.orchestrator == nil {
		h.orchestrator = &Orchestrator{h: h}
	}

	var sink ResponseSink
	var wrappedW http.ResponseWriter = w
	if parsed.Stream {
		rw := newResponsesAPIWriter(w, parsed.BodyModel)
		sink = rw
		wrappedW = rw
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
		// When the upstream provider returned raw bytes for verbatim passthrough
		// (Responses API), write them directly to avoid double-conversion loss
		// (web_search_call output items, tools, usage sub-details, etc.).
		if len(out.Response.RawPassthroughBody) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(out.Response.RawPassthroughBody) //nolint:errcheck
			return
		}
		writeJSON(w, http.StatusOK, canonicalToResponsesAPIResponse(out.Response))
	}
}
