package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// AdminPatchSemanticThreshold handles PATCH /admin/tenants/{tenant_id}/semantic-threshold.
//
// Convenience endpoint to set routing.semantic.threshold_default for a tenant without
// requiring a full config PATCH with If-Match-Version. Reads current version, applies a
// JSON Merge Patch for the single field, and invalidates the tenant config cache.
func (h *Handlers) AdminPatchSemanticThreshold(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.PathValue("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "missing tenant_id", "invalid_request_error")
		return
	}

	// Parse and validate request body
	var req struct {
		ThresholdDefault *float64 `json:"threshold_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}
	if req.ThresholdDefault == nil {
		writeError(w, http.StatusBadRequest, "threshold_default is required", "invalid_request_error")
		return
	}
	if *req.ThresholdDefault < 0 || *req.ThresholdDefault > 1 {
		writeError(w, http.StatusBadRequest, "threshold_default must be between 0.0 and 1.0", "invalid_request_error")
		return
	}

	// Ensure tenant config is seeded into DB (idempotent)
	if err := h.ensureTenantConfigInDB(ctx, tenantID); err != nil {
		h.log.ErrorContext(ctx, "failed to ensure tenant config exists",
			"error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to prepare config", "internal_error")
		return
	}

	// Build RFC 7396 JSON Merge Patch targeting only the threshold field.
	// Built once — doesn't depend on the stored config state.
	mergePatch, _ := json.Marshal(map[string]interface{}{
		"routing": map[string]interface{}{
			"semantic": map[string]interface{}{
				"threshold_default": *req.ThresholdDefault,
			},
		},
	})

	// Extract actor
	actorSub := auth.SubFromContext(ctx)
	if actorSub == "" {
		actorSub = "admin-token"
	}
	actorRoles := auth.RolesFromContext(ctx)
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"}
	}

	// Read-then-patch loop: re-reads the current version on ErrVersionConflict so
	// that stale reads (from seeding, cache warm-up, or other concurrent patches)
	// are retried automatically instead of surfacing a spurious 409 to the caller.
	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		_, version, exists, err := h.store.GetTenantConfig(ctx, tenantID)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to read tenant config",
				"error", err, "tenant_id", tenantID)
			writeError(w, http.StatusInternalServerError, "failed to read config", "internal_error")
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "tenant not found", "not_found")
			return
		}

		_, err = h.store.PatchTenantConfig(ctx, tenantID, version, json.RawMessage(mergePatch), actorSub, actorRoles)
		if err != nil {
			if _, ok := err.(storage.ErrVersionConflict); ok {
				lastErr = err
				continue // stale version — re-read and retry
			}
			h.log.ErrorContext(ctx, "failed to patch semantic threshold",
				"error", err, "tenant_id", tenantID)
			writeError(w, http.StatusInternalServerError, "failed to update threshold", "internal_error")
			return
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		writeError(w, http.StatusConflict, "concurrent update detected, please retry", "version_conflict_error")
		return
	}

	// Invalidate cache so the new threshold applies immediately
	if h.tenantCache != nil {
		h.tenantCache.Invalidate(tenantID)
	}

	h.log.InfoContext(ctx, "semantic threshold updated",
		"tenant_id", tenantID,
		"threshold_default", *req.ThresholdDefault)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":            "updated",
		"tenant_id":         tenantID,
		"threshold_default": *req.ThresholdDefault,
	})
}
