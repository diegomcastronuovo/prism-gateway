package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── Request / Response types ─────────────────────────────────────────────────

type similarityTestRequest struct {
	Text              string   `json:"text"`
	Modality          string   `json:"modality"` // "text" (default) | "image"
	ImageURL          string   `json:"image_url"`
	TopK              *int     `json:"top_k,omitempty"`
	IncludeAnchorText bool     `json:"include_anchor_text"`
	Threshold         *float64 `json:"threshold,omitempty"` // optional explicit override
}

type similarityTestInput struct {
	Text string `json:"text"`
}

type similarityTestItem struct {
	Name             string   `json:"name"`
	RouteGroup       string   `json:"route_group"`
	PreferredModels  []string `json:"preferred_models"`
	Distance         float64  `json:"distance"`
	Similarity       float64  `json:"similarity"`
	Matched          bool     `json:"matched"`
	AnchorText       *string  `json:"anchor_text"`
	AnchorVectorDims int      `json:"anchor_vector_dims"`
}

type similarityTestResponse struct {
	Object         string               `json:"object"`
	TenantID       string               `json:"tenant_id"`
	EmbeddingModel string               `json:"embedding_model"`
	Threshold      float64              `json:"threshold"`
	Input          similarityTestInput  `json:"input"`
	TopMatch       *similarityTestItem  `json:"top_match"`
	Data           []similarityTestItem `json:"data"`
}

type semanticEvalError struct {
	status  int
	msg     string
	errType string
}

func (e *semanticEvalError) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

// ── Handler ──────────────────────────────────────────────────────────────────

// SemanticSimilarityTest handles POST /v1/semantic/anchors/similarity-test.
//
// Diagnostic endpoint: generates an embedding for the input text, queries all
// semantic anchors for the tenant ordered by cosine distance, and returns
// similarity scores plus whether each anchor would match given the tenant's
// configured threshold.
func (h *Handlers) SemanticSimilarityTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Resolve tenant
	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	// 2. Parse request
	var req similarityTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	if req.Modality == "" {
		req.Modality = "text"
	}
	switch req.Modality {
	case "text":
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, "text is required for text modality", "invalid_request_error")
			return
		}
	case "image":
		if req.ImageURL == "" {
			writeError(w, http.StatusBadRequest, "image_url is required for image modality", "invalid_request_error")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "unsupported modality: "+req.Modality, "invalid_request_error")
		return
	}

	topK := 10
	if req.TopK != nil {
		if *req.TopK < 1 || *req.TopK > 200 {
			writeError(w, http.StatusBadRequest, "top_k must be between 1 and 200", "invalid_request_error")
			return
		}
		topK = *req.TopK
	}

	// 3. Evaluate similarity
	embeddingModel, threshold, items, topMatch, evalErr := h.evaluateSemanticSimilarity(ctx, tenant, req, topK)
	if evalErr != nil {
		writeError(w, evalErr.status, evalErr.msg, evalErr.errType)
		return
	}

	// 9. Log (no embeddings, no vectors)
	topMatchedName := ""
	topSimilarity := 0.0
	topMatched := false
	matchedLabel := "no_anchors"
	if topMatch != nil {
		topMatchedName = topMatch.Name
		topSimilarity = topMatch.Similarity
		topMatched = topMatch.Matched
		if topMatched {
			matchedLabel = "true"
		} else {
			matchedLabel = "false"
		}
	}

	h.log.InfoContext(ctx, "semantic similarity test",
		"tenant_id", tenant.ID,
		"top_k", topK,
		"matched_top", topMatched,
		"top_anchor_name", topMatchedName,
		"top_similarity", topSimilarity,
		"threshold", threshold,
		"embedding_model", embeddingModel,
	)

	gatewayotel.SemanticSimilarityTestCounter.WithLabelValues(matchedLabel).Inc()

	writeJSON(w, http.StatusOK, similarityTestResponse{
		Object:         "semantic_anchor_similarity_test",
		TenantID:       tenant.ID,
		EmbeddingModel: embeddingModel,
		Threshold:      threshold,
		Input:          similarityTestInput{Text: req.Text},
		TopMatch:       topMatch,
		Data:           items,
	})
}

