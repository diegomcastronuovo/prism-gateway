package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// TestAdminPutTenantConfig_SeedFromYAML tests that PUT works when tenant exists in YAML but not in DB
// The fix ensures:
// 1. Seeding from YAML happens automatically (version=0, no change log)
// 2. The actual PUT creates a change log entry
// 3. actor_roles is never NULL
func TestAdminPutTenantConfig_SeedFromYAML(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping Postgres integration test")
	}

	// Setup database
	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	defer cleanupTestTenant(t, db, "tenant_seed_test")

	// Setup config with tenant in YAML
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{
				ID:            "tenant_seed_test",
				AllowedModels: []string{"gpt-4o-mini"},
				Routing: config.RoutingConfig{
					Strategy: "round_robin",
				},
				RateLimit: config.RateLimitConfig{
					RPM:   100,
					Burst: 10,
				},
				Compliance: config.ComplianceConfig{
					RetentionDays: 30,
					LogMode:       "metadata_only",
				},
				Budgets: config.BudgetsConfig{
					MonthlyUSD: 1000,
					Timezone:   "UTC",
				},
			},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	// Create handlers
	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	// Prepare PUT request body
	putBody := map[string]interface{}{
		"id":             "tenant_seed_test",
		"allowed_models": []string{"gpt-4o-mini"},
		"routing":        map[string]string{"strategy": "cost_based"},
		"rate_limit":     map[string]int{"rpm": 200, "burst": 20},
		"compliance":     map[string]interface{}{"retention_days": 60, "log_mode": "redacted"},
		"budgets":        map[string]interface{}{"monthly_usd": 2000, "timezone": "UTC"},
	}

	bodyJSON, _ := json.Marshal(putBody)

	// Create request with If-Match-Version: 0 (expects seeding)
	req := httptest.NewRequest("PUT", "/admin/tenants/tenant_seed_test/config", bytes.NewReader(bodyJSON))
	req.Header.Set("If-Match-Version", "0")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "tenant_seed_test")

	// Set context with admin-token auth (no roles - should default to ["admin"])
	ctx := auth.WithSub(req.Context(), "admin-token")
	ctx = auth.WithRoles(ctx, []string{}) // typed key; RolesFromContext must see empty → default actor role
	req = req.WithContext(ctx)

	// Execute request
	rec := httptest.NewRecorder()
	handlers.AdminPutTenantConfig(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &response)

	if response["version"].(float64) != 1 {
		t.Errorf("expected version 1 after first PUT, got %v", response["version"])
	}

	// Verify config was saved in DB
	configJSON, version, exists, err := store.GetTenantConfig(context.Background(), "tenant_seed_test")
	if err != nil {
		t.Fatalf("failed to get tenant config: %v", err)
	}
	if !exists {
		t.Fatal("tenant config should exist in DB after PUT")
	}
	if version != 1 {
		t.Errorf("expected version 1 in DB, got %d", version)
	}

	var savedConfig map[string]interface{}
	json.Unmarshal(configJSON, &savedConfig)

	// Verify routing strategy was updated
	if routing, ok := savedConfig["routing"].(map[string]interface{}); ok {
		if routing["strategy"] != "cost_based" {
			t.Errorf("expected strategy 'cost_based', got %v", routing["strategy"])
		}
	} else {
		t.Error("routing not found in saved config")
	}

	// Verify change log was created (only 1 entry - seeding doesn't create log)
	changes, err := store.ListTenantConfigChanges(context.Background(), "tenant_seed_test", 10)
	if err != nil {
		t.Fatalf("failed to get change log: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 change log entry (PUT only, not seeding), got %d", len(changes))
	}

	// Verify the change log has correct actor_roles (should be ["admin"], not NULL)
	if len(changes) > 0 {
		if len(changes[0].ActorRoles) == 0 {
			t.Error("actor_roles should not be empty (should default to [\"admin\"])")
		}
		if changes[0].ActorSub != "admin-token" {
			t.Errorf("expected actor_sub 'admin-token', got %s", changes[0].ActorSub)
		}
	}
}

