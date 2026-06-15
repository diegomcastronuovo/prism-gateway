package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestPostgres_GetTenantConfig_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// 1. Versioned path: no active config
	mock.ExpectQuery("SELECT tcv.config_yaml, tcv.version").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)
	// 2. Flat path fallback: also not found
	mock.ExpectQuery("SELECT config_json, version FROM tenants_config").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	_, _, exists, err := store.GetTenantConfig(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if exists {
		t.Error("expected exists=false for nonexistent tenant")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_GetTenantConfig_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	configJSON := json.RawMessage(`{"allowed_models":["gpt-4o-mini"]}`)

	// 1. Versioned path: no active config for this tenant
	mock.ExpectQuery("SELECT tcv.config_yaml, tcv.version").
		WithArgs("test-tenant").
		WillReturnError(sql.ErrNoRows)
	// 2. Flat path fallback: found
	mock.ExpectQuery("SELECT config_json, version FROM tenants_config").
		WithArgs("test-tenant").
		WillReturnRows(sqlmock.NewRows([]string{"config_json", "version"}).
			AddRow(configJSON, 5))

	retrieved, version, exists, err := store.GetTenantConfig(context.Background(), "test-tenant")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !exists {
		t.Error("expected exists=true")
	}
	if version != 5 {
		t.Errorf("expected version 5, got %d", version)
	}
	if string(retrieved) != string(configJSON) {
		t.Errorf("config mismatch: got %s", string(retrieved))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestPostgres_GetTenantConfig_VersionedPathUsed verifies that when a versioned config
// exists (tenant_active_config → tenant_config_versions), it is used in preference to
// tenants_config. This is the fix for bug_source_of_truth: seeded configs must be readable.
func TestPostgres_GetTenantConfig_VersionedPathUsed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Versioned config contains semantic_intent stage (the new stage that was missing from tenants_config)
	versionedConfig := `{"allowed_models":["gpt-4o-mini"],"routing":{"strategy":"smart","smart":{"stages":[{"name":"semantic_intent"}]}}}`

	// 1. Versioned path: found — flat table should NOT be queried
	mock.ExpectQuery("SELECT tcv.config_yaml, tcv.version").
		WithArgs("tenant-versioned").
		WillReturnRows(sqlmock.NewRows([]string{"config_yaml", "version"}).
			AddRow(versionedConfig, 3))
	// Note: no expectation for tenants_config — it must not be queried

	retrieved, version, exists, err := store.GetTenantConfig(context.Background(), "tenant-versioned")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !exists {
		t.Error("expected exists=true")
	}
	if version != 3 {
		t.Errorf("expected version 3, got %d", version)
	}
	// Config must contain the semantic_intent stage from the versioned row
	if !json.Valid(retrieved) {
		t.Errorf("returned config is not valid JSON: %s", string(retrieved))
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(retrieved, &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (tenants_config must not have been queried): %v", err)
	}
}

func TestPostgres_PutTenantConfig_NewTenant(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	configJSON := json.RawMessage(`{"allowed_models":["gpt-4o-mini"]}`)

	mock.ExpectBegin()
	// Step 1: check versioned path — not found (truly new tenant)
	mock.ExpectQuery("SELECT tcv.version").
		WithArgs("new-tenant").
		WillReturnError(sql.ErrNoRows)
	// Step 2: check flat tenants_config — also not found
	mock.ExpectQuery("SELECT version FROM tenants_config WHERE tenant_id = \\$1 FOR UPDATE").
		WithArgs("new-tenant").
		WillReturnError(sql.ErrNoRows)
	// Step 3: insert into flat tenants_config
	mock.ExpectQuery("INSERT INTO tenants_config").
		WithArgs("new-tenant", configJSON, "admin").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
	// Step 4: insert change log
	mock.ExpectExec("INSERT INTO config_change_log").
		WithArgs("new-tenant", "admin", pq.Array([]string{"admin"}), 0, 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	newVersion, err := store.PutTenantConfig(
		context.Background(),
		"new-tenant",
		0, // ifMatchVersion=0 for new tenant
		configJSON,
		"admin",
		[]string{"admin"},
		"Initial config",
		json.RawMessage(`{}`),
	)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if newVersion != 1 {
		t.Errorf("expected version 1, got %d", newVersion)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_PutTenantConfig_VersionConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	configJSON := json.RawMessage(`{"allowed_models":["gpt-4o-mini"]}`)

	mock.ExpectBegin()
	// Step 1: versioned path — not found, fall through to flat path
	mock.ExpectQuery("SELECT tcv.version").
		WithArgs("test-tenant").
		WillReturnError(sql.ErrNoRows)
	// Step 2: flat path — returns version 7, but caller expects 5
	mock.ExpectQuery("SELECT version FROM tenants_config WHERE tenant_id = \\$1 FOR UPDATE").
		WithArgs("test-tenant").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(7))
	mock.ExpectRollback()

	_, err = store.PutTenantConfig(
		context.Background(),
		"test-tenant",
		5, // ifMatchVersion=5, but current is 7
		configJSON,
		"admin",
		[]string{"admin"},
		"Update",
		json.RawMessage(`{}`),
	)

	if err == nil {
		t.Error("expected version conflict error")
	}

	if _, ok := err.(ErrVersionConflict); !ok {
		t.Errorf("expected ErrVersionConflict, got: %T", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_ListTenantConfigChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	testTime := sql.NullTime{Time: sql.NullTime{}.Time, Valid: true}

	mock.ExpectQuery("SELECT id, ts, tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json").
		WithArgs("test-tenant", 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ts", "tenant_id", "actor_sub", "actor_roles", "from_version", "to_version", "change_summary", "diff_json"}).
			AddRow("uuid-1", testTime, "test-tenant", "admin", pq.Array([]string{"admin"}), 1, 2, "Updated config", json.RawMessage(`{}`)))

	changes, err := store.ListTenantConfigChanges(context.Background(), "test-tenant", 50)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if len(changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(changes))
	}

	if len(changes) > 0 && changes[0].ActorSub != "admin" {
		t.Errorf("expected actor_sub 'admin', got %s", changes[0].ActorSub)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ============================================================================
// Integration tests for PascalCase → snake_case normalization
// These require DATABASE_URL environment variable
// ============================================================================

// TestPostgres_NormalizeLegacyConfig_Integration verifies that legacy PascalCase configs
// are automatically normalized to snake_case when read from DB
func TestPostgres_NormalizeLegacyConfig_Integration(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx := context.Background()

	tenantID := "test_legacy_pascal"
	defer func() {
		db.Exec("DELETE FROM config_change_log WHERE tenant_id = $1", tenantID)
		db.Exec("DELETE FROM tenants_config WHERE tenant_id = $1", tenantID)
	}()

	// 1. Directly INSERT legacy PascalCase config into DB (simulating old data)
	legacyConfig := json.RawMessage(`{
		"ID": "test_legacy_pascal",
		"AllowedModels": ["gpt-4o-mini"],
		"Routing": {"Strategy": "round_robin"},
		"Budgets": {
			"MonthlyUSD": 1000,
			"Timezone": "UTC"
		},
		"RateLimit": {"RPM": 100, "Burst": 10},
		"Compliance": {"RetentionDays": 30, "LogMode": "metadata_only"}
	}`)

	_, err = db.ExecContext(ctx, `
		INSERT INTO tenants_config (tenant_id, version, config_json, updated_by)
		VALUES ($1, 1, $2, 'legacy')
	`, tenantID, legacyConfig)
	if err != nil {
		t.Fatalf("failed to insert legacy config: %v", err)
	}

	// 2. GET config - should return snake_case
	configJSON, version, exists, err := store.GetTenantConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetTenantConfig failed: %v", err)
	}
	if !exists {
		t.Fatal("config should exist")
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configJSON, &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	// 3. Verify keys are snake_case, not PascalCase
	if _, ok := config["allowed_models"]; !ok {
		t.Error("expected 'allowed_models' key (snake_case)")
	}
	if _, ok := config["AllowedModels"]; ok {
		t.Error("should NOT have 'AllowedModels' key (PascalCase)")
	}

	budgets := config["budgets"].(map[string]interface{})
	if _, ok := budgets["monthly_usd"]; !ok {
		t.Error("expected 'budgets.monthly_usd' key (snake_case)")
	}
	if _, ok := budgets["MonthlyUSD"]; ok {
		t.Error("should NOT have 'budgets.MonthlyUSD' key (PascalCase)")
	}

	rateLimit := config["rate_limit"].(map[string]interface{})
	if _, ok := rateLimit["rpm"]; !ok {
		t.Error("expected 'rate_limit.rpm' key (snake_case)")
	}
}

// TestPostgres_PatchSnakeCase_OnLegacyConfig_Integration verifies the original bug is fixed:
// PATCH with snake_case should modify a legacy PascalCase config correctly
func TestPostgres_PatchSnakeCase_OnLegacyConfig_Integration(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx := context.Background()

	tenantID := "test_patch_snake"
	defer func() {
		db.Exec("DELETE FROM config_change_log WHERE tenant_id = $1", tenantID)
		db.Exec("DELETE FROM tenants_config WHERE tenant_id = $1", tenantID)
	}()

	// 1. Insert legacy PascalCase config
	legacyConfig := json.RawMessage(`{
		"ID": "test_patch_snake",
		"AllowedModels": ["gpt-4o-mini"],
		"Routing": {"Strategy": "round_robin"},
		"Budgets": {
			"MonthlyUSD": 1000,
			"Timezone": "UTC"
		},
		"RateLimit": {"RPM": 100, "Burst": 10},
		"Compliance": {"RetentionDays": 30, "LogMode": "metadata_only"}
	}`)

	_, err = db.ExecContext(ctx, `
		INSERT INTO tenants_config (tenant_id, version, config_json, updated_by)
		VALUES ($1, 1, $2, 'legacy')
	`, tenantID, legacyConfig)
	if err != nil {
		t.Fatalf("failed to insert legacy config: %v", err)
	}

	// 2. PATCH with snake_case (the original bug: this didn't work)
	patch := json.RawMessage(`{
		"budgets": {
			"monthly_usd": 501
		}
	}`)

	newVersion, err := store.PatchTenantConfig(ctx, tenantID, 1, patch, "admin", []string{"admin"})
	if err != nil {
		t.Fatalf("PatchTenantConfig failed: %v", err)
	}
	if newVersion != 2 {
		t.Errorf("expected version 2 after patch, got %d", newVersion)
	}

	// 3. Verify the patch was applied
	configJSON, version, exists, err := store.GetTenantConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetTenantConfig failed: %v", err)
	}
	if !exists {
		t.Fatal("config should exist")
	}
	if version != 2 {
		t.Errorf("expected version 2, got %d", version)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configJSON, &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	budgets := config["budgets"].(map[string]interface{})
	monthlyUSD := budgets["monthly_usd"].(float64)

	// 4. CRITICAL: Verify monthly_usd changed to 501 (bug fix verification)
	if monthlyUSD != 501 {
		t.Errorf("PATCH did not apply! Expected budgets.monthly_usd=501, got %v", monthlyUSD)
	}

	// 5. Verify entire config is now snake_case (not PascalCase)
	if _, ok := config["allowed_models"]; !ok {
		t.Error("config should have 'allowed_models' key")
	}
	if _, ok := config["AllowedModels"]; ok {
		t.Error("config should NOT have PascalCase 'AllowedModels' key")
	}
}

// TestPostgres_PatchMultipleFields_Integration verifies complex PATCH operations
func TestPostgres_PatchMultipleFields_Integration(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx := context.Background()

	tenantID := "test_patch_multi"
	defer func() {
		db.Exec("DELETE FROM config_change_log WHERE tenant_id = $1", tenantID)
		db.Exec("DELETE FROM tenants_config WHERE tenant_id = $1", tenantID)
	}()

	// 1. Insert config
	initialConfig := json.RawMessage(`{
		"id": "test_patch_multi",
		"allowed_models": ["gpt-4o-mini"],
		"routing": {"strategy": "round_robin"},
		"budgets": {"monthly_usd": 1000, "timezone": "UTC"},
		"rate_limit": {"rpm": 100, "burst": 10},
		"compliance": {"retention_days": 30, "log_mode": "metadata_only"}
	}`)

	_, err = db.ExecContext(ctx, `
		INSERT INTO tenants_config (tenant_id, version, config_json, updated_by)
		VALUES ($1, 1, $2, 'test')
	`, tenantID, initialConfig)
	if err != nil {
		t.Fatalf("failed to insert config: %v", err)
	}

	// 2. PATCH multiple fields at once
	patch := json.RawMessage(`{
		"budgets": {"monthly_usd": 2500, "timezone": "America/New_York"},
		"rate_limit": {"rpm": 500},
		"compliance": {"log_mode": "redacted"}
	}`)

	newVersion, err := store.PatchTenantConfig(ctx, tenantID, 1, patch, "admin", []string{"admin"})
	if err != nil {
		t.Fatalf("PatchTenantConfig failed: %v", err)
	}
	if newVersion != 2 {
		t.Errorf("expected version 2, got %d", newVersion)
	}

	// 3. Verify all patches applied
	configJSON, _, _, err := store.GetTenantConfig(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetTenantConfig failed: %v", err)
	}

	var config map[string]interface{}
	json.Unmarshal(configJSON, &config)

	budgets := config["budgets"].(map[string]interface{})
	if budgets["monthly_usd"].(float64) != 2500 {
		t.Errorf("expected monthly_usd=2500, got %v", budgets["monthly_usd"])
	}
	if budgets["timezone"] != "America/New_York" {
		t.Errorf("expected timezone=America/New_York, got %v", budgets["timezone"])
	}

	rateLimit := config["rate_limit"].(map[string]interface{})
	if rateLimit["rpm"].(float64) != 500 {
		t.Errorf("expected rpm=500, got %v", rateLimit["rpm"])
	}
	// Burst should be preserved (not in patch)
	if rateLimit["burst"].(float64) != 10 {
		t.Errorf("expected burst=10 (preserved), got %v", rateLimit["burst"])
	}

	compliance := config["compliance"].(map[string]interface{})
	if compliance["log_mode"] != "redacted" {
		t.Errorf("expected log_mode=redacted, got %v", compliance["log_mode"])
	}
	// retention_days should be preserved
	if compliance["retention_days"].(float64) != 30 {
		t.Errorf("expected retention_days=30 (preserved), got %v", compliance["retention_days"])
	}
}
