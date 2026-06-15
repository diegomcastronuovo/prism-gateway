package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── Request / Response types ─────────────────────────────────────────────────

type calibrateRequest struct {
	Dataset []calibrateItem `json:"dataset"`
}

type calibrateItem struct {
	Text  string `json:"text"`
	Route string `json:"route"`
}

type calibrateResponse struct {
	RecommendedThreshold float64 `json:"recommended_threshold"`
	Precision            float64 `json:"precision"`
	Recall               float64 `json:"recall"`
	F1                   float64 `json:"f1"`
}

// internal only — not serialised
type calibrateScore struct {
	similarity     float64
	predictedRoute string // nearest anchor's route_group; "" when not found
	trueRoute      string
	found          bool
}

// ── Handler ──────────────────────────────────────────────────────────────────

// CalibrateSemanticThreshold handles POST /admin/semantic/anchors/calibrate.
//
// Given a labeled dataset of {text, route} pairs, generates embeddings, compares
// each against existing anchors, sweeps 101 candidate thresholds (0.00…1.00),
// and returns the one that maximises F1.
func (h *Handlers) CalibrateSemanticThreshold(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Resolve tenant
	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	// 2. Parse request
	var req calibrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	// 3. Validate dataset
	if len(req.Dataset) == 0 {
		writeError(w, http.StatusBadRequest, "dataset must not be empty", "invalid_request_error")
		return
	}
	if len(req.Dataset) > 200 {
		writeError(w, http.StatusBadRequest, "dataset exceeds maximum of 200 items", "invalid_request_error")
		return
	}
	for i, item := range req.Dataset {
		if item.Text == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("dataset[%d].text is required", i), "invalid_request_error")
			return
		}
		if item.Route == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("dataset[%d].route is required", i), "invalid_request_error")
			return
		}
	}

	// 4. Resolve embedding provider — uses tenant routing.semantic.embedding_model (SPEC_103).
	embModel, embErr := h.embeddingModelForModality(ctx, tenant, "text")
	if embModel == nil {
		msg := "no embedding model configured"
		if embErr != nil {
			msg = embErr.Error()
		}
		writeError(w, http.StatusBadRequest, msg, "invalid_request_error")
		return
	}

	var ep providers.EmbeddingProvider
	if embModel.Mock.Enabled {
		ep = providers.NewMockProvider(embModel.Mock, embModel.Name, tenant.ID, embModel.Pricing, nil)
	} else {
		var ok bool
		ep, ok = h.embeddingProviderFor(ctx, embModel.Provider)
		if !ok {
			writeError(w, http.StatusInternalServerError, "no embedding model configured", "internal_error")
			return
		}
	}

	// 5. Loop over dataset items — generate embeddings and query nearest anchor
	scores := make([]calibrateScore, 0, len(req.Dataset))
	for _, item := range req.Dataset {
		embResp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{
			Input: []string{item.Text},
			Model: embModel.Name,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to generate embedding: "+err.Error(), "upstream_error")
			return
		}
		if len(embResp.Data) == 0 {
			writeError(w, http.StatusBadGateway, "no embedding data returned", "upstream_error")
			return
		}

		embedding := embResp.Data[0].Embedding
		_, routeGroup, _, distance, found, err := h.store.GetNearestSemanticAnchor(ctx, tenant.ID, embedding, "text")
		if err != nil {
			h.log.ErrorContext(ctx, "failed to query nearest semantic anchor during calibration",
				"tenant_id", tenant.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to query anchors", "internal_error")
			return
		}

		scores = append(scores, calibrateScore{
			similarity:     1 - distance,
			predictedRoute: routeGroup,
			trueRoute:      item.Route,
			found:          found,
		})
	}

	// 6. Guard: no anchors found at all
	anyFound := false
	for _, s := range scores {
		if s.found {
			anyFound = true
			break
		}
	}
	if !anyFound {
		writeError(w, http.StatusUnprocessableEntity, "no anchors found for this tenant", "invalid_request_error")
		return
	}

	// 7. Sweep thresholds and find the best F1
	threshold, precision, recall, f1 := sweepThresholds(scores)

	h.log.InfoContext(ctx, "semantic threshold calibration complete",
		"tenant_id", tenant.ID,
		"dataset_size", len(scores),
		"recommended_threshold", threshold,
		"f1", f1,
	)

	writeJSON(w, http.StatusOK, calibrateResponse{
		RecommendedThreshold: threshold,
		Precision:            precision,
		Recall:               recall,
		F1:                   f1,
	})
}

// ── Math helpers ─────────────────────────────────────────────────────────────

// sweepThresholds evaluates 101 thresholds (0.00…1.00) and returns the one
// with highest F1. On ties, the higher threshold wins (more conservative).
//
// Confusion matrix semantics (all dataset items have a route, so TN = 0):
//   - TP: found && sim ≥ T && predictedRoute == trueRoute
//   - FP: found && sim ≥ T && predictedRoute != trueRoute
//   - FN: !found || sim < T  (abstaining on a labeled item)
func sweepThresholds(scores []calibrateScore) (threshold, precision, recall, f1 float64) {
	const steps = 101
	bestF1 := -1.0
	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps-1)
		tp, fp, fn := 0, 0, 0
		for _, s := range scores {
			if s.found && s.similarity >= t {
				if s.predictedRoute == s.trueRoute {
					tp++
				} else {
					fp++
				}
			} else {
				fn++ // item has a known route but wasn't matched → FN
			}
		}
		p := 1.0
		if tp+fp > 0 {
			p = float64(tp) / float64(tp+fp)
		}
		r := 0.0
		if tp+fn > 0 {
			r = float64(tp) / float64(tp+fn)
		}
		f := 0.0
		if p+r > 0 {
			f = 2 * p * r / (p + r)
		}
		if f >= bestF1 { // >= so higher threshold wins on tie
			bestF1, threshold, precision, recall, f1 = f, t, p, r, f
		}
	}
	if bestF1 < 0 {
		bestF1 = 0 //nolint:ineffassign
	}
	return
}
