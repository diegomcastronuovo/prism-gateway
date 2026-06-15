package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims we extract
type Claims struct {
	TenantID string   `json:"tenant_id"`
	Tenants  []string `json:"tenants,omitempty"` // optional; used by local_admin (fallback: TenantID)
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

// JWTValidator validates JWT tokens using JWKS
type JWTValidator struct {
	issuer        string // Primary issuer (e.g., internal URL in Docker)
	issuerPublic  string // Fallback issuer (e.g., public URL) - optional
	audience      string
	clockSkew     time.Duration
	cache         *jwksCache
	log           *slog.Logger
	claimKeys     map[string]string // Maps "tenant_id" -> actual claim name
}

// jwksCache caches JWKS keys with TTL
type jwksCache struct {
	mu         sync.RWMutex
	keys       map[string]*rsa.PublicKey
	lastFetch  time.Time
	ttl        time.Duration
	jwksURL    string
	httpClient *http.Client
}

// jwksResponse represents the JWKS JSON structure
type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

type jwksKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// NewJWTValidator creates a new JWT validator
func NewJWTValidator(cfg config.JWTConfig, log *slog.Logger) *JWTValidator {
	return &JWTValidator{
		issuer:       cfg.Issuer,
		issuerPublic: cfg.IssuerPublic,
		audience:     cfg.Audience,
		clockSkew:    time.Duration(cfg.ClockSkewSeconds) * time.Second,
		cache: &jwksCache{
			keys:       make(map[string]*rsa.PublicKey),
			ttl:        time.Duration(cfg.CacheTTLMinutes) * time.Minute,
			jwksURL:    cfg.JWKSURL,
			httpClient: &http.Client{Timeout: 10 * time.Second},
		},
		log:       log,
		claimKeys: cfg.RequiredClaims,
	}
}

// ValidateToken validates a JWT token and returns claims
func (v *JWTValidator) ValidateToken(ctx context.Context, tokenString string) (*Claims, error) {
	// Parse token with validation
	token, err := jwt.ParseWithClaims(tokenString, &jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Get key ID from header
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}

		// Get public key from JWKS cache
		pubKey, err := v.cache.getKey(ctx, kid)
		if err != nil {
			return nil, fmt.Errorf("failed to get key: %w", err)
		}

		return pubKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	mapClaims, ok := token.Claims.(*jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate issuer - accept both primary and public issuer (for multi-URL scenarios like Docker)
	iss, err := mapClaims.GetIssuer()
	if err != nil {
		return nil, fmt.Errorf("invalid issuer claim: %w", err)
	}

	validIssuer := iss == v.issuer || (v.issuerPublic != "" && iss == v.issuerPublic)
	if !validIssuer {
		v.log.Error("JWT_ISSUER_VALIDATION_FAILED",
			"token_issuer", iss,
			"expected_issuer", v.issuer,
			"expected_fallback", v.issuerPublic,
			"match_primary", iss == v.issuer,
			"match_fallback", v.issuerPublic != "" && iss == v.issuerPublic,
		)
		return nil, fmt.Errorf("invalid token issuer. Expected '%s'%s, got '%s'", v.issuer,
			func() string { if v.issuerPublic != "" { return " or '" + v.issuerPublic + "'" } ; return "" }(), iss)
	}

	// Validate audience
	aud, err := mapClaims.GetAudience()
	if err != nil {
		return nil, fmt.Errorf("invalid audience claim: %w", err)
	}
	validAud := false
	for _, a := range aud {
		if a == v.audience {
			validAud = true
			break
		}
	}
	if !validAud {
		return nil, fmt.Errorf("invalid audience: expected %s", v.audience)
	}

	// Validate expiration with clock skew
	exp, err := mapClaims.GetExpirationTime()
	if err != nil {
		return nil, fmt.Errorf("invalid exp claim: %w", err)
	}
	if time.Now().Add(v.clockSkew).After(exp.Time) {
		return nil, fmt.Errorf("token expired")
	}

	// Extract custom claims
	claims := &Claims{}

	// Get subject
	sub, err := mapClaims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("invalid sub claim: %w", err)
	}
	claims.Subject = sub

	// Get tenant_id
	tenantIDKey := v.claimKeys["tenant_id"]
	tenantID, ok := (*mapClaims)[tenantIDKey].(string)
	if !ok || tenantID == "" {
		return nil, fmt.Errorf("missing or invalid %s claim", tenantIDKey)
	}
	claims.TenantID = tenantID

	// Get roles (handle multiple formats)
	rolesKey := v.claimKeys["roles"]
	if rolesRaw, ok := (*mapClaims)[rolesKey]; ok {
		claims.Roles = normalizeRoles(rolesRaw)
	}

	// Copy registered claims
	claims.Issuer = iss
	claims.Audience = aud
	claims.ExpiresAt = exp

	return claims, nil
}

// ValidateTokenForAdmin validates a JWT for admin use: same as ValidateToken but tenant_id is optional.
// Admin users are not bound to a tenant; if the token omits tenant_id or it is empty, TenantID is set to "".
func (v *JWTValidator) ValidateTokenForAdmin(ctx context.Context, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}
		pubKey, err := v.cache.getKey(ctx, kid)
		if err != nil {
			return nil, fmt.Errorf("failed to get key: %w", err)
		}
		return pubKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	mapClaims, ok := token.Claims.(*jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	iss, err := mapClaims.GetIssuer()
	if err != nil {
		return nil, fmt.Errorf("invalid issuer claim: %w", err)
	}
	validIssuer := iss == v.issuer || (v.issuerPublic != "" && iss == v.issuerPublic)
	if !validIssuer {
		return nil, fmt.Errorf("invalid token issuer. Expected '%s'%s, got '%s'", v.issuer,
			func() string { if v.issuerPublic != "" { return " or '" + v.issuerPublic + "'" } ; return "" }(), iss)
	}
	aud, err := mapClaims.GetAudience()
	if err != nil {
		return nil, fmt.Errorf("invalid audience claim: %w", err)
	}
	validAud := false
	for _, a := range aud {
		if a == v.audience {
			validAud = true
			break
		}
	}
	if !validAud {
		return nil, fmt.Errorf("invalid audience: expected %s", v.audience)
	}
	exp, err := mapClaims.GetExpirationTime()
	if err != nil {
		return nil, fmt.Errorf("invalid exp claim: %w", err)
	}
	if time.Now().Add(v.clockSkew).After(exp.Time) {
		return nil, fmt.Errorf("token expired")
	}
	claims := &Claims{}
	sub, err := mapClaims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("invalid sub claim: %w", err)
	}
	claims.Subject = sub
	// tenant_id optional for admin — do not require or restrict by it
	if tenantIDKey := v.claimKeys["tenant_id"]; tenantIDKey != "" {
		if tid, ok := (*mapClaims)[tenantIDKey].(string); ok && tid != "" {
			claims.TenantID = tid
		}
	}
	// tenants array (local_admin): claim "tenants" → []string; backward compat uses tenant_id in TenantsFromClaims
	if raw, ok := (*mapClaims)["tenants"]; ok {
		claims.Tenants = normalizeRoles(raw)
	}
	if rolesKey := v.claimKeys["roles"]; rolesKey != "" {
		if rolesRaw, ok := (*mapClaims)[rolesKey]; ok {
			claims.Roles = normalizeRoles(rolesRaw)
		}
	}
	claims.Issuer = iss
	claims.Audience = aud
	claims.ExpiresAt = exp
	return claims, nil
}

