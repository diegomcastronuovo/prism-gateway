package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/google/uuid"
)

// ============================================================================
// Admin API Key Handlers
// ============================================================================

// AdminCreateAPIKey handles POST /admin/tenants/{tenant_id}/api-keys
func (h *Handlers) AdminCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing tenant_id", "invalid_request")
		return
	}

	// Parse request
	var req struct {
		Name      string    `json:"name"`
		Scopes    []string  `json:"scopes"`
		ExpiresAt *string   `json:"expires_at,omitempty"` // ISO8601 format
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON", "invalid_request")
		return
	}

	// Validate request
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required", "invalid_request")
		return
	}

	if len(req.Scopes) == 0 {
		writeJSONError(w, http.StatusBadRequest, "scopes are required", "invalid_request")
		return
	}

	// Validate scopes
	validScopes := map[string]bool{
		"inference":   true,
		"admin_read":  true,
		"admin_write": true,
	}
	for _, scope := range req.Scopes {
		if !validScopes[scope] {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid scope: %s (valid: inference, admin_read, admin_write)", scope), "invalid_request")
			return
		}
	}

	// Parse expires_at
	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid expires_at format (use RFC3339/ISO8601)", "invalid_request")
			return
		}
		if t.Before(time.Now()) {
			writeJSONError(w, http.StatusBadRequest, "expires_at must be in the future", "invalid_request")
			return
		}
		expiresAt = &t
	}

	// Get actor info from context (JWT claims or admin token)
	actorSub := auth.SubFromContext(r.Context())
	if actorSub == "" {
		actorSub = "admin" // Fallback for X-Admin-Token
	}
	actorRoles := auth.RolesFromContext(r.Context())
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"} // Fallback for X-Admin-Token
	}

	// Create API key
	result, err := h.store.CreateAPIKey(r.Context(), tenantID, req.Name, req.Scopes, expiresAt, actorSub, actorRoles)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to create api key",
			"error", err,
			"tenant_id", tenantID,
			"name", req.Name)
		writeJSONError(w, http.StatusInternalServerError, "failed to create api key", "internal_error")
		return
	}

	// Record metric
	gatewayotel.APIKeyAdminActionsCounter.WithLabelValues("create").Inc()

	h.log.InfoContext(r.Context(), "api key created",
		"tenant_id", tenantID,
		"key_id", result.ID.String(),
		"name", req.Name,
		"prefix", result.Prefix,
		"scopes", req.Scopes)

	// Return 201 with plaintext key (ONLY time it's returned)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// apiKeyListItem is the per-item response shape for GET /admin/tenants/{tenant_id}/api-keys.
// Only the fields defined in the spec are included — never plaintext key or key_hash.
type apiKeyListItem struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// apiKeyListResponse is the top-level response shape for GET /admin/tenants/{tenant_id}/api-keys.
type apiKeyListResponse struct {
	Object   string           `json:"object"`
	TenantID string           `json:"tenant_id"`
	Data     []apiKeyListItem `json:"data"`
	HasMore  bool             `json:"has_more"`
}

// AdminListAPIKeys handles GET /admin/tenants/{tenant_id}/api-keys
func (h *Handlers) AdminListAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing tenant_id", "invalid_request")
		return
	}

	q := r.URL.Query()

	// include_revoked (default: false)
	includeRevoked := false
	if v := q.Get("include_revoked"); v == "true" || v == "1" {
		includeRevoked = true
	}

	// limit (default: 50, max: 200)
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeJSONError(w, http.StatusBadRequest, "limit must be an integer between 1 and 200", "invalid_request")
			return
		}
		limit = n
	}

	// offset (default: 0)
	offset := 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeJSONError(w, http.StatusBadRequest, "offset must be a non-negative integer", "invalid_request")
			return
		}
		offset = n
	}

	keys, hasMore, err := h.store.ListAPIKeysPaged(r.Context(), tenantID, includeRevoked, limit, offset)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list api keys",
			"error", err,
			"tenant_id", tenantID)
		writeJSONError(w, http.StatusInternalServerError, "failed to list api keys", "internal_error")
		return
	}

	h.log.InfoContext(r.Context(), "admin list api keys",
		"tenant_id", tenantID,
		"count", len(keys),
		"include_revoked", includeRevoked)

	// Map to spec fields — never expose key_hash or plaintext key
	items := make([]apiKeyListItem, len(keys))
	for i, k := range keys {
		items[i] = apiKeyListItem{
			ID:         k.ID,
			Name:       k.Name,
			Prefix:     k.Prefix,
			Scopes:     k.Scopes,
			CreatedAt:  k.CreatedAt,
			LastUsedAt: k.LastUsedAt,
			RevokedAt:  k.RevokedAt,
		}
	}

	writeJSON(w, http.StatusOK, apiKeyListResponse{
		Object:   "list",
		TenantID: tenantID,
		Data:     items,
		HasMore:  hasMore,
	})
}