func (h *Handlers) evaluateSemanticSimilarity(ctx context.Context, tenant *config.TenantConfig, req similarityTestRequest, topK int) (string, float64, []similarityTestItem, *similarityTestItem, *semanticEvalError) {
	// Resolve embedding model for the given modality.
	embModel, embErr := h.embeddingModelForModality(ctx, tenant, req.Modality)
	if embModel == nil {
		msg := "no embedding model configured for modality: " + req.Modality
		if embErr != nil {
			msg = embErr.Error()
		}
		return "", 0, nil, nil, &semanticEvalError{status: http.StatusBadRequest, msg: msg, errType: "invalid_request_error"}
	}

	// Get embedding provider from registry
	var ep providers.EmbeddingProvider
	if embModel.Mock.Enabled {
		ep = providers.NewMockProvider(embModel.Mock, embModel.Name, tenant.ID, embModel.Pricing, nil)
	} else {
		var ok bool
		ep, ok = h.embeddingProviderFor(ctx, embModel.Provider)
		if !ok {
			return "", 0, nil, nil, &semanticEvalError{status: http.StatusInternalServerError, msg: "no embedding model configured for modality: " + req.Modality, errType: "internal_error"}
		}
	}

	// Generate embedding for input (text or image URL based on modality).
	var embInput string
	if req.Modality == "image" {
		embInput = req.ImageURL
	} else {
		embInput = req.Text
	}
	embResp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{
		Input: []string{embInput},
		Model: embModel.Name,
	})
	if err != nil {
		return "", 0, nil, nil, &semanticEvalError{status: http.StatusBadGateway, msg: "failed to generate embedding: " + err.Error(), errType: "upstream_error"}
	}
	if len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
		return "", 0, nil, nil, &semanticEvalError{status: http.StatusBadGateway, msg: "no embedding data returned", errType: "upstream_error"}
	}
	embedding := embResp.Data[0].Embedding

	// Resolve threshold: explicit request override → tenant default → hardcoded default.
	threshold := resolveSemanticThreshold(tenant, req.Threshold)

	// Query anchors ordered by distance, filtered by modality
	rows, err := h.store.ListSemanticAnchorsSorted(ctx, tenant.ID, embedding, topK, req.Modality)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list semantic anchors for similarity test",
			"tenant_id", tenant.ID, "error", err)
		return "", 0, nil, nil, &semanticEvalError{status: http.StatusInternalServerError, msg: "failed to query anchors", errType: "internal_error"}
	}

	// Build response items
	items := make([]similarityTestItem, len(rows))
	for i, row := range rows {
		similarity := 1.0 - row.Distance
		matched := similarity >= threshold

		var anchorText *string
		if req.IncludeAnchorText {
			anchorText = row.AnchorText // nil until anchor_text column is added
		}

		items[i] = similarityTestItem{
			Name:             row.Name,
			RouteGroup:       row.RouteGroup,
			PreferredModels:  row.PreferredModels,
			Distance:         row.Distance,
			Similarity:       similarity,
			Matched:          matched,
			AnchorText:       anchorText,
			AnchorVectorDims: row.VectorDims,
		}
	}

	var topMatch *similarityTestItem
	if len(items) > 0 {
		topMatch = &items[0]
	}

	return embModel.Name, threshold, items, topMatch, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// defaultSemanticThreshold is the global fallback when no threshold is configured anywhere.
const defaultSemanticThreshold = 0.60

// resolveSemanticThreshold returns the effective semantic similarity threshold for a tenant.
//
// Precedence (highest to lowest):
//  1. override — explicit value from the request (non-nil and > 0)
//  2. tenant.Routing.Semantic.ThresholdDefault (> 0) — set by PATCH /admin/…/semantic-threshold
//  3. Stage/rule-level semantic_similarity.threshold (> 0) — static config fallback
//  4. Global default: 0.60
//
// The calibrated tenant default (level 2) intentionally beats per-rule static thresholds
// (level 3) so that PATCH calibration is always reflected in similarity-test.
func resolveSemanticThreshold(tenant *config.TenantConfig, override *float64) float64 {
	// Level 1: explicit per-request override
	if override != nil && *override > 0 {
		return *override
	}
	// Level 2: calibrated tenant-level default
	if tenant.Routing.Semantic.ThresholdDefault > 0 {
		return tenant.Routing.Semantic.ThresholdDefault
	}
	// Level 3: static per-rule threshold (v2 stages first, then legacy flat rules)
	for _, stage := range tenant.Routing.Smart.Stages {
		for _, rule := range stage.Rules {
			if rule.When.SemanticSimilarity != nil && rule.When.SemanticSimilarity.Threshold > 0 {
				return rule.When.SemanticSimilarity.Threshold
			}
		}
	}
	for _, rule := range tenant.Routing.Smart.Rules {
		if rule.When.SemanticSimilarity != nil && rule.When.SemanticSimilarity.Threshold > 0 {
			return rule.When.SemanticSimilarity.Threshold
		}
	}
	// Level 4: global default
	return defaultSemanticThreshold
}
