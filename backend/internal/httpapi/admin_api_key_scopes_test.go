package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// TestAdminEndpoints_APIKeyWithAdminRead verifies admin_read scope grants read access
func TestAdminEndpoints_APIKeyWithAdminRead(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	tenantID := "test_admin_read"
	defer cleanupTestTenant(t, db, tenantID)

	// Create API key with admin_read scope
	result, err := store.CreateAPIKey(context.Background(), tenantID, "admin-read-key", []string{"admin_read"}, nil, "test", []string{"admin"})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: tenantID, AllowedModels: []string{"gpt-4o-mini"}},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	// Test GET /admin/tenants/{tenant}/api-keys (read operation)
	req := httptest.NewRequest("GET", "/admin/tenants/"+tenantID+"/api-keys", nil)
	req.Header.Set("X-API-Key", result.Key)
	req.SetPathValue("tenant_id", tenantID)

	rec := httptest.NewRecorder()

	// Simulate full middleware chain
	adminAuthMW := AdminMiddleware(cfg, config.NewTenantConfigCache(1*time.Second), store, nil, auth.NewJWTValidatorCache(testLogger()), testLogger())
	adminScopeMW := AdminScopeMiddleware(testLogger())
	adminTenantMW := AdminTenantIsolationMiddleware(testLogger())

	handler := adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(handlers.AdminListAPIKeys))))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 with admin_read for GET, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminEndpoints_APIKeyWithAdminRead_WriteOperationDenied verifies admin_read cannot write
func TestAdminEndpoints_APIKeyWithAdminRead_WriteOperationDenied(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	tenantID := "test_admin_read_deny_write"
	defer cleanupTestTenant(t, db, tenantID)

	// Create API key with admin_read scope
	result, err := store.CreateAPIKey(context.Background(), tenantID, "admin-read-key", []string{"admin_read"}, nil, "test", []string{"admin"})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: tenantID, AllowedModels: []string{"gpt-4o-mini"}},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	// Test POST /admin/tenants/{tenant}/api-keys (write operation) - should be denied
	reqBody := `{"name":"new-key","scopes":["inference"]}`
	req := httptest.NewRequest("POST", "/admin/tenants/"+tenantID+"/api-keys", nil)
	req.Header.Set("X-API-Key", result.Key)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", tenantID)

	rec := httptest.NewRecorder()

	adminAuthMW := AdminMiddleware(cfg, config.NewTenantConfigCache(1*time.Second), store, nil, auth.NewJWTValidatorCache(testLogger()), testLogger())
	adminScopeMW := AdminScopeMiddleware(testLogger())
	adminTenantMW := AdminTenantIsolationMiddleware(testLogger())

	handler := adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(handlers.AdminCreateAPIKey))))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 with admin_read for POST, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp["error"].(map[string]interface{})["type"] != "authorization_error" {
		t.Errorf("expected authorization_error, got %v", errResp)
	}

	_ = reqBody // avoid unused warning
}

// TestAdminEndpoints_APIKeyWithAdminWrite verifies admin_write grants full access
func TestAdminEndpoints_APIKeyWithAdminWrite(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	tenantID := "test_admin_write"
	defer cleanupTestTenant(t, db, tenantID)

	// Create API key with admin_write scope
	result, err := store.CreateAPIKey(context.Background(), tenantID, "admin-write-key", []string{"admin_write"}, nil, "test", []string{"admin"})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: tenantID, AllowedModels: []string{"gpt-4o-mini"}},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	// Test GET (read operation) - should work
	req := httptest.NewRequest("GET", "/admin/tenants/"+tenantID+"/api-keys", nil)
	req.Header.Set("X-API-Key", result.Key)
	req.SetPathValue("tenant_id", tenantID)

	rec := httptest.NewRecorder()

	adminAuthMW := AdminMiddleware(cfg, config.NewTenantConfigCache(1*time.Second), store, nil, auth.NewJWTValidatorCache(testLogger()), testLogger())
	adminScopeMW := AdminScopeMiddleware(testLogger())
	adminTenantMW := AdminTenantIsolationMiddleware(testLogger())

	handler := adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(handlers.AdminListAPIKeys))))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 with admin_write for GET, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminEndpoints_NoCredentials verifies 401 when no credentials provided
