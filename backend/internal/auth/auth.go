package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	tenantKey         contextKey = "tenant"
	tenantIDKey       contextKey = "tenant_id"
	allowedTenantsKey contextKey = "allowed_tenants" // for local_admin: list of tenant IDs allowed to access
	subKey            contextKey = "sub"
	rolesKey          contextKey = "roles"
	authTypeKey       contextKey = "auth_type"
	apiKeyIDKey       contextKey = "api_key_id"
	apiKeyNameKey     contextKey = "api_key_name"
	scopesKey         contextKey = "scopes"
)

// TenantFromContext extracts the tenant config from the request context.
func TenantFromContext(ctx context.Context) *config.TenantConfig {
	t, _ := ctx.Value(tenantKey).(*config.TenantConfig)
	return t
}

// WithTenant attaches a tenant to the context (for testing).
func WithTenant(ctx context.Context, tenant *config.TenantConfig) context.Context {
	return context.WithValue(ctx, tenantKey, tenant)
}

// WithSub attaches a JWT subject to the context (for testing).
func WithSub(ctx context.Context, sub string) context.Context {
	return context.WithValue(ctx, subKey, sub)
}

// WithAuthType attaches an auth type to the context (for testing).
func WithAuthType(ctx context.Context, authType string) context.Context {
	return context.WithValue(ctx, authTypeKey, authType)
}

// WithRoles attaches JWT roles to the context (typed key; must match RolesFromContext).
func WithRoles(ctx context.Context, roles []string) context.Context {
	return context.WithValue(ctx, rolesKey, roles)
}

// WithContextTenantID sets the primary tenant_id claim in context (typed key; must match TenantIDFromContext).
func WithContextTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// WithJWTAdminContext sets context for admin-panel JWT auth: tenant_id, sub, roles, auth_type=jwt.
// Keys must match TenantIDFromContext, SubFromContext, RolesFromContext, AuthTypeFromContext.
func WithJWTAdminContext(ctx context.Context, tenantID, sub string, roles []string) context.Context {
	ctx = context.WithValue(ctx, tenantIDKey, tenantID)
	ctx = context.WithValue(ctx, subKey, sub)
	ctx = context.WithValue(ctx, rolesKey, roles)
	ctx = context.WithValue(ctx, authTypeKey, "jwt")
	return ctx
}

// TenantIDFromContext extracts the tenant ID from context
func TenantIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(tenantIDKey).(string)
	return id
}

// AllowedTenantsFromContext returns the list of tenant IDs the caller is allowed to access (local_admin).
func AllowedTenantsFromContext(ctx context.Context) []string {
	sl, _ := ctx.Value(allowedTenantsKey).([]string)
	return sl
}

// WithAllowedTenants sets the allowed tenants list in context (for local_admin).
func WithAllowedTenants(ctx context.Context, tenants []string) context.Context {
	return context.WithValue(ctx, allowedTenantsKey, tenants)
}

// TenantInRequestAllowed returns true if tenant t is in the allowed list.
func TenantInRequestAllowed(t string, allowed []string) bool {
	for _, a := range allowed {
		if a == t {
			return true
		}
	}
	return false
}

// SubFromContext extracts the JWT subject from context
func SubFromContext(ctx context.Context) string {
	sub, _ := ctx.Value(subKey).(string)
	return sub
}

// RolesFromContext extracts the JWT roles from context
func RolesFromContext(ctx context.Context) []string {
	roles, _ := ctx.Value(rolesKey).([]string)
	return roles
}

// AuthTypeFromContext extracts the auth type ("jwt" or "api_key") from context
func AuthTypeFromContext(ctx context.Context) string {
	authType, _ := ctx.Value(authTypeKey).(string)
	return authType
}

// APIKeyIDFromContext extracts the API key ID from context (for DB-backed keys)
func APIKeyIDFromContext(ctx context.Context) string {
	id, ok := ctx.Value(apiKeyIDKey).(string)
	if !ok {
		return ""
	}
	return id
}

