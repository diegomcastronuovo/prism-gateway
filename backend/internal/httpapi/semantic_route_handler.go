package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type createSemanticRouteRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Utterances  []string `json:"utterances"`
	Action      string   `json:"action"`
	Threshold   float64  `json:"threshold"`
}

// patchSemanticRouteRequest supports partial updates: nil pointer fields are omitted
// and the current persisted value is preserved. Utterances nil = unchanged; non-nil = replace.
type patchSemanticRouteRequest struct {
	Description *string  `json:"description"`
	Action      *string  `json:"action"`
	Threshold   *float64 `json:"threshold"`
	Utterances  []string `json:"utterances"`
}

type patchSemanticRouteResponse struct {
	TenantID    string   `json:"tenant_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Action      string   `json:"action"`
	Threshold   float64  `json:"threshold"`
	Utterances  []string `json:"utterances"`
}

type semanticRouteResponse struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Action      string   `json:"action"`
	Utterances  []string `json:"utterances"`
	Threshold   float64  `json:"threshold"`
}

type semanticRouteListResponse struct {
	Object   string                  `json:"object"`
	TenantID string                  `json:"tenant_id"`
	Data     []semanticRouteResponse `json:"data"`
}

// CreateSemanticRoute handles POST /admin/semantic/routes.
//
// Flow:
//  1. Resolve tenant from context (set by admin API-key auth).
//  2. Validate: name, action required; utterances >= 1.
//  3. Apply default threshold if not provided.
//  4. Obtain embedding model; 503 if none configured.
//  5. Generate embedding for each utterance; 503 if fails.
//  6. Call store.CreateSemanticRoute; 409 if ErrRouteAlreadyExists.
//  7. Return 201 with semanticRouteResponse.
func (h *Handlers) CreateSemanticRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	var req createSemanticRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "invalid_request_error")
		return
	}
	if req.Action == "" {
		writeError(w, http.StatusBadRequest, "action is required", "invalid_request_error")
		return
	}
	if len(req.Utterances) == 0 {
		writeError(w, http.StatusBadRequest, "utterances must contain at least one item", "invalid_request_error")
		return
	}

	if req.Threshold <= 0 {
		req.Threshold = 0.80
	}

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
			writeError(w, http.StatusServiceUnavailable, "no embedding provider available", "internal_error")
			return
		}
	}

	embeddings := make([][]float64, 0, len(req.Utterances))
	for _, utterance := range req.Utterances {
		embResp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{
			Input: []string{utterance},
			Model: embModel.Name,
		})
		if err != nil || len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
			writeError(w, http.StatusServiceUnavailable, "failed to generate embedding for utterance", "upstream_error")
			return
		}
		embeddings = append(embeddings, embResp.Data[0].Embedding)
	}

	err = h.store.CreateSemanticRoute(ctx, tenant.ID, req.Name, req.Description, req.Action,
		req.Threshold, req.Utterances, embeddings)
	if err != nil {
		if errors.Is(err, storage.ErrRouteAlreadyExists) {
			writeError(w, http.StatusConflict, "semantic route already exists", "conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to store semantic route",
			"tenant", tenant.ID, "name", req.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store route", "internal_error")
		return
	}

	writeJSON(w, http.StatusCreated, semanticRouteResponse{
		Name:        req.Name,
		Description: req.Description,
		Action:      req.Action,
		Utterances:  req.Utterances,
		Threshold:   req.Threshold,
	})
}

// ListSemanticRoutes handles GET /admin/semantic/routes.
func (h *Handlers) ListSemanticRoutes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	rows, err := h.store.ListSemanticRoutes(ctx, tenant.ID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list semantic routes", "tenant", tenant.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list routes", "internal_error")
		return
	}

	data := make([]semanticRouteResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, semanticRouteResponse{
			Name:        row.Name,
			Description: row.Description,
			Action:      row.Action,
			Threshold:   row.Threshold,
		})
	}

	writeJSON(w, http.StatusOK, semanticRouteListResponse{
		Object:   "list",
		TenantID: tenant.ID,
		Data:     data,
	})
}

// DeleteSemanticRoute handles DELETE /admin/semantic/routes/{name}.
func (h *Handlers) DeleteSemanticRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "route name is required", "invalid_request_error")
		return
	}

	found, err := h.store.DeleteSemanticRoute(ctx, tenant.ID, name)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to delete semantic route",
			"tenant", tenant.ID, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete route", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "route not found", "not_found_error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

// PatchSemanticRoute handles PATCH /admin/semantic/routes/{name}.
//
// Supports partial update: only provided fields are written; absent fields preserve
// the current persisted value. If utterances are provided they fully replace the
// existing set; new embeddings are generated before the atomic storage update.
func (h *Handlers) PatchSemanticRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "route name is required", "invalid_request_error")
		return
	}

	var req patchSemanticRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	// Validate present fields.
	if req.Action != nil && strings.TrimSpace(*req.Action) == "" {
		writeError(w, http.StatusBadRequest, "action cannot be empty", "invalid_request_error")
		return
	}
	if req.Threshold != nil && (*req.Threshold < 0 || *req.Threshold > 1) {
		writeError(w, http.StatusBadRequest, "threshold must be between 0 and 1", "invalid_request_error")
		return
	}
	if req.Utterances != nil {
		trimmed := make([]string, 0, len(req.Utterances))
		for _, u := range req.Utterances {
			if s := strings.TrimSpace(u); s != "" {
				trimmed = append(trimmed, s)
			}
		}
		if len(trimmed) == 0 {
			writeError(w, http.StatusBadRequest, "utterances must contain at least one non-empty item", "invalid_request_error")
			return
		}
		req.Utterances = trimmed
	}

	// Load current state — fail-fast before potentially expensive embedding and to
	// build the effective response without a second round-trip after the update.
	current, found, err := h.store.GetSemanticRoute(ctx, tenant.ID, name)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to load semantic route for patch",
			"tenant", tenant.ID, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load route", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "route not found", "not_found_error")
		return
	}

	// Embed replacement utterances when provided.
	var embeddings [][]float64
	if req.Utterances != nil {
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
				writeError(w, http.StatusServiceUnavailable, "no embedding provider available", "internal_error")
				return
			}
		}

		embeddings = make([][]float64, 0, len(req.Utterances))
		for _, utterance := range req.Utterances {
			embResp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{
				Input: []string{utterance},
				Model: embModel.Name,
			})
			if err != nil || len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
				writeError(w, http.StatusServiceUnavailable, "failed to generate embedding for utterance", "upstream_error")
				return
			}
			embeddings = append(embeddings, embResp.Data[0].Embedding)
		}
	}

	found, err = h.store.UpdateSemanticRoute(ctx, tenant.ID, name, storage.SemanticRoutePatch{
		Description: req.Description,
		Action:      req.Action,
		Threshold:   req.Threshold,
		Utterances:  req.Utterances,
	}, embeddings)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to update semantic route",
			"tenant", tenant.ID, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update route", "internal_error")
		return
	}
	if !found {
		// Race: deleted between our GET and the UPDATE.
		writeError(w, http.StatusNotFound, "route not found", "not_found_error")
		return
	}

	// Build effective response: current state overridden by the patch.
	resp := patchSemanticRouteResponse{
		TenantID:    tenant.ID,
		Name:        name,
		Description: current.Description,
		Action:      current.Action,
		Threshold:   current.Threshold,
		Utterances:  current.Utterances,
	}
	if req.Description != nil {
		resp.Description = *req.Description
	}
	if req.Action != nil {
		resp.Action = *req.Action
	}
	if req.Threshold != nil {
		resp.Threshold = *req.Threshold
	}
	if req.Utterances != nil {
		resp.Utterances = req.Utterances
	}

	writeJSON(w, http.StatusOK, resp)
}