// TestAdminPatchTenantConfig_SeedFromYAML tests that PATCH works when tenant exists in YAML but not in DB
func TestAdminPatchTenantConfig_SeedFromYAML(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping Postgres integration test")
	}

	// Setup database
	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	defer cleanupTestTenant(t, db, "tenant_patch_test")

	// Setup config with tenant in YAML
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{
				ID:            "tenant_patch_test",
				AllowedModels: []string{"gpt-4o-mini"},
				Routing: config.RoutingConfig{
					Strategy: "round_robin",
				},
				RateLimit: config.RateLimitConfig{
					RPM:   100,
					Burst: 10,
				},
				Compliance: config.ComplianceConfig{
					RetentionDays: 30,
					LogMode:       "metadata_only",
				},
				Budgets: config.BudgetsConfig{
					MonthlyUSD: 1000,
					Timezone:   "UTC",
				},
			},
		},
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	// Create handlers
	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	// Prepare PATCH request (only update RPM)
	patchBody := map[string]interface{}{
		"rate_limit": map[string]int{"rpm": 500},
	}

	bodyJSON, _ := json.Marshal(patchBody)

	// Create request with If-Match-Version: 0 (expects seeding)
	req := httptest.NewRequest("PATCH", "/admin/tenants/tenant_patch_test/config", bytes.NewReader(bodyJSON))
	req.Header.Set("If-Match-Version", "0")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "tenant_patch_test")

	// Set context with admin-token auth
	ctx := auth.WithSub(req.Context(), "admin-token")
	ctx = auth.WithRoles(ctx, []string{})
	req = req.WithContext(ctx)

	// Execute request
	rec := httptest.NewRecorder()
	handlers.AdminPatchTenantConfig(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &response)

	if response["version"].(float64) != 1 {
		t.Errorf("expected version 1 after first PATCH, got %v", response["version"])
	}

	// Verify config was saved and merged correctly
	configJSON, version, exists, err := store.GetTenantConfig(context.Background(), "tenant_patch_test")
	if err != nil {
		t.Fatalf("failed to get tenant config: %v", err)
	}
	if !exists {
		t.Fatal("tenant config should exist in DB after PATCH")
	}
	if version != 1 {
		t.Errorf("expected version 1 in DB, got %d", version)
	}

	var savedConfig map[string]interface{}
	json.Unmarshal(configJSON, &savedConfig)

	// Verify patched field (rpm) and preserved fields from YAML
	if rateLimit, ok := savedConfig["rate_limit"].(map[string]interface{}); ok {
		if rpm := rateLimit["rpm"].(float64); rpm != 500 {
			t.Errorf("expected rpm 500, got %v", rpm)
		}
		// Burst should be preserved from YAML seeding
		if burst := rateLimit["burst"].(float64); burst != 10 {
			t.Errorf("expected burst 10 (from YAML), got %v", burst)
		}
	} else {
		t.Error("rate_limit not found in saved config")
	}

	// Verify change log
	changes, err := store.ListTenantConfigChanges(context.Background(), "tenant_patch_test", 10)
	if err != nil {
		t.Fatalf("failed to get change log: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 change log entry (PATCH only, not seeding), got %d", len(changes))
	}

	// Verify actor_roles is not NULL/empty
	if len(changes) > 0 {
		if len(changes[0].ActorRoles) == 0 {
			t.Error("actor_roles should not be empty")
		}
	}
}