// AdminRevokeAPIKey handles POST /admin/tenants/{tenant_id}/api-keys/{id}/revoke
func (h *Handlers) AdminRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing tenant_id", "invalid_request")
		return
	}

	keyIDStr := r.PathValue("id")
	if keyIDStr == "" {
		writeJSONError(w, http.StatusBadRequest, "missing api key id", "invalid_request")
		return
	}

	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid api key id format", "invalid_request")
		return
	}

	// Get actor info
	actorSub := auth.SubFromContext(r.Context())
	if actorSub == "" {
		actorSub = "admin"
	}
	actorRoles := auth.RolesFromContext(r.Context())
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"}
	}

	// Revoke key
	revokedAt, err := h.store.RevokeAPIKey(r.Context(), tenantID, keyID, actorSub, actorRoles)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to revoke api key",
			"error", err,
			"tenant_id", tenantID,
			"key_id", keyID.String())

		// Check for not found
		if err.Error() == "api key not found" || err.Error() == "api key already revoked or not found" {
			writeJSONError(w, http.StatusNotFound, err.Error(), "not_found")
			return
		}

		writeJSONError(w, http.StatusInternalServerError, "failed to revoke api key", "internal_error")
		return
	}

	// Record metric
	gatewayotel.APIKeyAdminActionsCounter.WithLabelValues("revoke").Inc()

	h.log.InfoContext(r.Context(), "api key revoked",
		"tenant_id", tenantID,
		"key_id", keyID.String())

	// Return 200 with revocation info
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         keyID.String(),
		"revoked_at": revokedAt,
	})
}

// AdminRotateAPIKey handles POST /admin/tenants/{tenant_id}/api-keys/{id}/rotate
func (h *Handlers) AdminRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing tenant_id", "invalid_request")
		return
	}

	keyIDStr := r.PathValue("id")
	if keyIDStr == "" {
		writeJSONError(w, http.StatusBadRequest, "missing api key id", "invalid_request")
		return
	}

	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid api key id format", "invalid_request")
		return
	}

	// Get actor info
	actorSub := auth.SubFromContext(r.Context())
	if actorSub == "" {
		actorSub = "admin"
	}
	actorRoles := auth.RolesFromContext(r.Context())
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"}
	}

	// Rotate key
	oldKeyID, newKey, err := h.store.RotateAPIKey(r.Context(), tenantID, keyID, actorSub, actorRoles)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to rotate api key",
			"error", err,
			"tenant_id", tenantID,
			"key_id", keyID.String())

		// Check for not found
		if err.Error() == "api key not found or already revoked" {
			writeJSONError(w, http.StatusNotFound, err.Error(), "not_found")
			return
		}

		writeJSONError(w, http.StatusInternalServerError, "failed to rotate api key", "internal_error")
		return
	}

	// Record metric
	gatewayotel.APIKeyAdminActionsCounter.WithLabelValues("rotate").Inc()

	h.log.InfoContext(r.Context(), "api key rotated",
		"tenant_id", tenantID,
		"old_key_id", oldKeyID.String(),
		"new_key_id", newKey.ID.String(),
		"new_prefix", newKey.Prefix)

	// Return 201 with new key (includes plaintext ONLY once)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"old_id": oldKeyID.String(),
		"new":    newKey,
	})
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, status int, message, errorType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errorType,
		},
	})
}