// getKey retrieves a public key from cache or fetches from JWKS
func (c *jwksCache) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// Try read lock first
	c.mu.RLock()
	if key, ok := c.keys[kid]; ok && time.Since(c.lastFetch) < c.ttl {
		c.mu.RUnlock()
		return key, nil
	}
	c.mu.RUnlock()

	// Upgrade to write lock for refresh
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (singleflight pattern)
	if key, ok := c.keys[kid]; ok && time.Since(c.lastFetch) < c.ttl {
		return key, nil
	}

	// Fetch JWKS
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}

	// Try again after refresh
	if key, ok := c.keys[kid]; ok {
		return key, nil
	}

	return nil, fmt.Errorf("key %s not found in JWKS", kid)
}

// refresh fetches JWKS from the endpoint
func (c *jwksCache) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("creating JWKS request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("decoding JWKS: %w", err)
	}

	// Parse keys
	newKeys := make(map[string]*rsa.PublicKey)
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" {
			continue
		}

		pubKey, err := parseRSAPublicKey(key.N, key.E)
		if err != nil {
			continue // Skip invalid keys
		}

		newKeys[key.Kid] = pubKey
	}

	c.keys = newKeys
	c.lastFetch = time.Now()

	return nil
}

// parseRSAPublicKey parses RSA public key from JWKS n and e values
func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("decoding n: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("decoding e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// normalizeRoles handles different role claim formats
func normalizeRoles(raw interface{}) []string {
	switch v := raw.(type) {
	case []interface{}:
		roles := make([]string, 0, len(v))
		for _, r := range v {
			if s, ok := r.(string); ok {
				roles = append(roles, s)
			}
		}
		return roles
	case []string:
		return v
	case string:
		// Handle space-separated or comma-separated
		if strings.Contains(v, ",") {
			return strings.Split(v, ",")
		}
		return strings.Fields(v)
	default:
		return nil
	}
}

// TenantsFromClaims returns the list of tenant IDs for the user (local_admin).
// Prefers c.Tenants; if empty, falls back to c.TenantID as single-element list.
func TenantsFromClaims(c *Claims) []string {
	if len(c.Tenants) > 0 {
		return c.Tenants
	}
	if c.TenantID != "" {
		return []string{c.TenantID}
	}
	return nil
}

// HasAnyRole checks if user has any of the required roles
func HasAnyRole(userRoles, requiredRoles []string) bool {
	roleSet := make(map[string]bool, len(requiredRoles))
	for _, r := range requiredRoles {
		roleSet[r] = true
	}
	for _, r := range userRoles {
		if roleSet[r] {
			return true
		}
	}
	return false
}
