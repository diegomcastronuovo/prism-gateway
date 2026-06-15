package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

// mockJWKSServer creates a test JWKS endpoint with a valid RSA key
func mockJWKSServer(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	pubKey := &privKey.PublicKey

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := map[string]interface{}{
			"keys": []map[string]interface{}{{
				"kty": "RSA",
				"kid": "test-key",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey.E)).Bytes()),
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))

	return srv, privKey
}

// createTestToken creates a valid JWT for testing
func createTestToken(t *testing.T, privKey *rsa.PrivateKey, claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"

	tokenString, err := token.SignedString(privKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return tokenString
}

func TestJWTValidator_ValidToken(t *testing.T) {
	jwksSrv, privKey := mockJWKSServer(t)
	defer jwksSrv.Close()

	cfg := config.JWTConfig{
		Issuer:           "https://test.issuer.com",
		Audience:         "test-audience",
		JWKSURL:          jwksSrv.URL,
		ClockSkewSeconds: 60,
		RequiredClaims: map[string]string{
			"tenant_id": "tenant_id",
			"roles":     "roles",
		},
		CacheTTLMinutes: 10,
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewJWTValidator(cfg, log)

	claims := jwt.MapClaims{
		"iss":       "https://test.issuer.com",
		"aud":       []string{"test-audience"},
		"sub":       "user123",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"tenant_id": "tenant_a",
		"roles":     []interface{}{"user", "admin"},
	}

	tokenString := createTestToken(t, privKey, claims)

	result, err := validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}

	if result.TenantID != "tenant_a" {
		t.Errorf("expected tenant_id 'tenant_a', got '%s'", result.TenantID)
	}

	if result.Subject != "user123" {
		t.Errorf("expected subject 'user123', got '%s'", result.Subject)
	}

	if len(result.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(result.Roles))
	}
}

