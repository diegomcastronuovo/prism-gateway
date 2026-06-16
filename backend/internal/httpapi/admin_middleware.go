package httpapi

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// AdminMiddleware validates admin access via:
// 1. X-Admin-Token (super admin, backward compatibility)
// 2. JWT with admin role (uses active global config, not bootstrap)
// 3. X-API-Key with admin_read or admin_write scopes
func AdminMiddleware(cfg *config.Config, cache *config.TenantConfigCache, store storage.Storage, globalCfgCache *config.GlobalConfigCache, jwtValidatorCache *auth.JWTValidatorCache, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Path 1: X-Admin-Token (super admin - bypasses all scope checks)
			adminToken := r.Header.Get("X-Admin-Token")
			expectedToken := os.Getenv("ADMIN_TOKEN")
			if adminToken != "" && expectedToken != "" && subtle.ConstantTimeCompare([]byte(adminToken), []byte(expectedToken)) == 1 {
				ctx := auth.WithAuthType(r.Context(), "admin_token")
				log.InfoContext(ctx, "admin auth success", "auth_type", "admin_token")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Path 2: JWT with admin role — use active global config (cache → DB → YAML), not bootstrap.
			gc, err := cfg.ResolveGlobalConfig(r.Context(), globalCfgCache, store, store, store, log)
			if err != nil {
				log.WarnContext(r.Context(), "admin auth: failed to resolve global config", "error", err)
				writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
				return
			}
			if gc.Auth != nil && (gc.Auth.Mode == "jwt" || gc.Auth.Mode == "both") {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					tokenString := strings.TrimPrefix(authHeader, "Bearer ")
					validator := jwtValidatorCache.GetOrCreate(gc.Auth.JWT)
					claims, err := validator.ValidateTokenForAdmin(r.Context(), tokenString)
					if err != nil {
						log.WarnContext(r.Context(), "admin jwt validation failed", "error", err)
						writeError(w, http.StatusUnauthorized, "invalid token", "authentication_error")
						return
					}
					adminRoles := gc.Auth.JWT.RBAC.AdminRoles
					if len(adminRoles) == 0 {
						adminRoles = gc.Auth.RBAC.AdminRoles
					}
					hasAdminRole := auth.HasAnyRole(claims.Roles, adminRoles)
					hasLocalAdmin := auth.HasAnyRole(claims.Roles, []string{"local_admin"})
					hasUser := auth.HasAnyRole(claims.Roles, []string{"user"})
					if !hasAdminRole && !hasLocalAdmin && !hasUser {
						log.WarnContext(r.Context(), "admin access denied: insufficient roles",
							"sub", claims.Subject,
							"roles", claims.Roles,
							"required_roles", adminRoles,
						)
						writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
						return
					}
					ctx := auth.WithJWTAdminContext(r.Context(), claims.TenantID, claims.Subject, claims.Roles)
					if (hasLocalAdmin || hasUser) && !hasAdminRole {
						// local_admin / user: must have at least one tenant; restrict access to those tenants only
						allowedTenants := auth.TenantsFromClaims(claims)
						if len(allowedTenants) == 0 {
							roleLabel := "local_admin"
							if hasUser && !hasLocalAdmin {
								roleLabel = "user"
							}
							log.WarnContext(ctx, roleLabel+" without tenants", "sub", claims.Subject)
							writeError(w, http.StatusForbidden, roleLabel+" must have at least one tenant assigned", "authorization_error")
							return
						}
						ctx = auth.WithAllowedTenants(ctx, allowedTenants)
						ctx = auth.WithContextTenantID(ctx, allowedTenants[0])
					}
					log.InfoContext(ctx, "admin auth success", "auth_type", "jwt", "sub", claims.Subject)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Path 3: X-API-Key with admin scopes
			apiKey := r.Header.Get("X-API-Key")
			if apiKey != "" {
				// Attempt API key authentication using existing auth logic
				authenticated := false
				var authCtx context.Context

				// Create a response recorder to capture auth result
				authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					authenticated = true
					authCtx = r.Context()
				})

				// Use existing API key auth from auth package
				auth.HandleAPIKeyAuthForAdmin(w, r, authHandler, cfg, log, cache, store)

				if authenticated {
					// Verify API key has admin scopes (admin_read or admin_write)
					scopes := auth.ScopesFromContext(authCtx)
					hasAdminScope := false
					for _, scope := range scopes {
						if scope == "admin_read" || scope == "admin_write" {
							hasAdminScope = true
							break
						}
					}

					if !hasAdminScope {
						log.WarnContext(authCtx, "admin access denied: missing admin scopes",
							"scopes", scopes,
						)
						writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
						return
					}

					log.InfoContext(authCtx, "admin auth success",
						"auth_type", "api_key",
						"scopes", scopes,
					)

					next.ServeHTTP(w, r.WithContext(authCtx))
					return
				}
				// If not authenticated, auth.HandleAPIKeyAuthForAdmin already wrote error
				return
			}

			// No valid credentials provided
			if expectedToken == "" {
				writeError(w, http.StatusForbidden, "admin API disabled", "permission_error")
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid admin credentials", "authentication_error")
		})
	}
}