// APIKeyNameFromContext extracts the API key display name from context (for DB-backed keys)
func APIKeyNameFromContext(ctx context.Context) string {
	name, ok := ctx.Value(apiKeyNameKey).(string)
	if !ok {
		return ""
	}
	return name
}

// APIKeyAttributionFromContext returns API key id and name for request_log/usage attribution.
// Returns (nil, nil) when not authenticated via a DB-backed API key (e.g. JWT or YAML key).
func APIKeyAttributionFromContext(ctx context.Context) (*uuid.UUID, *string) {
	idStr := APIKeyIDFromContext(ctx)
	nameStr := APIKeyNameFromContext(ctx)
	if idStr == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(idStr)
	if err != nil {
		return nil, nil
	}
	nameCopy := nameStr
	return &parsed, &nameCopy
}

// JWTSubFromContext returns a pointer to the JWT subject (claims.sub) when present and non-empty.
// Returns nil for API-key-only flows or when sub was not set by auth middleware.
func JWTSubFromContext(ctx context.Context) *string {
	s, ok := ctx.Value(subKey).(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	out := s
	return &out
}

// ErrInferenceJWTNotEnabledInGlobal is returned when active global config does not include JWT auth.
var ErrInferenceJWTNotEnabledInGlobal = errors.New("jwt auth not enabled in active global config")

// resolveInferenceJWTValidator uses the same global auth resolution as AdminMiddleware (cache → DB → YAML)
// and the shared JWTValidatorCache so /v1/* validates tokens with the same issuer/audience/JWKS as /admin/*.
func resolveInferenceJWTValidator(ctx context.Context, cfg *config.Config, globalCfgCache *config.GlobalConfigCache, store config.Storage, log *slog.Logger, jwtCache *JWTValidatorCache) (*JWTValidator, error) {
	if jwtCache == nil {
		return nil, fmt.Errorf("jwt validator cache is nil")
	}
	gc, err := cfg.ResolveGlobalConfig(ctx, globalCfgCache, store, nil, nil, log)
	if err != nil {
		return nil, err
	}

	// DEBUG: Log the resolved JWT config to diagnose issuer mismatches
	log.Error("JWT_CONFIG_RESOLVED",
		"issuer", gc.Auth.JWT.Issuer,
		"issuer_public", gc.Auth.JWT.IssuerPublic,
		"audience", gc.Auth.JWT.Audience,
		"jwks_url", gc.Auth.JWT.JWKSURL,
	)

	if gc.Auth == nil || (gc.Auth.Mode != "jwt" && gc.Auth.Mode != "both") {
		return nil, ErrInferenceJWTNotEnabledInGlobal
	}
	if strings.TrimSpace(gc.Auth.JWT.JWKSURL) == "" {
		return nil, fmt.Errorf("active global config: auth.jwt.jwks_url is empty")
	}
	return jwtCache.GetOrCreate(gc.Auth.JWT), nil
}

// ScopesFromContext extracts the API key scopes from context
func ScopesFromContext(ctx context.Context) []string {
	scopes, _ := ctx.Value(scopesKey).([]string)
	return scopes
}

// Middleware returns an HTTP middleware that validates authentication
// based on the configured mode (api_key, jwt, or both).
// JWT validation for /v1/* uses the same resolved global auth config as AdminMiddleware (not bootstrap-only JWKS).
func Middleware(cfg *config.Config, log *slog.Logger, cache *config.TenantConfigCache, store config.Storage, globalCfgCache *config.GlobalConfigCache, jwtCache *JWTValidatorCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch cfg.Auth.Mode {
			case "jwt":
				handleJWTAuth(w, r, next, cfg, log, cache, store, globalCfgCache, jwtCache)
			case "api_key":
				handleAPIKeyAuth(w, r, next, cfg, log, cache, store)
			case "both":
				handleBothAuth(w, r, next, cfg, log, cache, store, globalCfgCache, jwtCache)
			default:
				writeError(w, http.StatusInternalServerError, "invalid auth mode", "internal_error")
			}
		})
	}
}

