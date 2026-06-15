package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ── fakeStorage override for semantic threshold tests ────────────────────────

type semThreshFakeStorage struct {
	fakeStorage
	// controls what GetTenantConfig returns
	cfgExists  bool
	cfgJSON    []byte
	cfgVersion int
	cfgErr     error
	// captures calls to PatchTenantConfig
	patchCalled       bool
	patchCallCount    int
	patchMergePatch   []byte
	patchErr          error
	// patchConflictOnce: when true, the first PatchTenantConfig call returns
	// ErrVersionConflict; subsequent calls use patchErr (typically nil → success).
	// Use this to test that the handler retries on stale-version conflicts.
	patchConflictOnce bool
}

func (s *semThreshFakeStorage) GetTenantConfig(_ context.Context, _ string) (json.RawMessage, int, bool, error) {
	return s.cfgJSON, s.cfgVersion, s.cfgExists, s.cfgErr
}

func (s *semThreshFakeStorage) PatchTenantConfig(_ context.Context, _ string, _ int, patch json.RawMessage, _ string, _ []string) (int, error) {
	s.patchCalled = true
	s.patchCallCount++
	s.patchMergePatch = patch
	if s.patchConflictOnce && s.patchCallCount == 1 {
		return 0, storage.ErrVersionConflict{Expected: s.cfgVersion, Current: s.cfgVersion + 1}
	}
	return s.cfgVersion + 1, s.patchErr
}

// SeedTenantConfig is already a no-op in fakeStorage; also provide GetTenantConfig
// and PatchTenantConfig overrides above.

// ── helper ────────────────────────────────────────────────────────────────────

func semThreshHandlers(store *semThreshFakeStorage) *Handlers {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "tenant_a"}},
	}
	return &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLogger(),
		tenantCache: config.NewTenantConfigCache(0),
	}
}

func patchSemanticThreshold(h *Handlers, tenantID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch,
		"/admin/tenants/"+tenantID+"/semantic-threshold",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("tenant_id", tenantID)
	// inject a minimal admin tenant context
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: tenantID})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.AdminPatchSemanticThreshold(rec, req)
	return rec
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestAdminPatchSemanticThreshold_ValidUpdate(t *testing.T) {
	store := &semThreshFakeStorage{
		cfgExists:  true,
		cfgVersion: 3,
		cfgJSON:    []byte(`{"id":"tenant_a"}`),
	}
	h := semThreshHandlers(store)
	rec := patchSemanticThreshold(h, "tenant_a", `{"threshold_default": 0.75}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["status"] != "updated" {
		t.Errorf("expected status=updated, got %v", resp["status"])
	}
	if resp["tenant_id"] != "tenant_a" {
		t.Errorf("expected tenant_id=tenant_a, got %v", resp["tenant_id"])
	}
	if got, ok := resp["threshold_default"].(float64); !ok || abs64(got-0.75) > 1e-9 {
		t.Errorf("expected threshold_default=0.75, got %v", resp["threshold_default"])
	}
}

func TestAdminPatchSemanticThreshold_PatchContainsCorrectField(t *testing.T) {
	store := &semThreshFakeStorage{
		cfgExists:  true,
		cfgVersion: 1,
		cfgJSON:    []byte(`{}`),
	}
	h := semThreshHandlers(store)
	patchSemanticThreshold(h, "tenant_a", `{"threshold_default": 0.42}`)

	if !store.patchCalled {
		t.Fatal("expected PatchTenantConfig to be called")
	}

	var patch map[string]interface{}
	json.Unmarshal(store.patchMergePatch, &patch)

	routing, _ := patch["routing"].(map[string]interface{})
	if routing == nil {
		t.Fatalf("patch must have routing key; got %v", patch)
	}
	semantic, _ := routing["semantic"].(map[string]interface{})
	if semantic == nil {
		t.Fatalf("patch.routing must have semantic key; got %v", routing)
	}
	thresh, _ := semantic["threshold_default"].(float64)
	if abs64(thresh-0.42) > 1e-9 {
		t.Errorf("expected threshold_default=0.42 in patch, got %v", thresh)
	}
}

func TestAdminPatchSemanticThreshold_CacheInvalidated(t *testing.T) {
	store := &semThreshFakeStorage{
		cfgExists:  true,
		cfgVersion: 1,
		cfgJSON:    []byte(`{}`),
	}
	cache := config.NewTenantConfigCache(60_000) // large TTL so manual set sticks
	cache.Set("tenant_a", &config.TenantConfig{ID: "tenant_a"}, 1)

	h := semThreshHandlers(store)
	h.tenantCache = cache

	// Confirm entry is cached before the call
	_, _, inCache := cache.Get("tenant_a")
	if !inCache {
		t.Fatal("precondition: tenant_a should be in cache before patch")
	}

	patchSemanticThreshold(h, "tenant_a", `{"threshold_default": 0.5}`)

	_, _, stillInCache := cache.Get("tenant_a")
	if stillInCache {
		t.Error("expected cache entry to be invalidated after patch")
	}
}

func TestAdminPatchSemanticThreshold_MissingField(t *testing.T) {
	h := semThreshHandlers(&semThreshFakeStorage{cfgExists: true, cfgJSON: []byte(`{}`)})
	rec := patchSemanticThreshold(h, "tenant_a", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when threshold_default is absent, got %d", rec.Code)
	}
}

func TestAdminPatchSemanticThreshold_BelowZero(t *testing.T) {
	h := semThreshHandlers(&semThreshFakeStorage{cfgExists: true, cfgJSON: []byte(`{}`)})
	rec := patchSemanticThreshold(h, "tenant_a", `{"threshold_default": -0.1}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for threshold < 0, got %d", rec.Code)
	}
}

