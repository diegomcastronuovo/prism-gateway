package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// validTenantIDRe validates tenant IDs: start with a letter, then alphanumeric/hyphen/underscore.
var validTenantIDRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

// AdminCreateTenant creates a new tenant with an empty initial configuration.
// POST /admin/tenants
// Body: {"tenant_id": "tenant_b"}
func (h *Handlers) AdminCreateTenant(w http.ResponseWriter, r *http.Request) {
	// Defense in depth: only full admin roles may create tenants.
	// Middleware should already enforce this, but handlers can be called directly in tests.
	roles := auth.RolesFromContext(r.Context())
	if len(roles) > 0 && !auth.HasAnyRole(roles, adminBypassRoles) {
		writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
		return
	}

	var req struct {
		TenantID    string `json:"tenant_id"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", "invalid_request_error")
		return
	}

	if req.TenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required", "invalid_request_error")
		return
	}
	if !validTenantIDRe.MatchString(req.TenantID) {
		writeError(w, http.StatusBadRequest,
			"invalid tenant_id: must start with a letter and contain only letters, digits, hyphens, or underscores (max 64 chars)",
			"invalid_request_error")
		return
	}

	// Normalize and validate environment.
	env, err := normalizeTenantEnvironment(req.Environment)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}

	// Reject if tenant already exists in static YAML config.
	if h.cfg.TenantByID(req.TenantID) != nil {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("tenant %q already exists", req.TenantID),
			"conflict_error")
		return
	}

	actorSub := auth.SubFromContext(r.Context())
	if actorSub == "" {
		actorSub = "admin-token"
	}
	actorRoles := auth.RolesFromContext(r.Context())
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"}
	}

	// Build initial config JSON with environment.
	initialCfg, _ := json.Marshal(map[string]string{"environment": env})

	if err := h.store.CreateTenant(r.Context(), req.TenantID, json.RawMessage(initialCfg), actorSub, actorRoles); err != nil {
		if _, ok := err.(storage.ErrTenantAlreadyExists); ok {
			writeError(w, http.StatusConflict, err.Error(), "conflict_error")
			return
		}
		h.log.ErrorContext(r.Context(), "failed to create tenant", "error", err, "tenant_id", req.TenantID)
		writeError(w, http.StatusInternalServerError, "failed to create tenant", "internal_error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"tenant_id": req.TenantID,
		"version":   1,
		"message":   "Tenant created successfully",
	})
}

// AdminDeleteTenant removes all configuration data for a tenant from the database.
// DELETE /admin/tenants/{tenant_id}
func (h *Handlers) AdminDeleteTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	found, err := h.store.DeleteTenant(r.Context(), tenantID)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to delete tenant", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to delete tenant", "internal_error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound,
			fmt.Sprintf("tenant %q not found", tenantID),
			"not_found")
		return
	}

	// Invalidate cached config so subsequent requests don't serve stale data.
	if h.tenantCache != nil {
		h.tenantCache.Invalidate(tenantID)
	}

	w.WriteHeader(http.StatusNoContent)
}