// handleJWTAuth validates JWT token and injects claims into context
func handleJWTAuth(w http.ResponseWriter, r *http.Request, next http.Handler, cfg *config.Config, log *slog.Logger, cache *config.TenantConfigCache, store config.Storage, globalCfgCache *config.GlobalConfigCache, jwtCache *JWTValidatorCache) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header", "authentication_error")
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	validator, err := resolveInferenceJWTValidator(r.Context(), cfg, globalCfgCache, store, log, jwtCache)
	if err != nil {
		if errors.Is(err, ErrInferenceJWTNotEnabledInGlobal) {
			log.WarnContext(r.Context(), "jwt auth not enabled in active global config", "error", err)
			writeError(w, http.StatusUnauthorized, "invalid credentials", "authentication_error")
			return
		}
		log.ErrorContext(r.Context(), "failed to resolve jwt validator for inference", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
		return
	}
	claims, err := validator.ValidateToken(r.Context(), tokenString)
	if err != nil {
		log.WarnContext(r.Context(), "jwt validation failed", "error", err)
		writeError(w, http.StatusUnauthorized, "invalid token", "authentication_error")
		return
	}

	// Resolve tenant config (cache → DB → YAML fallback)
	tenant, err := cfg.ResolveTenantConfig(r.Context(), claims.TenantID, cache, store)
	if err != nil {
		log.ErrorContext(r.Context(), "failed to resolve tenant config", "error", err, "tenant_id", claims.TenantID)
		writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
		return
	}
	if tenant == nil {
		log.WarnContext(r.Context(), "tenant not found for JWT", "tenant_id", claims.TenantID)
		writeError(w, http.StatusUnauthorized, "invalid tenant", "authentication_error")
		return
	}

	// Inject context values
	ctx := r.Context()
	ctx = context.WithValue(ctx, tenantKey, tenant)
	ctx = context.WithValue(ctx, tenantIDKey, claims.TenantID)
	ctx = context.WithValue(ctx, subKey, claims.Subject)
	ctx = context.WithValue(ctx, rolesKey, claims.Roles)
	ctx = context.WithValue(ctx, authTypeKey, "jwt")

	// Add OTel span attributes
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		gatewayotel.AttrAuthType("jwt"),
		gatewayotel.AttrTenant(claims.TenantID),
		gatewayotel.AttrSub(claims.Subject),
	)

	log.InfoContext(ctx, "auth success",
		"auth_type", "jwt",
		"tenant_id", claims.TenantID,
		"sub", claims.Subject,
		"roles_count", len(claims.Roles),
	)

	next.ServeHTTP(w, r.WithContext(ctx))
}

