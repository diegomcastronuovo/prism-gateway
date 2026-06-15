package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// toolCatalogRequest is the body for POST /admin/tool-catalog and
// PUT /admin/tool-catalog/{provider}/{id}.
type toolCatalogRequest struct {
	ID           string   `json:"id"`
	Provider     string   `json:"provider"`
	DisplayName  string   `json:"display_name"`
	ToolType     string   `json:"tool_type"`
	Unit         string   `json:"unit"`
	PricePerUnit float64  `json:"price_per_unit"`
	IsActive     *bool    `json:"is_active"`
}

type toolCatalogListResponse struct {
	Data  []storage.ToolCatalogEntry `json:"data"`
	Total int                        `json:"total"`
}

var validToolUnits = map[string]struct{}{
	"call":    {},
	"session": {},
	"gb_day":  {},
}

// AdminListToolCatalog handles GET /admin/tool-catalog.
//
// Optional query params: provider, is_active (true|false).
func (h *Handlers) AdminListToolCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var filter storage.ToolCatalogFilter
	if v := r.URL.Query().Get("provider"); v != "" {
		filter.Provider = &v
	}
	if v := r.URL.Query().Get("is_active"); v != "" {
		active := strings.EqualFold(v, "true")
		filter.IsActive = &active
	}

	entries, err := h.store.ListToolCatalog(ctx, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list tool catalog", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tool catalog", "internal_error")
		return
	}

	writeJSON(w, http.StatusOK, toolCatalogListResponse{
		Data:  entries,
		Total: len(entries),
	})
}

// AdminCreateToolCatalogEntry handles POST /admin/tool-catalog.
func (h *Handlers) AdminCreateToolCatalogEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req toolCatalogRequest
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
	if req.Unit != "" {
		if _, ok := validToolUnits[req.Unit]; !ok {
			writeError(w, http.StatusBadRequest, "unit must be one of: call, session, gb_day", "invalid_request_error")
			return
		}
	}
	if req.PricePerUnit < 0 {
		writeError(w, http.StatusBadRequest, "price_per_unit must be non-negative", "invalid_request_error")
		return
	}

	unit := req.Unit
	if unit == "" {
		unit = "call"
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	entry := storage.ToolCatalogEntry{
		ID:           strings.TrimSpace(req.ID),
		Provider:     strings.TrimSpace(req.Provider),
		DisplayName:  req.DisplayName,
		ToolType:     req.ToolType,
		Unit:         unit,
		PricePerUnit: req.PricePerUnit,
		IsActive:     isActive,
	}

	if err := h.store.CreateToolCatalogEntry(ctx, entry); err != nil {
		if errors.Is(err, storage.ErrToolAlreadyExists) {
			writeError(w, http.StatusConflict, "tool catalog entry already exists", "conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to create tool catalog entry",
			"provider", req.Provider, "id", req.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create tool catalog entry", "internal_error")
		return
	}

	// Return the created entry by fetching it back.
	created, err := h.store.GetToolCatalogEntry(ctx, entry.Provider, entry.ID)
	if err != nil || created == nil {
		// Fall back to echoing back the submitted entry.
		writeJSON(w, http.StatusCreated, entry)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// AdminUpdateToolCatalogEntry handles PUT /admin/tool-catalog/{provider}/{id}.
func (h *Handlers) AdminUpdateToolCatalogEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	provider := r.PathValue("provider")
	id := r.PathValue("id")
	if provider == "" || id == "" {
		writeError(w, http.StatusBadRequest, "provider and id path parameters are required", "invalid_request_error")
		return
	}

	var req toolCatalogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	if req.Unit != "" {
		if _, ok := validToolUnits[req.Unit]; !ok {
			writeError(w, http.StatusBadRequest, "unit must be one of: call, session, gb_day", "invalid_request_error")
			return
		}
	}
	if req.PricePerUnit < 0 {
		writeError(w, http.StatusBadRequest, "price_per_unit must be non-negative", "invalid_request_error")
		return
	}

	unit := req.Unit
	if unit == "" {
		unit = "call"
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	entry := storage.ToolCatalogEntry{
		DisplayName:  req.DisplayName,
		ToolType:     req.ToolType,
		Unit:         unit,
		PricePerUnit: req.PricePerUnit,
		IsActive:     isActive,
	}

	if err := h.store.UpdateToolCatalogEntry(ctx, provider, id, entry); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "tool catalog entry not found", "not_found_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to update tool catalog entry",
			"provider", provider, "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update tool catalog entry", "internal_error")
		return
	}

	updated, err := h.store.GetToolCatalogEntry(ctx, provider, id)
	if err != nil || updated == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// AdminDeleteToolCatalogEntry handles DELETE /admin/tool-catalog/{provider}/{id}.
func (h *Handlers) AdminDeleteToolCatalogEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	provider := r.PathValue("provider")
	id := r.PathValue("id")
	if provider == "" || id == "" {
		writeError(w, http.StatusBadRequest, "provider and id path parameters are required", "invalid_request_error")
		return
	}

	if err := h.store.DeleteToolCatalogEntry(ctx, provider, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "tool catalog entry not found", "not_found_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to delete tool catalog entry",
			"provider", provider, "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete tool catalog entry", "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
