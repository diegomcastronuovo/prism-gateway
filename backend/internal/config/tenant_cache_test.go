package config

import (
	"testing"
	"time"
)

func TestTenantCache_GetSet(t *testing.T) {
	cache := NewTenantConfigCache(1 * time.Second)

	config := &TenantConfig{ID: "test-tenant"}
	cache.Set("test-tenant", config, 1)

	retrieved, version, ok := cache.Get("test-tenant")
	if !ok {
		t.Fatal("cache get failed: expected entry to exist")
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
	if retrieved.ID != "test-tenant" {
		t.Errorf("expected tenant ID 'test-tenant', got %s", retrieved.ID)
	}
}

func TestTenantCache_Expiry(t *testing.T) {
	cache := NewTenantConfigCache(100 * time.Millisecond)

	config := &TenantConfig{ID: "test-tenant"}
	cache.Set("test-tenant", config, 1)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	_, _, ok := cache.Get("test-tenant")
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestTenantCache_Invalidate(t *testing.T) {
	cache := NewTenantConfigCache(10 * time.Second)

	config := &TenantConfig{ID: "test-tenant"}
	cache.Set("test-tenant", config, 1)

	// Verify it exists
	_, _, ok := cache.Get("test-tenant")
	if !ok {
		t.Fatal("cache should contain entry before invalidation")
	}

	// Invalidate
	cache.Invalidate("test-tenant")

	// Verify it's gone
	_, _, ok = cache.Get("test-tenant")
	if ok {
		t.Error("expected cache miss after invalidation")
	}
}

func TestTenantCache_Clear(t *testing.T) {
	cache := NewTenantConfigCache(10 * time.Second)

	cache.Set("tenant-1", &TenantConfig{ID: "tenant-1"}, 1)
	cache.Set("tenant-2", &TenantConfig{ID: "tenant-2"}, 2)

	cache.Clear()

	_, _, ok1 := cache.Get("tenant-1")
	_, _, ok2 := cache.Get("tenant-2")

	if ok1 || ok2 {
		t.Error("expected all entries to be cleared")
	}
}

func TestTenantCache_NotFound(t *testing.T) {
	cache := NewTenantConfigCache(10 * time.Second)

	_, _, ok := cache.Get("nonexistent")
	if ok {
		t.Error("expected cache miss for nonexistent entry")
	}
}