// AdminScopeMiddleware enforces scope requirements for admin endpoints
// - GET endpoints require admin_read OR admin_write
// - POST/PUT/PATCH/DELETE endpoints require admin_write
// - Only enforces for API key auth (X-Admin-Token and JWT bypass)
func AdminScopeMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authType := auth.AuthTypeFromContext(r.Context())

			// X-Admin-Token and JWT bypass scope checks
			if authType == "admin_token" || authType == "jwt" {
				next.ServeHTTP(w, r)
				return
			}

			// Only enforce for API key auth
			if authType != "api_key" {
				next.ServeHTTP(w, r)
				return
			}

			scopes := auth.ScopesFromContext(r.Context())

			// Determine required scope based on HTTP method
			var requiredScope string
			if r.Method == "GET" || r.Method == "HEAD" {
				// Read operations: admin_read OR admin_write
				hasReadScope := false
				for _, scope := range scopes {
					if scope == "admin_read" || scope == "admin_write" {
						hasReadScope = true
						break
					}
				}
				if hasReadScope {
					next.ServeHTTP(w, r)
					return
				}
				requiredScope = "admin_read or admin_write"
			} else {
				// Write operations: admin_write only
				for _, scope := range scopes {
					if scope == "admin_write" {
						next.ServeHTTP(w, r)
						return
					}
				}
				requiredScope = "admin_write"
			}

			// Insufficient scope
			log.WarnContext(r.Context(), "admin operation denied: insufficient scope",
				"method", r.Method,
				"scopes", scopes,
				"required", requiredScope,
			)
			writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
		})
	}
}

// adminBypassRoles are roles that bypass tenant isolation on admin endpoints (role-based, not auth_type).
var adminBypassRoles = []string{"admin"}

// isWriteMethod returns true for HTTP methods that modify state (user/finance roles are read-only).
func isWriteMethod(method string) bool {
	return method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE"
}

// isConfigEndpoint returns true for admin config surfaces that are denied to non-admin read-only roles.
func isConfigEndpoint(path string) bool {
	return path == "/admin/config" ||
		strings.HasPrefix(path, "/admin/config/") ||
		strings.HasPrefix(path, "/admin/tenants/") && strings.Contains(path, "/config")
}

// AdminOnlyMiddleware restricts access to admin users only.
// Allows: admin_token (X-Admin-Token), API keys with admin scope (already verified upstream),
// and JWT tokens carrying the "admin" role.
// Denies: JWT tokens with non-admin roles (audit, finance, local_admin, user) → 403.
func AdminOnlyMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authType := auth.AuthTypeFromContext(r.Context())
			// X-Admin-Token = super admin; API key already cleared admin scope in AdminMiddleware/AdminScopeMiddleware
			if authType == "admin_token" || authType == "api_key" {
				next.ServeHTTP(w, r)
				return
			}
			// JWT: require admin role
			if auth.HasAnyRole(auth.RolesFromContext(r.Context()), adminBypassRoles) {
				next.ServeHTTP(w, r)
				return
			}
			log.WarnContext(r.Context(), "csv export denied: admin role required",
				"path", r.URL.Path,
				"roles", auth.RolesFromContext(r.Context()),
				"sub", auth.SubFromContext(r.Context()),
			)
			writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
		})
	}
}

