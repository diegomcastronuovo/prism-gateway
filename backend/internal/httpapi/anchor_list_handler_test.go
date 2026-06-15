package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// listAnchorsFakeStorage extends fakeStorage to control ListSemanticAnchorsPaged.
type listAnchorsFakeStorage struct {
	fakeStorage
	rows    []storage.SemanticAnchorMeta
	hasMore bool
	err     error
}

func (s *listAnchorsFakeStorage) ListSemanticAnchorsPaged(_ context.Context, _ string, _ bool, _, _ int) ([]storage.SemanticAnchorMeta, bool, error) {
	return s.rows, s.hasMore, s.err
}

func listAnchorsHandlers(store *listAnchorsFakeStorage) *Handlers {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "t1"}},
	}
	return &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

func getListAnchors(h *Handlers, query string) *httptest.ResponseRecorder {
	url := "/v1/semantic/anchors?tenant_id=t1"
	if query != "" {
		url += "&" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ListSemanticAnchors(rec, req)
	return rec
}

func TestListSemanticAnchors_Empty(t *testing.T) {
	store := &listAnchorsFakeStorage{rows: []storage.SemanticAnchorMeta{}, hasMore: false}
	h := listAnchorsHandlers(store)
	rec := getListAnchors(h, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp anchorListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("expected object=list, got %q", resp.Object)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty data, got %d items", len(resp.Data))
	}
	if resp.HasMore {
		t.Error("expected has_more=false")
	}
}

func TestListSemanticAnchors_WithData(t *testing.T) {
	rows := []storage.SemanticAnchorMeta{
		{Name: "politics", RouteGroup: "politics", PreferredModels: []string{"gpt-4"}, VectorDims: 1536},
		{Name: "science", RouteGroup: "science", PreferredModels: []string{"claude"}, VectorDims: 768},
	}
	store := &listAnchorsFakeStorage{rows: rows, hasMore: false}
	h := listAnchorsHandlers(store)
	rec := getListAnchors(h, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp anchorListResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.TenantID != "t1" {
		t.Errorf("expected tenant_id=t1, got %q", resp.TenantID)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "politics" {
		t.Errorf("expected first item name=politics, got %q", resp.Data[0].Name)
	}
	if resp.Data[1].VectorDims != 768 {
		t.Errorf("expected second item vector_dims=768, got %d", resp.Data[1].VectorDims)
	}
}

func TestListSemanticAnchors_HasMore(t *testing.T) {
	rows := []storage.SemanticAnchorMeta{
		{Name: "a", RouteGroup: "g", PreferredModels: []string{}, VectorDims: 1536},
	}
	store := &listAnchorsFakeStorage{rows: rows, hasMore: true}
	h := listAnchorsHandlers(store)
	rec := getListAnchors(h, "limit=1")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp anchorListResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.HasMore {
		t.Error("expected has_more=true")
	}
}

func TestListSemanticAnchors_InvalidLimit(t *testing.T) {
	store := &listAnchorsFakeStorage{}
	h := listAnchorsHandlers(store)

	cases := []string{"limit=0", "limit=201", "limit=abc"}
	for _, q := range cases {
		rec := getListAnchors(h, q)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("query %q: expected 400, got %d", q, rec.Code)
		}
	}
}

func TestListSemanticAnchors_IncludeAnchorText(t *testing.T) {
	txt := "hello world"
	rows := []storage.SemanticAnchorMeta{
		{Name: "a", RouteGroup: "g", PreferredModels: []string{}, AnchorText: &txt, VectorDims: 1536},
	}
	store := &listAnchorsFakeStorage{rows: rows}
	h := listAnchorsHandlers(store)
	rec := getListAnchors(h, "include_anchor_text=true")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp anchorListResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Data[0].AnchorText == nil || *resp.Data[0].AnchorText != txt {
		t.Errorf("expected anchor_text=%q, got %v", txt, resp.Data[0].AnchorText)
	}
}

func TestListSemanticAnchors_NoTenant_Returns400(t *testing.T) {
	store := &listAnchorsFakeStorage{}
	h := listAnchorsHandlers(store)
	req := httptest.NewRequest(http.MethodGet, "/v1/semantic/anchors", nil)
	rec := httptest.NewRecorder()
	h.ListSemanticAnchors(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