func TestAdminPatchSemanticThreshold_AboveOne(t *testing.T) {
	h := semThreshHandlers(&semThreshFakeStorage{cfgExists: true, cfgJSON: []byte(`{}`)})
	rec := patchSemanticThreshold(h, "tenant_a", `{"threshold_default": 1.1}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for threshold > 1, got %d", rec.Code)
	}
}

func TestAdminPatchSemanticThreshold_BoundaryValues(t *testing.T) {
	for _, v := range []float64{0.0, 1.0} {
		body, _ := json.Marshal(map[string]float64{"threshold_default": v})
		store := &semThreshFakeStorage{cfgExists: true, cfgVersion: 1, cfgJSON: []byte(`{}`)}
		h := semThreshHandlers(store)
		rec := patchSemanticThreshold(h, "tenant_a", string(body))
		if rec.Code != http.StatusOK {
			t.Errorf("threshold=%v should be valid, got %d: %s", v, rec.Code, rec.Body.String())
		}
	}
}

func TestAdminPatchSemanticThreshold_TenantNotFound(t *testing.T) {
	// GetTenantConfig returns not-exists; ensureTenantConfigInDB also finds nothing in YAML
	store := &semThreshFakeStorage{cfgExists: false}
	cfg := &config.Config{Tenants: []config.TenantConfig{}} // no tenants in YAML either
	h := &Handlers{
		cfg:         cfg,
		store:       store,
		log:         testLogger(),
		tenantCache: config.NewTenantConfigCache(0),
	}
	rec := patchSemanticThreshold(h, "unknown_tenant", `{"threshold_default": 0.5}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown tenant, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminPatchSemanticThreshold_StorageError(t *testing.T) {
	// patchErr is always ErrVersionConflict → all retries exhausted → 409.
	store := &semThreshFakeStorage{
		cfgExists:  true,
		cfgVersion: 1,
		cfgJSON:    []byte(`{}`),
		patchErr:   storage.ErrVersionConflict{Expected: 1, Current: 2},
	}
	h := semThreshHandlers(store)
	rec := patchSemanticThreshold(h, "tenant_a", `{"threshold_default": 0.5}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 on version conflict, got %d: %s", rec.Code, rec.Body.String())
	}
	// All 3 retry attempts must have been made before giving up.
	if store.patchCallCount != 3 {
		t.Errorf("expected 3 patch attempts (maxRetries), got %d", store.patchCallCount)
	}
}

func TestAdminPatchSemanticThreshold_VersionConflictRetry(t *testing.T) {
	// First PatchTenantConfig returns ErrVersionConflict (stale read); second succeeds.
	// Handler must retry transparently and return 200.
	store := &semThreshFakeStorage{
		cfgExists:         true,
		cfgVersion:        5,
		cfgJSON:           []byte(`{}`),
		patchConflictOnce: true, // call 1 → conflict, call 2+ → patchErr (nil) → success
	}
	h := semThreshHandlers(store)
	rec := patchSemanticThreshold(h, "tenant_a", `{"threshold_default": 0.6}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after retry, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.patchCallCount < 2 {
		t.Errorf("expected at least 2 patch calls (conflict + retry), got %d", store.patchCallCount)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "updated" {
		t.Errorf("expected status=updated after retry, got %v", resp["status"])
	}
}