// hasAdminWriteAccess is a reusable role check for write/config operations.
// Current contract: admin only.
func hasAdminWriteAccess(roles []string) bool {
	return auth.HasAnyRole(roles, adminBypassRoles)
}


// isTenantScopedGlobalGETAllowed allows GET on paths without {tenant_id} for JWT roles that are
// restricted to allowed_tenants (local_admin and user). Baseline: tenant list + config history
// (handler filters rows for non-bypass roles). Model catalog GET is allowed only for local_admin
// so viewer (user) cannot access global catalog while still matching tenant-scoped reads elsewhere.
// Mutations stay blocked by path (no tenant_id) or by isWriteMethod for role user.
func isTenantScopedGlobalGETAllowed(method, path string, roles []string) bool {
	if method != http.MethodGet {
		return false
	}
	switch path {
	case "/admin/tenants", "/admin/config/history":
		return true
	case "/admin/models":
		return auth.HasAnyRole(roles, []string{"local_admin"})
	default:
		return false
	}
}

// isPlatformControlPlaneRestrictedPath lists admin API areas that only full platform admins may use.
// local_admin and user are tenant operators / viewers — same class of denial as global-only endpoints
// (e.g. benchmarks, route groups catalog, semantic/route management, replay).
func isPlatformControlPlaneRestrictedPath(path string) bool {
	switch {
	case strings.HasPrefix(path, "/admin/route-groups"):
		return true
	case strings.HasPrefix(path, "/admin/benchmarks"):
		return true
	case strings.HasPrefix(path, "/admin/semantic"):
		return true
	case strings.HasPrefix(path, "/admin/observability/semantic"):
		return true
	case strings.HasPrefix(path, "/v1/semantic"):
		return true
	case strings.HasPrefix(path, "/admin/routing"):
		return true
	case strings.HasPrefix(path, "/admin/replay"):
		return true
	case strings.HasPrefix(path, "/admin/traffic/replay"):
		return true
	case strings.HasPrefix(path, "/admin/requests/") && strings.HasSuffix(path, "/routing"):
		return true
	case strings.Contains(path, "/semantic-threshold"):
		return true
	default:
		return false
	}
}

