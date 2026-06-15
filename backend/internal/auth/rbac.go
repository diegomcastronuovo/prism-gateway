package auth

import (
	"log/slog"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// RBACMiddleware enforces role-based access control for JWT authentication
func RBACMiddleware(cfg *config.Config, log *slog.Logger, requiredRoles []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authType := AuthTypeFromContext(r.Context())

			// Skip RBAC check for API key authentication (backward compat)
			if authType != "jwt" {
				next.ServeHTTP(w, r)
				return
			}

			userRoles := RolesFromContext(r.Context())

			// Check if user has any required role
			if !HasAnyRole(userRoles, requiredRoles) {
				log.WarnContext(r.Context(), "rbac: access denied",
					"user_roles", userRoles,
					"required_roles", requiredRoles,
					"sub", SubFromContext(r.Context()),
				)
				writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
