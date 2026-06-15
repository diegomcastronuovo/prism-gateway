package config

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeConfigStore is a minimal in-memory implementation of config.Storage for unit tests.
type fakeConfigStore struct {
	configJSON json.RawMessage
	version    int
	exists     bool
	called     bool
}

func (f *fakeConfigStore) GetTenantConfig(_ context.Context, _ string) (json.RawMessage, int, bool, error) {
	f.called = true
	return f.configJSON, f.version, f.exists, nil
}

func (f *fakeConfigStore) GetGlobalConfig(_ context.Context) (json.RawMessage, int, bool, error) {
	return nil, 0, false, nil
}

func baseTestConfig() *Config {
	return &Config{
		DynamicConfig: DynamicConfig{Enabled: true},
		Tenants: []TenantConfig{
			{
				ID:            "t1",
				AllowedModels: []string{"model-a"},
			},
		},
	}
}

func TestResolveTenantConfig_NoDBRow_FallsBackToYAML(t *testing.T) {
	cfg := baseTestConfig()
	store := &fakeConfigStore{exists: false}
	cache := NewTenantConfigCache(0) // zero TTL — never caches

	got, err := cfg.ResolveTenantConfig(context.Background(), "t1", cache, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected YAML fallback config, got nil")
	}
	if got.ID != "t1" {
		t.Errorf("expected tenant ID t1, got %q", got.ID)
	}
	if !store.called {
		t.Error("expected store.GetTenantConfig to be called (dynamic enabled)")
	}
}

func TestResolveTenantConfig_DBRowExists_OverridesYAML(t *testing.T) {
	cfg := baseTestConfig()

	dbCfg := TenantConfig{AllowedModels: []string{"model-db"}}
	raw, _ := json.Marshal(dbCfg)
	store := &fakeConfigStore{configJSON: raw, version: 3, exists: true}
	cache := NewTenantConfigCache(0)

	got, err := cfg.ResolveTenantConfig(context.Background(), "t1", cache, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected DB config, got nil")
	}
	if len(got.AllowedModels) != 1 || got.AllowedModels[0] != "model-db" {
		t.Errorf("expected DB model-db, got %v", got.AllowedModels)
	}
	if got.ID != "t1" {
		t.Errorf("expected tenant ID t1 to be injected, got %q", got.ID)
	}
}

func TestResolveTenantConfig_DynamicDisabled_AlwaysYAML(t *testing.T) {
	cfg := baseTestConfig()
	cfg.DynamicConfig.Enabled = false

	// Store has a DB row — but should never be consulted.
	dbCfg := TenantConfig{AllowedModels: []string{"model-db"}}
	raw, _ := json.Marshal(dbCfg)
	store := &fakeConfigStore{configJSON: raw, version: 1, exists: true}
	cache := NewTenantConfigCache(0)

	got, err := cfg.ResolveTenantConfig(context.Background(), "t1", cache, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected YAML config, got nil")
	}
	if len(got.AllowedModels) != 1 || got.AllowedModels[0] != "model-a" {
		t.Errorf("expected YAML model-a, got %v", got.AllowedModels)
	}
	if store.called {
		t.Error("store.GetTenantConfig must NOT be called when dynamic config is disabled")
	}
}