// AdminTenantIsolationMiddleware ensures API key tenant matches URL tenant.
// Bypass is role-based: admin_token (X-Admin-Token) or any JWT with an admin role; do not use auth_type == "jwt".
func AdminTenantIsolationMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			authType := auth.AuthTypeFromContext(ctx)

			// X-Admin-Token: global admin, no roles in context
			if authType == "admin_token" {
				log.DebugContext(ctx, "admin bypass tenant validation", "reason", "admin_token")
				next.ServeHTTP(w, r)
				return
			}

			// Role-based bypass: only admin role(s) get full access; JWT without admin role must respect tenant
			roles := auth.RolesFromContext(ctx)
			if auth.HasAnyRole(roles, adminBypassRoles) {
				log.DebugContext(ctx, "admin bypass tenant validation", "roles", roles)
				next.ServeHTTP(w, r)
				return
			}
			// local_admin / user: platform routing, semantic, route-groups, benchmarks, replay — admin-only surfaces
			if (auth.HasAnyRole(roles, []string{"local_admin"}) || auth.HasAnyRole(roles, []string{"user"})) &&
				isPlatformControlPlaneRestrictedPath(r.URL.Path) {
				log.WarnContext(ctx, "tenant-scoped role cannot access platform control-plane endpoint",
					"path", r.URL.Path,
					"sub", auth.SubFromContext(ctx),
					"roles", roles,
				)
				writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
				return
			}

			// local_admin: only allowed on tenant-scoped paths; path tenant must be in allowed list
			if auth.HasAnyRole(roles, []string{"local_admin"}) {
				allowedTenants := auth.AllowedTenantsFromContext(ctx)
				if len(allowedTenants) == 0 {
					log.WarnContext(ctx, "local_admin without allowed tenants in context")
					writeError(w, http.StatusForbidden, "local_admin must have tenants assigned", "authorization_error")
					return
				}
				pathTenantID := r.PathValue("tenant_id")
				if pathTenantID == "" {
					if isTenantScopedGlobalGETAllowed(r.Method, r.URL.Path, roles) {
						next.ServeHTTP(w, r)
						return
					}
					log.WarnContext(ctx, "local_admin cannot access global endpoints", "sub", auth.SubFromContext(ctx))
					writeError(w, http.StatusForbidden, "local_admin cannot access global endpoints", "authorization_error")
					return
				}
				if !auth.TenantInRequestAllowed(pathTenantID, allowedTenants) {
					log.WarnContext(ctx, "local_admin tenant not allowed",
						"path_tenant", pathTenantID,
						"allowed", allowedTenants,
						"sub", auth.SubFromContext(ctx),
					)
					writeError(w, http.StatusForbidden, "access denied: tenant not in allowed list", "authorization_error")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// user: same tenant isolation as local_admin but read-only (no POST/PUT/PATCH/DELETE)
			if auth.HasAnyRole(roles, []string{"user"}) {
				allowedTenants := auth.AllowedTenantsFromContext(ctx)
				if len(allowedTenants) == 0 {
					log.WarnContext(ctx, "user without allowed tenants in context")
					writeError(w, http.StatusForbidden, "user must have tenants assigned", "authorization_error")
					return
				}
				pathTenantID := r.PathValue("tenant_id")
				if pathTenantID == "" {
					if isTenantScopedGlobalGETAllowed(r.Method, r.URL.Path, roles) {
						next.ServeHTTP(w, r)
						return
					}
					log.WarnContext(ctx, "user cannot access global endpoints", "sub", auth.SubFromContext(ctx))
					writeError(w, http.StatusForbidden, "user cannot access global endpoints", "authorization_error")
					return
				}
				if !auth.TenantInRequestAllowed(pathTenantID, allowedTenants) {
					log.WarnContext(ctx, "user tenant not allowed",
						"path_tenant", pathTenantID,
						"allowed", allowedTenants,
						"sub", auth.SubFromContext(ctx),
					)
					writeError(w, http.StatusForbidden, "access denied: tenant not in allowed list", "authorization_error")
					return
				}
				if isWriteMethod(r.Method) {
					log.WarnContext(ctx, "user cannot perform write operations", "method", r.Method, "sub", auth.SubFromContext(ctx))
					writeError(w, http.StatusForbidden, "user cannot perform write operations", "authorization_error")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// API key auth: enforce tenant match only for non-admin-scoped keys.
			// Keys with admin_read or admin_write have global access (same as admin_token).
			if authType != "api_key" {
				next.ServeHTTP(w, r)
				return
			}

			scopes := auth.ScopesFromContext(r.Context())
			for _, s := range scopes {
				if s == "admin_read" || s == "admin_write" {
					log.DebugContext(ctx, "admin bypass tenant validation", "reason", "admin_scoped_api_key")
					next.ServeHTTP(w, r)
					return
				}
			}

			pathTenantID := r.PathValue("tenant_id")
			if pathTenantID == "" {
				next.ServeHTTP(w, r)
				return
			}

			apiKeyTenantID := auth.TenantIDFromContext(r.Context())
			if apiKeyTenantID != pathTenantID {
				log.WarnContext(r.Context(), "admin tenant isolation violation",
					"api_key_tenant", apiKeyTenantID,
					"url_tenant", pathTenantID,
				)
				writeError(w, http.StatusForbidden, "access denied: tenant mismatch", "authorization_error")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}


// RequireLogsReadAccessMiddleware allows all authenticated admins to read logs.
// In PrismGateway community edition all admin-authenticated users have log read access.
func RequireLogsReadAccessMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}
