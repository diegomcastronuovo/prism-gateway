package httpapi

import (
	"encoding/json"
	"net/http"
	"time"
)

type semanticTestRequest struct {
	Text string `json:"text"`
}

type semanticTestTopMatch struct {
	Anchor     string  `json:"anchor"`
	RouteGroup string  `json:"route_group"`
	Similarity float64 `json:"similarity"`
	Passed     bool    `json:"passed"`
}

type semanticTestDecision struct {
	Type   string  `json:"type"`
	Result *string `json:"result"`
}

type semanticTestResponse struct {
	Input     string                `json:"input"`
	Threshold float64               `json:"threshold"`
	TopMatch  *semanticTestTopMatch `json:"top_match"`
	Decision  semanticTestDecision  `json:"decision"`
}

type semanticTestErrorResponse struct {
	Input string `json:"input"`
	Error string `json:"error"`
}

// AdminSemanticTest handles POST /admin/semantic/test.
// Admin-only semantic test adapter for the playground.
func (h *Handlers) AdminSemanticTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}

	var req semanticTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error")
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required", "invalid_request_error")
		return
	}
	if len(req.Text) > 10000 {
		writeError(w, http.StatusBadRequest, "text must be <= 10000 chars", "invalid_request_error")
		return
	}

	ctx := r.Context()
	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	_, threshold, items, topMatch, evalErr := h.evaluateSemanticSimilarity(ctx, tenant, similarityTestRequest{
		Text:     req.Text,
		Modality: "text",
	}, 1)
	if evalErr != nil {
		h.log.ErrorContext(ctx, "semantic evaluation failed", "error", evalErr.msg, "tenant_id", tenant.ID)
		writeJSON(w, http.StatusInternalServerError, semanticTestErrorResponse{
			Input: req.Text,
			Error: "semantic evaluation failed",
		})
		return
	}

	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "no semantic anchors configured", "semantic_error")
		return
	}

	var top *semanticTestTopMatch
	decision := semanticTestDecision{Type: "none", Result: nil}
	if topMatch != nil && topMatch.Similarity >= threshold {
		top = &semanticTestTopMatch{
			Anchor:     topMatch.Name,
			RouteGroup: topMatch.RouteGroup,
			Similarity: topMatch.Similarity,
			Passed:     true,
		}
		decision.Type = "semantic_anchor"
		decision.Result = &top.RouteGroup
	}

	h.log.InfoContext(ctx, "semantic_test_executed",
		"event", "semantic_test_executed",
		"tenant_id", tenant.ID,
		"input_length", len(req.Text),
		"timestamp", time.Now().UTC().Format(time.RFC3339),
	)

	writeJSON(w, http.StatusOK, semanticTestResponse{
		Input:     req.Text,
		Threshold: threshold,
		TopMatch:  top,
		Decision:  decision,
	})
}