// handleAPIKeyAuth validates X-API-Key header with DB-first lookup, YAML fallback
func handleAPIKeyAuth(w http.ResponseWriter, r *http.Request, next http.Handler, cfg *config.Config, log *slog.Logger, cache *config.TenantConfigCache, store config.Storage) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		writeError(w, http.StatusUnauthorized, "missing X-API-Key header", "authentication_error")
		return
	}

	ctx := r.Context()

	// Try DB lookup first (if storage supports it)
	if pgStore, ok := store.(interface {
		LookupAPIKeyByHash(context.Context, string) (storage.APIKeyRecord, bool, error)
		TouchAPIKeyLastUsed(context.Context, uuid.UUID, time.Time) error
	}); ok {
		keyHash := hashAPIKey(apiKey)
		keyRecord, found, err := pgStore.LookupAPIKeyByHash(ctx, keyHash)

		if err != nil {
			log.ErrorContext(ctx, "api key lookup failed", "error", err)
			gatewayotel.APIKeyValidationsCounter.WithLabelValues("error").Inc()
			writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
			return
		}

		if found {
			// Validate not revoked (double-check, should be handled by query)
			if keyRecord.RevokedAt != nil {
				gatewayotel.APIKeyValidationsCounter.WithLabelValues("revoked").Inc()
				writeError(w, http.StatusUnauthorized, "api key revoked", "authentication_error")
				return
			}

			// Validate not expired (double-check, should be handled by query)
			if keyRecord.ExpiresAt != nil && keyRecord.ExpiresAt.Before(time.Now()) {
				gatewayotel.APIKeyValidationsCounter.WithLabelValues("expired").Inc()
				writeError(w, http.StatusUnauthorized, "api key expired", "authentication_error")
				return
			}

			// Resolve tenant config
			tenant, err := cfg.ResolveTenantConfig(ctx, keyRecord.TenantID, cache, store)
			if err != nil {
				log.ErrorContext(ctx, "failed to resolve tenant config", "error", err, "tenant_id", keyRecord.TenantID)
				writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
				return
			}
			if tenant == nil {
				writeError(w, http.StatusUnauthorized, "invalid tenant", "authentication_error")
				return
			}

			// Set context (for API key attribution in request_log and usage)
			ctx = context.WithValue(ctx, tenantKey, tenant)
			ctx = context.WithValue(ctx, tenantIDKey, keyRecord.TenantID)
			ctx = context.WithValue(ctx, apiKeyIDKey, keyRecord.ID.String())
			ctx = context.WithValue(ctx, apiKeyNameKey, keyRecord.Name)
			ctx = context.WithValue(ctx, scopesKey, keyRecord.Scopes)
			ctx = context.WithValue(ctx, authTypeKey, "api_key")

			// Add OTel span attributes
			span := trace.SpanFromContext(ctx)
			span.SetAttributes(
				gatewayotel.AttrAuthType("api_key"),
				gatewayotel.AttrTenant(keyRecord.TenantID),
			)

			// Update last_used_at asynchronously (best-effort, non-blocking)
			go func() {
				_ = pgStore.TouchAPIKeyLastUsed(context.Background(), keyRecord.ID, time.Now())
			}()

			gatewayotel.APIKeyValidationsCounter.WithLabelValues("ok").Inc()

			log.InfoContext(ctx, "auth success",
				"auth_type", "api_key",
				"tenant_id", keyRecord.TenantID,
				"key_id", keyRecord.ID.String(),
				"scopes", keyRecord.Scopes,
			)

			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
	}

	// YAML fallback (existing behavior)
	yamlTenant := cfg.TenantByAPIKey(apiKey)
	if yamlTenant == nil {
		gatewayotel.APIKeyValidationsCounter.WithLabelValues("not_found").Inc()
		writeError(w, http.StatusUnauthorized, "invalid API key", "authentication_error")
		return
	}

	// Resolve full config (may use DB override)
	tenant, err := cfg.ResolveTenantConfig(ctx, yamlTenant.ID, cache, store)
	if err != nil {
		log.ErrorContext(ctx, "failed to resolve tenant config", "error", err, "tenant_id", yamlTenant.ID)
		writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
		return
	}
	if tenant == nil {
		// Fallback to YAML if DB lookup failed
		tenant = yamlTenant
	}

	// YAML keys get implicit "inference" scope
	ctx = context.WithValue(ctx, tenantKey, tenant)
	ctx = context.WithValue(ctx, tenantIDKey, tenant.ID)
	ctx = context.WithValue(ctx, scopesKey, []string{"inference"})
	ctx = context.WithValue(ctx, authTypeKey, "api_key")

	// Add OTel span attribute
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		gatewayotel.AttrAuthType("api_key"),
		gatewayotel.AttrTenant(tenant.ID),
	)

	gatewayotel.APIKeyValidationsCounter.WithLabelValues("ok").Inc()

	log.InfoContext(ctx, "auth success",
		"auth_type", "api_key",
		"tenant_id", tenant.ID,
		"scopes", []string{"inference"},
	)

	next.ServeHTTP(w, r.WithContext(ctx))
}

