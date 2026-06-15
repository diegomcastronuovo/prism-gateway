package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

// listAPIKeysFakeStorage wraps fakeStorage to provide configurable ListAPIKeysPaged results.
type listAPIKeysFakeStorage struct {
	fakeStorage
	pagedKeys    []storage.APIKeyMeta
	pagedHasMore bool
	pagedErr     error
}

func (s *listAPIKeysFakeStorage) ListAPIKeysPaged(_ context.Context, _ string, _ bool, _, _ int) ([]storage.APIKeyMeta, bool, error) {
	return s.pagedKeys, s.pagedHasMore, s.pagedErr
}

func adminListAPIKeysHandler() *Handlers {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "tenant_a", AllowedModels: []string{"gpt-4"}}},
		Models:  []config.ModelConfig{{Name: "gpt-4", Provider: "openai"}},
	}
	return &Handlers{
		cfg:         cfg,
		store:       &listAPIKeysFakeStorage{},
		log:         testLogger(),
		tenantCache: config.NewTenantConfigCache(0),
	}
}

// TestAdminListAPIKeys_200_ResponseShape verifies the response conforms to the spec shape.
func TestAdminListAPIKeys_200_ResponseShape(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	lastUsed := now.Add(-5 * time.Minute)

	store := &listAPIKeysFakeStorage{
		pagedKeys: []storage.APIKeyMeta{
			{
				ID:         uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				TenantID:   "tenant_a",
				Name:       "test-key",
				Prefix:     "rk_live_ABCD",
				Scopes:     []string{"inference"},
				CreatedAt:  now,
				LastUsedAt: &lastUsed,
				RevokedAt:  nil,
			},
		},
		pagedHasMore: false,
	}

	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "tenant_a"}},
	}
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLogger(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/api-keys", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	rec := httptest.NewRecorder()

	h.AdminListAPIKeys(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp apiKeyListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object=list, got %q", resp.Object)
	}
	if resp.TenantID != "tenant_a" {
		t.Errorf("expected tenant_id=tenant_a, got %q", resp.TenantID)
	}
	if resp.HasMore {
		t.Error("expected has_more=false")
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Data))
	}

	item := resp.Data[0]
	if item.ID.String() != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("unexpected id: %s", item.ID)
	}
	if item.Name != "test-key" {
		t.Errorf("unexpected name: %s", item.Name)
	}
	if item.Prefix != "rk_live_ABCD" {
		t.Errorf("unexpected prefix: %s", item.Prefix)
	}
	if len(item.Scopes) != 1 || item.Scopes[0] != "inference" {
		t.Errorf("unexpected scopes: %v", item.Scopes)
	}
	if item.RevokedAt != nil {
		t.Error("expected revoked_at=null")
	}
	if item.LastUsedAt == nil {
		t.Error("expected last_used_at to be set")
	}
}

// TestAdminListAPIKeys_NoSecretInResponse verifies key_hash and plaintext key are never returned.
func TestAdminListAPIKeys_NoSecretInResponse(t *testing.T) {
	store := &listAPIKeysFakeStorage{
		pagedKeys: []storage.APIKeyMeta{
			{
				ID:        uuid.New(),
				TenantID:  "tenant_a",
				Name:      "secret-key",
				Prefix:    "rk_live_XXXX",
				Scopes:    []string{"inference"},
				CreatedAt: time.Now().UTC(),
			},
		},
	}

	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "tenant_a"}},
	}
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLogger(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/api-keys", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	rec := httptest.NewRecorder()

	h.AdminListAPIKeys(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "key_hash") {
		t.Errorf("response must not contain key_hash: %s", body)
	}
	if strings.Contains(body, "rk_live_") && strings.Count(body, "rk_live_") > 1 {
		// prefix is allowed, full key is not
		t.Errorf("response may contain only prefix, not full key: %s", body)
	}
	// Ensure it doesn't contain "key" as a standalone field
	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)
	data, _ := raw["data"].([]interface{})
	if len(data) > 0 {
		item := data[0].(map[string]interface{})
		if _, hasKey := item["key"]; hasKey {
			t.Errorf("response item must not contain 'key' field")
		}
		if _, hasHash := item["key_hash"]; hasHash {
			t.Errorf("response item must not contain 'key_hash' field")
		}
	}
}

// TestAdminListAPIKeys_HasMore verifies has_more=true when there are more pages.
func TestAdminListAPIKeys_HasMore(t *testing.T) {
	store := &listAPIKeysFakeStorage{
		pagedKeys:    []storage.APIKeyMeta{{ID: uuid.New(), Name: "k", Prefix: "p", Scopes: []string{"inference"}, CreatedAt: time.Now()}},
		pagedHasMore: true,
	}

	cfg := &config.Config{Tenants: []config.TenantConfig{{ID: "tenant_a"}}}
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLogger(),
		tenantCache: config.NewTenantConfigCache(0),
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/api-keys?limit=1", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	rec := httptest.NewRecorder()

	h.AdminListAPIKeys(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp apiKeyListResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.HasMore {
		t.Error("expected has_more=true")
	}
}

// TestAdminListAPIKeys_InvalidLimit verifies bad limit param returns 400.
func TestAdminListAPIKeys_InvalidLimit(t *testing.T) {
	h := adminListAPIKeysHandler()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/api-keys?limit=999", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	rec := httptest.NewRecorder()

	h.AdminListAPIKeys(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for limit>200, got %d", rec.Code)
	}
}

// TestAdminListAPIKeys_InvalidOffset verifies bad offset param returns 400.
func TestAdminListAPIKeys_InvalidOffset(t *testing.T) {
	h := adminListAPIKeysHandler()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/api-keys?offset=-1", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	rec := httptest.NewRecorder()

	h.AdminListAPIKeys(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative offset, got %d", rec.Code)
	}
}

// TestAdminListAPIKeys_EmptyList verifies 200 with empty data array when no keys exist.
func TestAdminListAPIKeys_EmptyList(t *testing.T) {
	h := adminListAPIKeysHandler()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant_a/api-keys", nil)
	req.SetPathValue("tenant_id", "tenant_a")
	rec := httptest.NewRecorder()

	h.AdminListAPIKeys(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp apiKeyListResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Object != "list" {
		t.Errorf("expected object=list, got %q", resp.Object)
	}
	if resp.Data == nil {
		t.Error("expected data array (empty), got nil")
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Data))
	}
}
