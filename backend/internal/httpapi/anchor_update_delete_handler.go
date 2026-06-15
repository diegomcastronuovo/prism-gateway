package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// patchAnchorRequest is the JSON body for PATCH /v1/semantic/anchors/{name}.
// Only non-nil fields are applied.
type patchAnchorRequest struct {
	RouteGroup      *string   `json:"route_group"`
	PreferredModels *[]string `json:"preferred_models"`
	AnchorText      *string   `json:"anchor_text"`
}

// PatchSemanticAnchor handles PATCH /v1/semantic/anchors/{name}.
//
// Updates one or more fields of an existing anchor. If anchor_text is supplied
// and non-empty, a new embedding is generated and also stored.
func (h *Handlers) PatchSemanticAnchor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "anchor name is required", "invalid_request_error")
		return
	}

	var req patchAnchorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	// At least one field must be present.
	if req.RouteGroup == nil && req.PreferredModels == nil && req.AnchorText == nil {
		writeError(w, http.StatusBadRequest, "at least one field must be provided", "invalid_request_error")
		return
	}

	// Validate preferred_models is not an empty slice when provided.
	if req.PreferredModels != nil && len(*req.PreferredModels) == 0 {
		writeError(w, http.StatusBadRequest, "preferred_models must not be empty when provided", "invalid_request_error")
		return
	}

	patch := storage.SemanticAnchorPatch{
		RouteGroup:      req.RouteGroup,
		PreferredModels: req.PreferredModels,
		AnchorText:      req.AnchorText,
	}

	// If anchor_text is non-empty, re-generate embedding.
	if req.AnchorText != nil && *req.AnchorText != "" {
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

		embResp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{
			Input: []string{*req.AnchorText},
			Model: embModel.Name,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to generate embedding: "+err.Error(), "upstream_error")
			return
		}
		if len(embResp.Data) == 0 || len(embResp.Data[0].Embedding) == 0 {
			writeError(w, http.StatusBadGateway, "no embedding data returned", "upstream_error")
			return
		}
		emb := embResp.Data[0].Embedding
		patch.Embedding = &emb
	}

	found, err := h.store.UpdateSemanticAnchor(ctx, tenant.ID, name, patch)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to update semantic anchor", "tenant", tenant.ID, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update anchor", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "anchor not found", "not_found_error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "updated",
		"name":      name,
		"tenant_id": tenant.ID,
	})
}

// DeleteSemanticAnchor handles DELETE /v1/semantic/anchors/{name}.
func (h *Handlers) DeleteSemanticAnchor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "anchor name is required", "invalid_request_error")
		return
	}

	found, err := h.store.DeleteSemanticAnchor(ctx, tenant.ID, name)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to delete semantic anchor", "tenant", tenant.ID, "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete anchor", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "anchor not found", "not_found_error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "deleted",
		"name":      name,
		"tenant_id": tenant.ID,
	})
}
