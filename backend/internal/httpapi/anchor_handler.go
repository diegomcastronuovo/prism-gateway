package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// anchorRequest is the JSON body for POST /v1/semantic/anchors.
type anchorRequest struct {
	Name            string   `json:"name"`
	Modality        string   `json:"modality"`         // "text" (default) | "image"
	Text            string   `json:"text"`
	ImageURL        string   `json:"image_url"`
	RouteGroup      string   `json:"route_group"`
	PreferredModels []string `json:"preferred_models"`
	AnchorText      *string  `json:"anchor_text,omitempty"`
}

// anchorResponse is the JSON body returned on success.
type anchorResponse struct {
	Status     string `json:"status"`
	Name       string `json:"name"`
	RouteGroup string `json:"route_group"`
	Modality   string `json:"modality"`
}

// CreateSemanticAnchor handles POST /v1/semantic/anchors.
//
// Flow:
//  1. Resolve tenant: API-key auth sets TenantFromContext; JWT admin supplies tenant_id and config is loaded via ResolveTenantConfig.
//  2. Validate required fields (name, text, route_group).
//  3. Resolve the first configured embedding model from global config.
//  4. Generate an embedding for the supplied text via the provider registry.
//  5. Insert the anchor into semantic_anchors (anchor_text from text or explicit anchor_text; 409 on duplicate name per tenant).
func (h *Handlers) CreateSemanticAnchor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	var req anchorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "invalid_request_error")
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
	if req.RouteGroup == "" {
		writeError(w, http.StatusBadRequest, "route_group is required", "invalid_request_error")
		return
	}

	// Resolve the embedding model for the given modality.
	embModel, embErr := h.embeddingModelForModality(ctx, tenant, req.Modality)
	if embModel == nil {
		msg := "no embedding model configured for modality: " + req.Modality
		if embErr != nil {
			msg = embErr.Error()
		}
		writeError(w, http.StatusBadRequest, msg, "invalid_request_error")
		return
	}

	// Resolve embedding provider — reuse registry, same path as /v1/embeddings.
	var ep providers.EmbeddingProvider
	if embModel.Mock.Enabled {
		ep = providers.NewMockProvider(embModel.Mock, embModel.Name, tenant.ID, embModel.Pricing, nil)
	} else {
		var ok bool
		ep, ok = h.embeddingProviderFor(ctx, embModel.Provider)
		if !ok {
			writeError(w, http.StatusInternalServerError, "no embedding model configured for modality: "+req.Modality, "internal_error")
			return
		}
	}

	// Select embedding input based on modality.
	var embInput string
	switch req.Modality {
	case "image":
		embInput = req.ImageURL
	default:
		embInput = req.Text
	}

	// Generate the embedding for the anchor content.
	embResp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{
		Input: []string{embInput},
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

	embedding := embResp.Data[0].Embedding

	// Log internal embedding usage (anchor creation is a standalone op — uses its own requestID).
	h.logInternalEmbeddingAsync(ctx, uuid.New().String(), tenant, embModel,
		embResp.Usage.PromptTokens, len(embInput))

	// Persist semantic_anchors.anchor_text: explicit JSON anchor_text wins; else use text for text modality.
	anchorText := req.AnchorText
	if anchorText == nil && req.Modality == "text" {
		t := req.Text
		anchorText = &t
	}

	// Insert anchor into semantic_anchors.
	err = h.store.UpsertSemanticAnchor(ctx, tenant.ID, req.Name, embedding, req.RouteGroup, req.PreferredModels, anchorText, req.Modality)
	if err != nil {
		if errors.Is(err, storage.ErrAnchorAlreadyExists) {
			writeError(w, http.StatusConflict, "anchor already exists", "conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to store semantic anchor",
			"tenant", tenant.ID, "name", req.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store anchor", "internal_error")
		return
	}

	writeJSON(w, http.StatusOK, anchorResponse{
		Status:     "created",
		Name:       req.Name,
		RouteGroup: req.RouteGroup,
		Modality:   req.Modality,
	})
}