func TestAdminEndpoints_NoCredentials(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())

	cfg := &config.Config{
		Tenants: []config.TenantConfig{},
		Models:  []config.ModelConfig{},
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	req := httptest.NewRequest("GET", "/admin/tenants/tenant_a/api-keys", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	// No headers set

	rec := httptest.NewRecorder()

	adminAuthMW := AdminMiddleware(cfg, config.NewTenantConfigCache(1*time.Second), store, nil, auth.NewJWTValidatorCache(testLogger()), testLogger())
	handler := adminAuthMW(http.HandlerFunc(handlers.AdminListAPIKeys))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 with no credentials, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp["error"].(map[string]interface{})["type"] != "authentication_error" {
		t.Errorf("expected authentication_error, got %v", errResp)
	}
}

// TestAdminEndpoints_XAdminTokenBypass verifies X-Admin-Token bypasses scope checks
func TestAdminEndpoints_XAdminTokenBypass(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	// Set admin token for test
	originalToken := os.Getenv("ADMIN_TOKEN")
	testToken := "test-admin-token-12345"
	os.Setenv("ADMIN_TOKEN", testToken)
	defer func() {
		if originalToken != "" {
			os.Setenv("ADMIN_TOKEN", originalToken)
		} else {
			os.Unsetenv("ADMIN_TOKEN")
		}
	}()

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	tenantID := "test_admin_token"
	defer cleanupTestTenant(t, db, tenantID)

	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: tenantID, AllowedModels: []string{"gpt-4o-mini"}},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	// Test with X-Admin-Token (should bypass all checks)
	req := httptest.NewRequest("GET", "/admin/tenants/"+tenantID+"/api-keys", nil)
	req.Header.Set("X-Admin-Token", testToken)
	req.SetPathValue("tenant_id", tenantID)

	rec := httptest.NewRecorder()

	adminAuthMW := AdminMiddleware(cfg, config.NewTenantConfigCache(1*time.Second), store, nil, auth.NewJWTValidatorCache(testLogger()), testLogger())
	adminScopeMW := AdminScopeMiddleware(testLogger())

	handler := adminAuthMW(adminScopeMW(http.HandlerFunc(handlers.AdminListAPIKeys)))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 with X-Admin-Token, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminEndpoints_TenantIsolation verifies API key cannot access other tenant's data
func TestAdminEndpoints_TenantIsolation(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	tenantA := "test_tenant_a"
	tenantB := "test_tenant_b"
	defer cleanupTestTenant(t, db, tenantA)
	defer cleanupTestTenant(t, db, tenantB)

	// Create API key for tenant A with admin_read
	result, err := store.CreateAPIKey(context.Background(), tenantA, "tenant-a-key", []string{"admin_read"}, nil, "test", []string{"admin"})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: tenantA, AllowedModels: []string{"gpt-4o-mini"}},
			{ID: tenantB, AllowedModels: []string{"gpt-4o-mini"}},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	// Try to access tenant B's data with tenant A's API key - should be denied
	req := httptest.NewRequest("GET", "/admin/tenants/"+tenantB+"/api-keys", nil)
	req.Header.Set("X-API-Key", result.Key)
	req.SetPathValue("tenant_id", tenantB)

	rec := httptest.NewRecorder()

	adminAuthMW := AdminMiddleware(cfg, config.NewTenantConfigCache(1*time.Second), store, nil, auth.NewJWTValidatorCache(testLogger()), testLogger())
	adminScopeMW := AdminScopeMiddleware(testLogger())
	adminTenantMW := AdminTenantIsolationMiddleware(testLogger())

	handler := adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(handlers.AdminListAPIKeys))))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for tenant isolation violation, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp["error"].(map[string]interface{})["type"] != "authorization_error" {
		t.Errorf("expected authorization_error, got %v", errResp)
	}
}

// TestAdminEndpoints_APIKeyWithoutAdminScopes verifies API key without admin scopes is denied
func TestAdminEndpoints_APIKeyWithoutAdminScopes(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	tenantID := "test_no_admin_scope"
	defer cleanupTestTenant(t, db, tenantID)

	// Create API key with only "inference" scope (no admin scope)
	result, err := store.CreateAPIKey(context.Background(), tenantID, "inference-only-key", []string{"inference"}, nil, "test", []string{"admin"})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: tenantID, AllowedModels: []string{"gpt-4o-mini"}},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	req := httptest.NewRequest("GET", "/admin/tenants/"+tenantID+"/api-keys", nil)
	req.Header.Set("X-API-Key", result.Key)
	req.SetPathValue("tenant_id", tenantID)

	rec := httptest.NewRecorder()

	adminAuthMW := AdminMiddleware(cfg, config.NewTenantConfigCache(1*time.Second), store, nil, auth.NewJWTValidatorCache(testLogger()), testLogger())
	handler := adminAuthMW(http.HandlerFunc(handlers.AdminListAPIKeys))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403 for API key without admin scopes, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp["error"].(map[string]interface{})["type"] != "authorization_error" {
		t.Errorf("expected authorization_error, got %v", errResp)
	}
}