// handleBothAuth implements the correct evaluation order for mode "both":
// 1. API key lookup (DB hash → YAML exact match) using whichever credential is present.
// 2. JWT validation (only when bearer token was not recognised as an API key).
// 3. 401 only when both mechanisms have been exhausted.
//
// X-API-Key header is handled exclusively via the existing handleAPIKeyAuth path.
// Authorization: Bearer <token> is tried as an API key first, then as a JWT.
func handleBothAuth(w http.ResponseWriter, r *http.Request, next http.Handler, cfg *config.Config, log *slog.Logger, cache *config.TenantConfigCache, store config.Storage, globalCfgCache *config.GlobalConfigCache, jwtCache *JWTValidatorCache) {
	// X-API-Key is unambiguously an API key — delegate directly.
	if r.Header.Get("X-API-Key") != "" {
		handleAPIKeyAuth(w, r, next, cfg, log, cache, store)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeError(w, http.StatusUnauthorized, "missing credentials", "authentication_error")
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	ctx := r.Context()

	// Step 1: try token as a DB-backed API key.
	if pgStore, ok := store.(interface {
		LookupAPIKeyByHash(context.Context, string) (storage.APIKeyRecord, bool, error)
		TouchAPIKeyLastUsed(context.Context, uuid.UUID, time.Time) error
	}); ok {
		keyHash := hashAPIKey(token)
		keyRecord, found, err := pgStore.LookupAPIKeyByHash(ctx, keyHash)
		if err != nil {
			log.ErrorContext(ctx, "api key lookup failed", "error", err)
			gatewayotel.APIKeyValidationsCounter.WithLabelValues("error").Inc()
			writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
			return
		}
		if found {
			if keyRecord.RevokedAt != nil {
				gatewayotel.APIKeyValidationsCounter.WithLabelValues("revoked").Inc()
				writeError(w, http.StatusUnauthorized, "api key revoked", "authentication_error")
				return
			}
			if keyRecord.ExpiresAt != nil && keyRecord.ExpiresAt.Before(time.Now()) {
				gatewayotel.APIKeyValidationsCounter.WithLabelValues("expired").Inc()
				writeError(w, http.StatusUnauthorized, "api key expired", "authentication_error")
				return
			}
			tenant, err := cfg.ResolveTenantConfig(ctx, keyRecord.TenantID, cache, store)
			if err != nil {
				log.ErrorContext(ctx, "failed to resolve tenant config", "error", err, "tenant_id", keyRecord.TenantID)
				writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
				return
			}
			if tenant == nil {
				writeError(w, http.StatusUnauthorized, "invalid tenant", "authentication_error")
				return
			}
			ctx = context.WithValue(ctx, tenantKey, tenant)
			ctx = context.WithValue(ctx, tenantIDKey, keyRecord.TenantID)
			ctx = context.WithValue(ctx, apiKeyIDKey, keyRecord.ID.String())
			ctx = context.WithValue(ctx, apiKeyNameKey, keyRecord.Name)
			ctx = context.WithValue(ctx, scopesKey, keyRecord.Scopes)
			ctx = context.WithValue(ctx, authTypeKey, "api_key")
			span := trace.SpanFromContext(ctx)
			span.SetAttributes(gatewayotel.AttrAuthType("api_key"), gatewayotel.AttrTenant(keyRecord.TenantID))
			go func() { _ = pgStore.TouchAPIKeyLastUsed(context.Background(), keyRecord.ID, time.Now()) }()
			gatewayotel.APIKeyValidationsCounter.WithLabelValues("ok").Inc()
			log.InfoContext(ctx, "auth success", "auth_type", "api_key", "tenant_id", keyRecord.TenantID, "key_id", keyRecord.ID.String(), "scopes", keyRecord.Scopes)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
	}

	// Step 2: try token as a YAML-defined API key (exact string match).
	if yamlTenant := cfg.TenantByAPIKey(token); yamlTenant != nil {
		tenant, err := cfg.ResolveTenantConfig(ctx, yamlTenant.ID, cache, store)
		if err != nil {
			log.ErrorContext(ctx, "failed to resolve tenant config", "error", err, "tenant_id", yamlTenant.ID)
			writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
			return
		}
		if tenant == nil {
			tenant = yamlTenant
		}
		ctx = context.WithValue(ctx, tenantKey, tenant)
		ctx = context.WithValue(ctx, tenantIDKey, tenant.ID)
		ctx = context.WithValue(ctx, scopesKey, []string{"inference"})
		ctx = context.WithValue(ctx, authTypeKey, "api_key")
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(gatewayotel.AttrAuthType("api_key"), gatewayotel.AttrTenant(tenant.ID))
		gatewayotel.APIKeyValidationsCounter.WithLabelValues("ok").Inc()
		log.InfoContext(ctx, "auth success", "auth_type", "api_key", "tenant_id", tenant.ID, "scopes", []string{"inference"})
		next.ServeHTTP(w, r.WithContext(ctx))
		return
	}

	// Step 3: token not recognised as an API key — attempt JWT validation (same global JWKS as admin).
	validator, err := resolveInferenceJWTValidator(ctx, cfg, globalCfgCache, store, log, jwtCache)
	if err != nil {
		if errors.Is(err, ErrInferenceJWTNotEnabledInGlobal) {
			log.WarnContext(ctx, "both auth: jwt not enabled in global config", "error", err)
			gatewayotel.APIKeyValidationsCounter.WithLabelValues("not_found").Inc()
			writeError(w, http.StatusUnauthorized, "invalid credentials", "authentication_error")
			return
		}
		log.ErrorContext(ctx, "both auth: failed to resolve jwt validator", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
		return
	}
	claims, err := validator.ValidateToken(ctx, token)
	if err != nil {
		log.WarnContext(ctx, "both auth failed", "error", err)
		gatewayotel.APIKeyValidationsCounter.WithLabelValues("not_found").Inc()
		writeError(w, http.StatusUnauthorized, "invalid credentials", "authentication_error")
		return
	}
	tenant, err := cfg.ResolveTenantConfig(ctx, claims.TenantID, cache, store)
	if err != nil {
		log.ErrorContext(ctx, "failed to resolve tenant config", "error", err, "tenant_id", claims.TenantID)
		writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
		return
	}
	if tenant == nil {
		log.WarnContext(ctx, "tenant not found for JWT", "tenant_id", claims.TenantID)
		writeError(w, http.StatusUnauthorized, "invalid tenant", "authentication_error")
		return
	}
	ctx = context.WithValue(ctx, tenantKey, tenant)
	ctx = context.WithValue(ctx, tenantIDKey, claims.TenantID)
	ctx = context.WithValue(ctx, subKey, claims.Subject)
	ctx = context.WithValue(ctx, rolesKey, claims.Roles)
	ctx = context.WithValue(ctx, authTypeKey, "jwt")
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(gatewayotel.AttrAuthType("jwt"), gatewayotel.AttrTenant(claims.TenantID), gatewayotel.AttrSub(claims.Subject))
	log.InfoContext(ctx, "auth success", "auth_type", "jwt", "tenant_id", claims.TenantID, "sub", claims.Subject, "roles_count", len(claims.Roles))
	next.ServeHTTP(w, r.WithContext(ctx))
}

// hashAPIKey computes SHA256 hash of plaintext key
func hashAPIKey(plaintext string) string {
	hash := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(hash[:])
}

// writeError writes an OpenAI-compatible error response
func writeError(w http.ResponseWriter, status int, message, errorType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":{"message":"` + message + `","type":"` + errorType + `"}}`))
}

// ScopeMiddleware returns middleware that enforces API key scopes
// Only enforces for API key auth (JWT uses existing RBAC)
func ScopeMiddleware(requiredScopes []string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authType := AuthTypeFromContext(r.Context())

			// Only enforce for API key auth (JWT uses RBAC)
			if authType != "api_key" {
				next.ServeHTTP(w, r)
				return
			}

			scopes := ScopesFromContext(r.Context())

			// Check if any required scope is present
			for _, required := range requiredScopes {
				for _, has := range scopes {
					if has == required {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			log.WarnContext(r.Context(), "insufficient scopes",
				"required", requiredScopes,
				"has", scopes,
				"tenant_id", TenantIDFromContext(r.Context()))

			writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
		})
	}
}

// HandleAPIKeyAuthForAdmin validates X-API-Key for admin endpoints.
// Unlike the regular path, admin-scoped keys (admin_read / admin_write) are allowed
// even when their tenant config cannot be resolved (e.g. the tenant was deleted but
// the key itself still exists). Admin handlers do not use the tenant from context.
func HandleAPIKeyAuthForAdmin(w http.ResponseWriter, r *http.Request, next http.Handler, cfg *config.Config, log *slog.Logger, cache *config.TenantConfigCache, store config.Storage) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		writeError(w, http.StatusUnauthorized, "missing X-API-Key header", "authentication_error")
		return
	}

	ctx := r.Context()

	// DB path: look up the key, check scopes, tolerate nil tenant for admin-scoped keys.
	if pgStore, ok := store.(interface {
		LookupAPIKeyByHash(context.Context, string) (storage.APIKeyRecord, bool, error)
		TouchAPIKeyLastUsed(context.Context, uuid.UUID, time.Time) error
	}); ok {
		keyHash := hashAPIKey(apiKey)
		keyRecord, found, err := pgStore.LookupAPIKeyByHash(ctx, keyHash)
		if err != nil {
			log.ErrorContext(ctx, "api key lookup failed (admin)", "error", err)
			gatewayotel.APIKeyValidationsCounter.WithLabelValues("error").Inc()
			writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
			return
		}

		if found {
			if keyRecord.RevokedAt != nil {
				gatewayotel.APIKeyValidationsCounter.WithLabelValues("revoked").Inc()
				writeError(w, http.StatusUnauthorized, "api key revoked", "authentication_error")
				return
			}
			if keyRecord.ExpiresAt != nil && keyRecord.ExpiresAt.Before(time.Now()) {
				gatewayotel.APIKeyValidationsCounter.WithLabelValues("expired").Inc()
				writeError(w, http.StatusUnauthorized, "api key expired", "authentication_error")
				return
			}

			// Check if the key has admin scopes before requiring tenant resolution.
			hasAdminScope := false
			for _, s := range keyRecord.Scopes {
				if s == "admin_read" || s == "admin_write" {
					hasAdminScope = true
					break
				}
			}

			// Resolve tenant config (best-effort for admin keys).
			tenant, resolveErr := cfg.ResolveTenantConfig(ctx, keyRecord.TenantID, cache, store)
			if resolveErr != nil {
				log.ErrorContext(ctx, "failed to resolve tenant config (admin)", "error", resolveErr, "tenant_id", keyRecord.TenantID)
				// For admin-scoped keys, log the error but continue.
				// For non-admin keys, fail with internal error.
				if !hasAdminScope {
					writeError(w, http.StatusInternalServerError, "internal error", "internal_error")
					return
				}
			}
			if tenant == nil && !hasAdminScope {
				writeError(w, http.StatusUnauthorized, "invalid tenant", "authentication_error")
				return
			}

			// Proceed: tenant may be nil for admin keys (handlers don't use it).
			if tenant != nil {
				ctx = context.WithValue(ctx, tenantKey, tenant)
			}
			ctx = context.WithValue(ctx, tenantIDKey, keyRecord.TenantID)
			ctx = context.WithValue(ctx, apiKeyIDKey, keyRecord.ID.String())
			ctx = context.WithValue(ctx, apiKeyNameKey, keyRecord.Name)
			ctx = context.WithValue(ctx, scopesKey, keyRecord.Scopes)
			ctx = context.WithValue(ctx, authTypeKey, "api_key")

			span := trace.SpanFromContext(ctx)
			span.SetAttributes(
				gatewayotel.AttrAuthType("api_key"),
				gatewayotel.AttrTenant(keyRecord.TenantID),
			)

			go func() {
				_ = pgStore.TouchAPIKeyLastUsed(context.Background(), keyRecord.ID, time.Now())
			}()

			gatewayotel.APIKeyValidationsCounter.WithLabelValues("ok").Inc()
			log.InfoContext(ctx, "admin auth success",
				"auth_type", "api_key",
				"tenant_id", keyRecord.TenantID,
				"key_id", keyRecord.ID.String(),
				"scopes", keyRecord.Scopes,
			)

			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
	}

	// YAML fallback: not applicable for admin keys (no YAML-level admin scopes),
	// but call the regular path for backwards compatibility.
	handleAPIKeyAuth(w, r, next, cfg, log, cache, store)
}
