package httpapi

import (
	"net/http"
	"strconv"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// anchorListResponse is the JSON envelope for GET /v1/semantic/anchors.
type anchorListResponse struct {
	Object   string               `json:"object"`
	TenantID string               `json:"tenant_id"`
	Data     []anchorMetaResponse `json:"data"`
	HasMore  bool                 `json:"has_more"`
}

// anchorMetaResponse is one entry in the list response.
type anchorMetaResponse struct {
	Name            string   `json:"name"`
	RouteGroup      string   `json:"route_group"`
	PreferredModels []string `json:"preferred_models"`
	AnchorText      *string  `json:"anchor_text,omitempty"`
	VectorDims      int      `json:"vector_dims"`
}

// ListSemanticAnchors handles GET /v1/semantic/anchors.
//
// Query params:
//   - limit  (int, default 50, min 1, max 200)
//   - offset (int, default 0, min 0)
//   - include_anchor_text (bool, default false)
func (h *Handlers) ListSemanticAnchors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	// Parse limit
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200", "invalid_request_error")
			return
		}
		limit = v
	}

	// Parse offset
	offset := 0
	if s := r.URL.Query().Get("offset"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "offset must be >= 0", "invalid_request_error")
			return
		}
		offset = v
	}

	// Parse include_anchor_text
	includeAnchorText := false
	if s := r.URL.Query().Get("include_anchor_text"); s != "" {
		v, err := strconv.ParseBool(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "include_anchor_text must be a boolean", "invalid_request_error")
			return
		}
		includeAnchorText = v
	}

	anchors, hasMore, err := h.store.ListSemanticAnchorsPaged(ctx, tenant.ID, includeAnchorText, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list semantic anchors", "tenant", tenant.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list anchors", "internal_error")
		return
	}

	data := make([]anchorMetaResponse, 0, len(anchors))
	for _, a := range anchors {
		data = append(data, toAnchorMetaResponse(a))
	}

	writeJSON(w, http.StatusOK, anchorListResponse{
		Object:   "list",
		TenantID: tenant.ID,
		Data:     data,
		HasMore:  hasMore,
	})
}

func toAnchorMetaResponse(m storage.SemanticAnchorMeta) anchorMetaResponse {
	models := m.PreferredModels
	if models == nil {
		models = []string{}
	}
	return anchorMetaResponse{
		Name:            m.Name,
		RouteGroup:      m.RouteGroup,
		PreferredModels: models,
		AnchorText:      m.AnchorText,
		VectorDims:      m.VectorDims,
	}
}
