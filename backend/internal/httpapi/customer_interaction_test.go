package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ciStore is a fakeStorage variant that captures InsertCustomerInteraction and
// InsertCustomerInteractionBatch calls for assertion.
type ciStore struct {
	fakeStorage

	// captured inserts
	lastInserted      *storage.CustomerInteraction
	lastBatch         []storage.CustomerInteraction
	insertErr         error
	batchErr          error

	// controls for ListCustomerInteractions
	listResult []storage.CustomerInteraction
	listTotal  int
	listFilter storage.CustomerInteractionFilter
	listLimit  int
	listOffset int
}

func (s *ciStore) InsertCustomerInteraction(_ context.Context, row storage.CustomerInteraction) error {
	s.lastInserted = &row
	return s.insertErr
}

func (s *ciStore) InsertCustomerInteractionBatch(_ context.Context, rows []storage.CustomerInteraction) error {
	s.lastBatch = rows
	return s.batchErr
}

func (s *ciStore) ListCustomerInteractions(_ context.Context, filter storage.CustomerInteractionFilter, limit, offset int) ([]storage.CustomerInteraction, int, error) {
	s.listFilter = filter
	s.listLimit = limit
	s.listOffset = offset
	if s.listResult != nil {
		return s.listResult, s.listTotal, nil
	}
	return []storage.CustomerInteraction{}, s.listTotal, nil
}

// ciHandlers builds a minimal *Handlers wired to the given ciStore.
func ciHandlers(store *ciStore) *Handlers {
	cfg := testConfig()
	return &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

// ciCtx attaches a tenant to the request context (simulates auth middleware).
func ciCtx(r *http.Request, tenantID string) *http.Request {
	ctx := auth.WithContextTenantID(r.Context(), tenantID)
	return r.WithContext(ctx)
}

// ─── POST /v1/interactions ────────────────────────────────────────────────────

func TestInteraction_Create_Valid(t *testing.T) {
	store := &ciStore{}
	h := ciHandlers(store)

	body := `{"source_system":"salesforce","occurred_at":"2026-05-31T10:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/interactions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = ciCtx(req, "tenant-a")
	rec := httptest.NewRecorder()

	h.HandleInteractions(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastInserted == nil {
		t.Fatal("expected InsertCustomerInteraction to be called")
	}
	if store.lastInserted.TenantID != "tenant-a" {
		t.Errorf("TenantID: got %q, want %q", store.lastInserted.TenantID, "tenant-a")
	}
	if store.lastInserted.SourceSystem != "salesforce" {
		t.Errorf("SourceSystem: got %q, want salesforce", store.lastInserted.SourceSystem)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["id"]; !ok {
		t.Error("response missing 'id' field")
	}
}

func TestInteraction_Create_MissingSourceSystem(t *testing.T) {
	store := &ciStore{}
	h := ciHandlers(store)

	body := `{"occurred_at":"2026-05-31T10:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/interactions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = ciCtx(req, "tenant-a")
	rec := httptest.NewRecorder()

	h.HandleInteractions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastInserted != nil {
		t.Error("expected InsertCustomerInteraction NOT to be called")
	}
}

func TestInteraction_Create_MissingOccurredAt(t *testing.T) {
	store := &ciStore{}
	h := ciHandlers(store)

	body := `{"source_system":"salesforce"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/interactions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = ciCtx(req, "tenant-a")
	rec := httptest.NewRecorder()

	h.HandleInteractions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── POST /v1/interactions/batch ──────────────────────────────────────────────

func TestInteractionBatch_Valid(t *testing.T) {
	store := &ciStore{}
	h := ciHandlers(store)

	batch := []map[string]interface{}{
		{"source_system": "crm", "occurred_at": "2026-05-31T09:00:00Z"},
		{"source_system": "telephony", "occurred_at": "2026-05-31T09:30:00Z"},
	}
	body, _ := json.Marshal(batch)

	req := httptest.NewRequest(http.MethodPost, "/v1/interactions/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = ciCtx(req, "tenant-b")
	rec := httptest.NewRecorder()

	h.HandleInteractionsBatch(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(store.lastBatch) != 2 {
		t.Fatalf("expected 2 rows in batch, got %d", len(store.lastBatch))
	}
	for _, row := range store.lastBatch {
		if row.TenantID != "tenant-b" {
			t.Errorf("TenantID: got %q, want tenant-b", row.TenantID)
		}
	}

	var resp map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["inserted"] != 2 {
		t.Errorf("inserted: got %d, want 2", resp["inserted"])
	}
}

func TestInteractionBatch_Exceeds500(t *testing.T) {
	store := &ciStore{}
	h := ciHandlers(store)

	// Build a slice of 501 valid interactions.
	batch := make([]map[string]interface{}, 501)
	ts := time.Now().UTC().Format(time.RFC3339)
	for i := range batch {
		batch[i] = map[string]interface{}{
			"source_system": "crm",
			"occurred_at":   ts,
		}
	}
	body, _ := json.Marshal(batch)

	req := httptest.NewRequest(http.MethodPost, "/v1/interactions/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = ciCtx(req, "tenant-b")
	rec := httptest.NewRecorder()

	h.HandleInteractionsBatch(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastBatch != nil {
		t.Error("expected no batch insert to be called")
	}
}

// ─── GET /v1/interactions ─────────────────────────────────────────────────────

func TestInteraction_List_DefaultPagination(t *testing.T) {
	store := &ciStore{}
	h := ciHandlers(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/interactions", nil)
	req = ciCtx(req, "tenant-a")
	rec := httptest.NewRecorder()

	h.HandleInteractions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.listLimit != 50 {
		t.Errorf("default limit: got %d, want 50", store.listLimit)
	}
	if store.listOffset != 0 {
		t.Errorf("default offset: got %d, want 0", store.listOffset)
	}
	// tenant from auth context must be forwarded to filter
	if store.listFilter.TenantID == nil || *store.listFilter.TenantID != "tenant-a" {
		t.Errorf("filter.TenantID: got %v, want tenant-a", store.listFilter.TenantID)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Error("response missing 'data' field")
	}
	if _, ok := resp["total"]; !ok {
		t.Error("response missing 'total' field")
	}
}

func TestInteraction_List_LimitExceedsMax(t *testing.T) {
	store := &ciStore{}
	h := ciHandlers(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/interactions?limit=300", nil)
	req = ciCtx(req, "tenant-a")
	rec := httptest.NewRecorder()

	h.HandleInteractions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