func TestJWTValidator_ExpiredToken(t *testing.T) {
	jwksSrv, privKey := mockJWKSServer(t)
	defer jwksSrv.Close()

	cfg := config.JWTConfig{
		Issuer:           "https://test.issuer.com",
		Audience:         "test-audience",
		JWKSURL:          jwksSrv.URL,
		ClockSkewSeconds: 60,
		RequiredClaims: map[string]string{
			"tenant_id": "tenant_id",
			"roles":     "roles",
		},
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewJWTValidator(cfg, log)

	claims := jwt.MapClaims{
		"iss":       "https://test.issuer.com",
		"aud":       []string{"test-audience"},
		"sub":       "user123",
		"exp":       time.Now().Add(-2 * time.Hour).Unix(), // Expired 2 hours ago
		"tenant_id": "tenant_a",
		"roles":     []interface{}{"user"},
	}

	tokenString := createTestToken(t, privKey, claims)

	_, err := validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestJWTValidator_InvalidIssuer(t *testing.T) {
	jwksSrv, privKey := mockJWKSServer(t)
	defer jwksSrv.Close()

	cfg := config.JWTConfig{
		Issuer:           "https://test.issuer.com",
		Audience:         "test-audience",
		JWKSURL:          jwksSrv.URL,
		ClockSkewSeconds: 60,
		RequiredClaims: map[string]string{
			"tenant_id": "tenant_id",
			"roles":     "roles",
		},
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewJWTValidator(cfg, log)

	claims := jwt.MapClaims{
		"iss":       "https://wrong.issuer.com", // Wrong issuer
		"aud":       []string{"test-audience"},
		"sub":       "user123",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"tenant_id": "tenant_a",
		"roles":     []interface{}{"user"},
	}

	tokenString := createTestToken(t, privKey, claims)

	_, err := validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for invalid issuer, got nil")
	}
}

func TestJWTValidator_InvalidAudience(t *testing.T) {
	jwksSrv, privKey := mockJWKSServer(t)
	defer jwksSrv.Close()

	cfg := config.JWTConfig{
		Issuer:           "https://test.issuer.com",
		Audience:         "test-audience",
		JWKSURL:          jwksSrv.URL,
		ClockSkewSeconds: 60,
		RequiredClaims: map[string]string{
			"tenant_id": "tenant_id",
			"roles":     "roles",
		},
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewJWTValidator(cfg, log)

	claims := jwt.MapClaims{
		"iss":       "https://test.issuer.com",
		"aud":       []string{"wrong-audience"}, // Wrong audience
		"sub":       "user123",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"tenant_id": "tenant_a",
		"roles":     []interface{}{"user"},
	}

	tokenString := createTestToken(t, privKey, claims)

	_, err := validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for invalid audience, got nil")
	}
}

func TestJWTValidator_MissingTenantID(t *testing.T) {
	jwksSrv, privKey := mockJWKSServer(t)
	defer jwksSrv.Close()

	cfg := config.JWTConfig{
		Issuer:           "https://test.issuer.com",
		Audience:         "test-audience",
		JWKSURL:          jwksSrv.URL,
		ClockSkewSeconds: 60,
		RequiredClaims: map[string]string{
			"tenant_id": "tenant_id",
			"roles":     "roles",
		},
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewJWTValidator(cfg, log)

	claims := jwt.MapClaims{
		"iss":   "https://test.issuer.com",
		"aud":   []string{"test-audience"},
		"sub":   "user123",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []interface{}{"user"},
		// Missing tenant_id
	}

	tokenString := createTestToken(t, privKey, claims)

	_, err := validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for missing tenant_id, got nil")
	}
}

func TestJWTValidator_ValidateTokenForAdmin_WithoutTenantID(t *testing.T) {
	jwksSrv, privKey := mockJWKSServer(t)
	defer jwksSrv.Close()

	cfg := config.JWTConfig{
		Issuer:           "https://test.issuer.com",
		Audience:         "test-audience",
		JWKSURL:          jwksSrv.URL,
		ClockSkewSeconds: 60,
		RequiredClaims:   map[string]string{"tenant_id": "tenant_id", "roles": "roles"},
		CacheTTLMinutes: 10,
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewJWTValidator(cfg, log)

	claims := jwt.MapClaims{
		"iss":   "https://test.issuer.com",
		"aud":   []string{"test-audience"},
		"sub":   "admin-user",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []interface{}{"admin"},
		// no tenant_id — admin can authenticate without tenant
	}

	tokenString := createTestToken(t, privKey, claims)

	result, err := validator.ValidateTokenForAdmin(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateTokenForAdmin should accept token without tenant_id, got error: %v", err)
	}
	if result.TenantID != "" {
		t.Errorf("expected empty TenantID for admin without tenant claim, got %q", result.TenantID)
	}
	if result.Subject != "admin-user" {
		t.Errorf("expected subject 'admin-user', got %q", result.Subject)
	}
	if !HasAnyRole(result.Roles, []string{"admin"}) {
		t.Errorf("expected admin role, got roles: %v", result.Roles)
	}
}

func TestJWTValidator_ValidateTokenForAdmin_WithTenantID(t *testing.T) {
	jwksSrv, privKey := mockJWKSServer(t)
	defer jwksSrv.Close()

	cfg := config.JWTConfig{
		Issuer:           "https://test.issuer.com",
		Audience:         "test-audience",
		JWKSURL:          jwksSrv.URL,
		ClockSkewSeconds: 60,
		RequiredClaims:   map[string]string{"tenant_id": "tenant_id", "roles": "roles"},
		CacheTTLMinutes: 10,
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewJWTValidator(cfg, log)

	claims := jwt.MapClaims{
		"iss":       "https://test.issuer.com",
		"aud":       []string{"test-audience"},
		"sub":       "admin-user",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"tenant_id": "tenant_a",
		"roles":     []interface{}{"admin"},
	}

	tokenString := createTestToken(t, privKey, claims)

	result, err := validator.ValidateTokenForAdmin(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateTokenForAdmin failed: %v", err)
	}
	if result.TenantID != "tenant_a" {
		t.Errorf("expected TenantID 'tenant_a' when present in token, got %q", result.TenantID)
	}
	// Admin access is not restricted by this value; we only check it is returned when present
	if result.Subject != "admin-user" {
		t.Errorf("expected subject 'admin-user', got %q", result.Subject)
	}
}

func TestTenantsFromClaims(t *testing.T) {
	// Prefer Tenants when present
	c1 := &Claims{Tenants: []string{"tenant_a", "tenant_b"}, TenantID: "ignored"}
	if got := TenantsFromClaims(c1); len(got) != 2 || got[0] != "tenant_a" || got[1] != "tenant_b" {
		t.Errorf("TenantsFromClaims(Tenants set): got %v", got)
	}
	// Fallback to TenantID when Tenants empty
	c2 := &Claims{TenantID: "single"}
	if got := TenantsFromClaims(c2); len(got) != 1 || got[0] != "single" {
		t.Errorf("TenantsFromClaims(tenant_id only): got %v", got)
	}
	// Empty when both empty
	c3 := &Claims{}
	if got := TenantsFromClaims(c3); got != nil {
		t.Errorf("TenantsFromClaims(empty): got %v", got)
	}
}

func TestTenantInRequestAllowed(t *testing.T) {
	allowed := []string{"tenant_a", "tenant_b"}
	if !TenantInRequestAllowed("tenant_a", allowed) {
		t.Error("tenant_a should be allowed")
	}
	if TenantInRequestAllowed("tenant_c", allowed) {
		t.Error("tenant_c should not be allowed")
	}
	if TenantInRequestAllowed("tenant_a", nil) {
		t.Error("nil allowed should not allow any")
	}
}

func TestNormalizeRoles_Array(t *testing.T) {
	roles := normalizeRoles([]interface{}{"role1", "role2", "role3"})

	if len(roles) != 3 {
		t.Errorf("expected 3 roles, got %d", len(roles))
	}

	if roles[0] != "role1" || roles[1] != "role2" || roles[2] != "role3" {
		t.Errorf("unexpected roles: %v", roles)
	}
}

func TestNormalizeRoles_StringSlice(t *testing.T) {
	roles := normalizeRoles([]string{"role1", "role2"})

	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

func TestNormalizeRoles_SpaceSeparated(t *testing.T) {
	roles := normalizeRoles("role1 role2 role3")

	if len(roles) != 3 {
		t.Errorf("expected 3 roles, got %d", len(roles))
	}

	if roles[0] != "role1" || roles[1] != "role2" || roles[2] != "role3" {
		t.Errorf("unexpected roles: %v", roles)
	}
}

func TestNormalizeRoles_CommaSeparated(t *testing.T) {
	roles := normalizeRoles("role1,role2,role3")

	if len(roles) != 3 {
		t.Errorf("expected 3 roles, got %d", len(roles))
	}

	if roles[0] != "role1" || roles[1] != "role2" || roles[2] != "role3" {
		t.Errorf("unexpected roles: %v", roles)
	}
}

func TestHasAnyRole_Match(t *testing.T) {
	userRoles := []string{"user", "viewer"}
	requiredRoles := []string{"admin", "user"}

	if !HasAnyRole(userRoles, requiredRoles) {
		t.Error("expected HasAnyRole to return true")
	}
}

func TestHasAnyRole_NoMatch(t *testing.T) {
	userRoles := []string{"viewer", "guest"}
	requiredRoles := []string{"admin", "user"}

	if HasAnyRole(userRoles, requiredRoles) {
		t.Error("expected HasAnyRole to return false")
	}
}

func TestHasAnyRole_Empty(t *testing.T) {
	userRoles := []string{}
	requiredRoles := []string{"admin"}

	if HasAnyRole(userRoles, requiredRoles) {
		t.Error("expected HasAnyRole to return false for empty user roles")
	}
}

func TestJWKSCache_TTL(t *testing.T) {
	jwksSrv, privKey := mockJWKSServer(t)
	defer jwksSrv.Close()

	cfg := config.JWTConfig{
		Issuer:           "https://test.issuer.com",
		Audience:         "test-audience",
		JWKSURL:          jwksSrv.URL,
		ClockSkewSeconds: 60,
		RequiredClaims: map[string]string{
			"tenant_id": "tenant_id",
			"roles":     "roles",
		},
		CacheTTLMinutes: 1, // 1 minute TTL
	}

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	validator := NewJWTValidator(cfg, log)

	claims := jwt.MapClaims{
		"iss":       "https://test.issuer.com",
		"aud":       []string{"test-audience"},
		"sub":       "user123",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"tenant_id": "tenant_a",
		"roles":     []interface{}{"user"},
	}

	tokenString := createTestToken(t, privKey, claims)

	// First validation - should fetch JWKS
	_, err := validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("first validation failed: %v", err)
	}

	// Second validation - should use cache
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("second validation failed: %v", err)
	}

	// Verify cache is working by checking lastFetch time hasn't changed much
	if time.Since(validator.cache.lastFetch) > 100*time.Millisecond {
		t.Error("cache should have been used for second validation")
	}
}