// TestAdminConfig_ActorRolesNeverNull verifies that actor_roles is never NULL in change log
func TestAdminConfig_ActorRolesNeverNull(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping Postgres integration test")
	}

	// Setup database
	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	defer cleanupTestTenant(t, db, "tenant_roles_test")

	cfg := &config.Config{
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	// Create new tenant with PUT (no YAML config)
	putBody := map[string]interface{}{
		"id":             "tenant_roles_test",
		"allowed_models": []string{"gpt-4o-mini"},
		"routing":        map[string]string{"strategy": "round_robin"},
		"rate_limit":     map[string]int{"rpm": 100, "burst": 10},
		"compliance":     map[string]interface{}{"retention_days": 30, "log_mode": "metadata_only"},
		"budgets":        map[string]interface{}{"monthly_usd": 1000, "timezone": "UTC"},
	}

	bodyJSON, _ := json.Marshal(putBody)

	req := httptest.NewRequest("PUT", "/admin/tenants/tenant_roles_test/config", bytes.NewReader(bodyJSON))
	req.Header.Set("If-Match-Version", "0")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "tenant_roles_test")

	// Context with empty roles (simulating admin-token auth with no roles)
	ctx := auth.WithSub(req.Context(), "admin-token")
	ctx = auth.WithRoles(ctx, []string{})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handlers.AdminPutTenantConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Check change log directly in database
	var actorRoles []string
	err = db.QueryRow(`
		SELECT actor_roles FROM config_change_log
		WHERE tenant_id = $1
		ORDER BY ts DESC LIMIT 1
	`, "tenant_roles_test").Scan(&actorRoles)

	if err != nil {
		t.Fatalf("failed to query actor_roles: %v", err)
	}

	if actorRoles == nil || len(actorRoles) == 0 {
		t.Error("actor_roles should NOT be NULL or empty in database")
	}

	if len(actorRoles) > 0 && actorRoles[0] != "admin" {
		t.Errorf("expected default role 'admin', got %v", actorRoles)
	}
}

// TestAdminConfig_NoSeedingWhenTenantNotInYAML verifies that PUT creates new tenant even when not in YAML
func TestAdminConfig_NoSeedingWhenTenantNotInYAML(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping Postgres integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := storage.NewPostgresFromDB(db, testLogger())
	defer cleanupTestTenant(t, db, "tenant_no_yaml")

	cfg := &config.Config{
		Models: []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
		},
		// No tenant_no_yaml in Tenants list
	}

	handlers := &Handlers{
		cfg:   cfg,
		store: store,
		log:   testLogger(),
	}

	putBody := map[string]interface{}{
		"id":             "tenant_no_yaml",
		"allowed_models": []string{"gpt-4o-mini"},
		"routing":        map[string]string{"strategy": "round_robin"},
		"rate_limit":     map[string]int{"rpm": 100, "burst": 10},
		"compliance":     map[string]interface{}{"retention_days": 30, "log_mode": "metadata_only"},
		"budgets":        map[string]interface{}{"monthly_usd": 1000, "timezone": "UTC"},
	}

	bodyJSON, _ := json.Marshal(putBody)

	req := httptest.NewRequest("PUT", "/admin/tenants/tenant_no_yaml/config", bytes.NewReader(bodyJSON))
	req.Header.Set("If-Match-Version", "0")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", "tenant_no_yaml")

	ctx := auth.WithSub(req.Context(), "admin")
	ctx = auth.WithRoles(ctx, []string{"admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handlers.AdminPutTenantConfig(rec, req)

	// Should succeed even though tenant is not in YAML
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify version is 1 (created directly, no seeding)
	var response map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &response)

	if response["version"].(float64) != 1 {
		t.Errorf("expected version 1, got %v", response["version"])
	}
}

// Helper function to cleanup test tenant data
func cleanupTestTenant(t *testing.T, db *sql.DB, tenantID string) {
	_, err := db.Exec("DELETE FROM config_change_log WHERE tenant_id = $1", tenantID)
	if err != nil {
		t.Logf("warning: failed to cleanup config_change_log: %v", err)
	}
	_, err = db.Exec("DELETE FROM tenants_config WHERE tenant_id = $1", tenantID)
	if err != nil {
		t.Logf("warning: failed to cleanup tenants_config: %v", err)
	}
	_, err = db.Exec("DELETE FROM api_keys WHERE tenant_id = $1", tenantID)
	if err != nil {
		t.Logf("warning: failed to cleanup api_keys: %v", err)
	}
}
