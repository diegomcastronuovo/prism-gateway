package auth

import (
	"log/slog"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// tenantBypassRoles are JWT roles that may access any tenant (no tenant_id match required).
var tenantBypassRoles = []string{"admin"}

// TenantIsolationMiddleware enforces tenant isolation for admin endpoints
// Ensures JWT tenant_id matches path {tenant_id} unless user has full admin role.
func TenantIsolationMiddleware(cfg *config.Config, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			authType := AuthTypeFromContext(ctx)

			// Skip for API key authentication
			if authType != "jwt" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract {tenant_id} from path
			pathTenantID := r.PathValue("tenant_id")
			if pathTenantID == "" {
				// No tenant in path, allow (might be global endpoint)
				next.ServeHTTP(w, r)
				return
			}

			roles := RolesFromContext(ctx)

			// Full admin: any path tenant
			if HasAnyRole(roles, tenantBypassRoles) {
				next.ServeHTTP(w, r)
				return
			}

			// local_admin / user: path tenant must be in allowed list (not only primary ctx tenant_id)
			if allowed := AllowedTenantsFromContext(ctx); len(allowed) > 0 {
				if TenantInRequestAllowed(pathTenantID, allowed) {
					next.ServeHTTP(w, r)
					return
				}
				log.WarnContext(ctx, "tenant isolation violation",
					"path_tenant", pathTenantID,
					"allowed_tenants", allowed,
					"sub", SubFromContext(ctx),
				)
				writeError(w, http.StatusForbidden, "access denied: tenant not in allowed list", "authorization_error")
				return
			}

			jwtTenantID := TenantIDFromContext(ctx)
			if jwtTenantID == pathTenantID {
				next.ServeHTTP(w, r)
				return
			}

			log.WarnContext(ctx, "tenant isolation violation",
				"jwt_tenant", jwtTenantID,
				"path_tenant", pathTenantID,
				"sub", SubFromContext(ctx),
			)
			writeError(w, http.StatusForbidden, "access denied: tenant mismatch", "authorization_error")
		})
	}
}
