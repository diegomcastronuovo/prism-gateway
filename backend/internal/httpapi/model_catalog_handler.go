package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// modelCatalogRequest is the body for POST /admin/model-catalog and
// PUT /admin/model-catalog/{provider}/{id}.
type modelCatalogRequest struct {
	ID                          string  `json:"id"`
	Provider                    string  `json:"provider"`
	DisplayName                 string  `json:"display_name"`
	Type                        string  `json:"type"`
	PromptPer1M                 float64 `json:"prompt_per_1m"`
	CachedInputPer1M            float64 `json:"cached_input_per_1m"`
	CompletionPer1M             float64 `json:"completion_per_1m"`
	InfrastructureMonthlyUSD    float64 `json:"infrastructure_monthly_usd"`
	IsActive                    *bool   `json:"is_active"`
	LongContext                 bool    `json:"long_context"`
	LongContextStartTokens      int     `json:"long_context_start_tokens"`
	LongContextPromptPer1M      float64 `json:"long_context_prompt_per_1m"`
	LongContextCachedInputPer1M float64 `json:"long_context_cached_input_per_1m"`
	LongContextCompletionPer1M  float64 `json:"long_context_completion_per_1m"`
}

type modelCatalogListResponse struct {
	Data  []storage.ModelCatalogEntry `json:"data"`
	Total int                         `json:"total"`
}

// AdminListModelCatalog handles GET /admin/model-catalog.
//
// Optional query params: provider, type, active (true|false).
func (h *Handlers) AdminListModelCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var filter storage.ModelCatalogFilter
	if v := r.URL.Query().Get("provider"); v != "" {
		filter.Provider = &v
	}
	if v := r.URL.Query().Get("type"); v != "" {
		filter.Type = &v
	}
	if v := r.URL.Query().Get("active"); v != "" {
		active := strings.EqualFold(v, "true")
		filter.IsActive = &active
	}

	entries, err := h.store.ListModelCatalog(ctx, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list model catalog", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list model catalog", "internal_error")
		return
	}

	writeJSON(w, http.StatusOK, modelCatalogListResponse{
		Data:  entries,
		Total: len(entries),
	})
}

// AdminCreateModelCatalogEntry handles POST /admin/model-catalog.
func (h *Handlers) AdminCreateModelCatalogEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req modelCatalogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	if strings.TrimSpace(req.ID) == "" {
		writeError(w, http.StatusBadRequest, "id is required", "invalid_request_error")
		return
	}
	if strings.TrimSpace(req.Provider) == "" {
		writeError(w, http.StatusBadRequest, "provider is required", "invalid_request_error")
		return
	}
	if req.PromptPer1M < 0 {
		writeError(w, http.StatusBadRequest, "prompt_per_1m must be non-negative", "invalid_request_error")
		return
	}
	if req.CachedInputPer1M < 0 {
		writeError(w, http.StatusBadRequest, "cached_input_per_1m must be non-negative", "invalid_request_error")
		return
	}
	if req.CompletionPer1M < 0 {
		writeError(w, http.StatusBadRequest, "completion_per_1m must be non-negative", "invalid_request_error")
		return
	}
	if req.InfrastructureMonthlyUSD < 0 {
		writeError(w, http.StatusBadRequest, "infrastructure_monthly_usd must be non-negative", "invalid_request_error")
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	entryType := req.Type
	if entryType == "" {
		entryType = "chat"
	}

	longContextStartTokens := req.LongContextStartTokens
	if longContextStartTokens == 0 {
		longContextStartTokens = 272000
	}

	entry := storage.ModelCatalogEntry{
		ID:                          req.ID,
		Provider:                    req.Provider,
		DisplayName:                 req.DisplayName,
		Type:                        entryType,
		PromptPer1M:                 req.PromptPer1M,
		CachedInputPer1M:            req.CachedInputPer1M,
		CompletionPer1M:             req.CompletionPer1M,
		InfrastructureMonthlyUSD:    req.InfrastructureMonthlyUSD,
		IsActive:                    isActive,
		LongContext:                 req.LongContext,
		LongContextStartTokens:      longContextStartTokens,
		LongContextPromptPer1M:      req.LongContextPromptPer1M,
		LongContextCachedInputPer1M: req.LongContextCachedInputPer1M,
		LongContextCompletionPer1M:  req.LongContextCompletionPer1M,
	}

	created, err := h.store.CreateModelCatalogEntry(ctx, entry)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, "model catalog entry already exists", "conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to create model catalog entry",
			"provider", req.Provider, "id", req.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create model catalog entry", "internal_error")
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// AdminUpdateModelCatalogEntry handles PUT /admin/model-catalog/{provider}/{id}.
func (h *Handlers) AdminUpdateModelCatalogEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	provider := r.PathValue("provider")
	id := r.PathValue("id")
	if provider == "" || id == "" {
		writeError(w, http.StatusBadRequest, "provider and id path parameters are required", "invalid_request_error")
		return
	}

	var req modelCatalogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	if req.PromptPer1M < 0 {
		writeError(w, http.StatusBadRequest, "prompt_per_1m must be non-negative", "invalid_request_error")
		return
	}
	if req.CachedInputPer1M < 0 {
		writeError(w, http.StatusBadRequest, "cached_input_per_1m must be non-negative", "invalid_request_error")
		return
	}
	if req.CompletionPer1M < 0 {
		writeError(w, http.StatusBadRequest, "completion_per_1m must be non-negative", "invalid_request_error")
		return
	}
	if req.InfrastructureMonthlyUSD < 0 {
		writeError(w, http.StatusBadRequest, "infrastructure_monthly_usd must be non-negative", "invalid_request_error")
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	entryType := req.Type
	if entryType == "" {
		entryType = "chat"
	}

	longContextStartTokens := req.LongContextStartTokens
	if longContextStartTokens == 0 {
		longContextStartTokens = 272000
	}

	entry := storage.ModelCatalogEntry{
		ID:                          id,
		Provider:                    provider,
		DisplayName:                 req.DisplayName,
		Type:                        entryType,
		PromptPer1M:                 req.PromptPer1M,
		CachedInputPer1M:            req.CachedInputPer1M,
		CompletionPer1M:             req.CompletionPer1M,
		InfrastructureMonthlyUSD:    req.InfrastructureMonthlyUSD,
		IsActive:                    isActive,
		LongContext:                 req.LongContext,
		LongContextStartTokens:      longContextStartTokens,
		LongContextPromptPer1M:      req.LongContextPromptPer1M,
		LongContextCachedInputPer1M: req.LongContextCachedInputPer1M,
		LongContextCompletionPer1M:  req.LongContextCompletionPer1M,
	}

	updated, found, err := h.store.UpdateModelCatalogEntry(ctx, entry)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to update model catalog entry",
			"provider", provider, "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update model catalog entry", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "model catalog entry not found", "not_found_error")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// AdminDeleteModelCatalogEntry handles DELETE /admin/model-catalog/{provider}/{id}.
func (h *Handlers) AdminDeleteModelCatalogEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	provider := r.PathValue("provider")
	id := r.PathValue("id")
	if provider == "" || id == "" {
		writeError(w, http.StatusBadRequest, "provider and id path parameters are required", "invalid_request_error")
		return
	}

	found, err := h.store.DeleteModelCatalogEntry(ctx, provider, id)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to delete model catalog entry",
			"provider", provider, "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete model catalog entry", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "model catalog entry not found", "not_found_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
