package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// ── Constructor ───────────────────────────────────────────────────────────────

func suggestHandlers() *Handlers {
	cfg := &config.Config{
		// Tenant explicitly sets routing.semantic.embedding_model (Phase 2 strict requirement).
		Tenants: []config.TenantConfig{{
			ID: "tenant_a",
			Routing: config.RoutingConfig{
				Semantic: config.SemanticRoutingConfig{EmbeddingModel: "embed-mock"},
			},
		}},
		Models: []config.ModelConfig{
			{Name: "embed-mock", Provider: "mock", Type: "embedding",
				Mock: config.MockConfig{Enabled: true}},
		},
	}
	return &Handlers{
		cfg:            cfg,
		store:          &fakeStorage{},
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

// ── Request helper ────────────────────────────────────────────────────────────

func postSuggest(h *Handlers, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/admin/semantic/anchors/suggest?tenant_id=tenant_a",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithTenant(req.Context(), &config.TenantConfig{ID: "tenant_a"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.SuggestSemanticAnchors(rec, req)
	return rec
}

func postSuggestNoTenant(h *Handlers, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/admin/semantic/anchors/suggest",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.SuggestSemanticAnchors(rec, req)
	return rec
}

// ── Handler tests ─────────────────────────────────────────────────────────────

func TestSuggest_NoTenant(t *testing.T) {
	h := suggestHandlers()
	rec := postSuggestNoTenant(h, `{"dataset":["hello"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuggest_BadJSON(t *testing.T) {
	h := suggestHandlers()
	rec := postSuggest(h, `{invalid`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuggest_EmptyDataset(t *testing.T) {
	h := suggestHandlers()
	rec := postSuggest(h, `{"dataset":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuggest_DatasetTooLarge(t *testing.T) {
	h := suggestHandlers()
	items := make([]string, 501)
	for i := range items {
		items[i] = `"item"`
	}
	body := `{"dataset":[` + strings.Join(items, ",") + `]}`
	rec := postSuggest(h, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuggest_EmptyItem(t *testing.T) {
	h := suggestHandlers()
	rec := postSuggest(h, `{"dataset":["valid","","also valid"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dataset[1]") {
		t.Errorf("expected error to mention dataset[1], got: %s", rec.Body.String())
	}
}

func TestSuggest_MaxClustersNegative(t *testing.T) {
	h := suggestHandlers()
	rec := postSuggest(h, `{"dataset":["a"],"max_clusters":-1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuggest_MaxClustersExceedsMax(t *testing.T) {
	h := suggestHandlers()
	rec := postSuggest(h, `{"dataset":["a"],"max_clusters":51}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuggest_MaxClustersZeroUsesDefault(t *testing.T) {
	h := suggestHandlers()
	rec := postSuggest(h, `{"dataset":["hello","world"],"max_clusters":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp suggestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Anchors) == 0 {
		t.Fatal("expected at least 1 anchor")
	}
}

// TestSuggest_NoEmbeddingModel verifies Phase 2 strict mode:
// when routing.semantic.embedding_model is not configured, returns 400.
func TestSuggest_NoEmbeddingModel(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "tenant_a"}}, // no embedding_model configured
		Models:  []config.ModelConfig{},
	}
	h := &Handlers{
		cfg:            cfg,
		store:          &fakeStorage{},
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
	rec := postSuggest(h, `{"dataset":["a","b"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when embedding_model not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuggest_MaxClusters1(t *testing.T) {
	h := suggestHandlers()
	rec := postSuggest(h, `{"dataset":["a","b","c"],"max_clusters":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp suggestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Anchors) != 1 {
		t.Fatalf("expected 1 anchor, got %d", len(resp.Anchors))
	}
	if resp.Anchors[0].Examples != 3 {
		t.Errorf("expected 3 examples in single cluster, got %d", resp.Anchors[0].Examples)
	}
}

func TestSuggest_ResponseShape(t *testing.T) {
	h := suggestHandlers()
	dataset := []string{"first item", "second item", "third item"}
	body := `{"dataset":["first item","second item","third item"]}`
	rec := postSuggest(h, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp suggestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Anchors) < 1 {
		t.Fatal("expected at least 1 anchor")
	}
	total := 0
	for _, a := range resp.Anchors {
		total += a.Examples
	}
	if total != len(dataset) {
		t.Errorf("expected sum of examples == %d, got %d", len(dataset), total)
	}
}

func TestSuggest_AnchorsAreDatasetMembers(t *testing.T) {
	h := suggestHandlers()
	dataset := []string{"first item", "second item", "third item"}
	body := `{"dataset":["first item","second item","third item"]}`
	rec := postSuggest(h, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp suggestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	set := make(map[string]bool, len(dataset))
	for _, s := range dataset {
		set[s] = true
	}
	for _, a := range resp.Anchors {
		if !set[a.Anchor] {
			t.Errorf("anchor %q not in original dataset", a.Anchor)
		}
	}
}

// ── Pure math unit tests ──────────────────────────────────────────────────────

func TestComputeCentroid_Single(t *testing.T) {
	items := []embeddedItem{{text: "a", embedding: []float64{1.0, 2.0, 3.0}}}
	c := computeCentroid(items)
	for i, v := range c {
		if v != items[0].embedding[i] {
			t.Errorf("dim %d: expected %f, got %f", i, items[0].embedding[i], v)
		}
	}
}

func TestComputeCentroid_Two(t *testing.T) {
	items := []embeddedItem{
		{text: "a", embedding: []float64{1.0, 3.0}},
		{text: "b", embedding: []float64{3.0, 1.0}},
	}
	c := computeCentroid(items)
	if abs64(c[0]-2.0) > 1e-9 {
		t.Errorf("dim 0: expected 2.0, got %f", c[0])
	}
	if abs64(c[1]-2.0) > 1e-9 {
		t.Errorf("dim 1: expected 2.0, got %f", c[1])
	}
}

func TestKMeansCluster_KOne(t *testing.T) {
	items := []embeddedItem{
		{text: "a", embedding: []float64{1, 0}},
		{text: "b", embedding: []float64{2, 0}},
		{text: "c", embedding: []float64{3, 0}},
	}
	clusters := kMeansCluster(items, 1)
	nonEmpty := 0
	total := 0
	for _, c := range clusters {
		if len(c.members) > 0 {
			nonEmpty++
			total += len(c.members)
		}
	}
	if nonEmpty != 1 {
		t.Errorf("expected 1 non-empty cluster, got %d", nonEmpty)
	}
	if total != 3 {
		t.Errorf("expected 3 total members, got %d", total)
	}
}

func TestKMeansCluster_EmptyItems(t *testing.T) {
	result := kMeansCluster(nil, 3)
	if result != nil {
		t.Errorf("expected nil result for empty items, got %v", result)
	}
}

func TestKMeansCluster_Deterministic(t *testing.T) {
	items := []embeddedItem{
		{text: "a", embedding: []float64{1, 0}},
		{text: "b", embedding: []float64{2, 0}},
		{text: "c", embedding: []float64{10, 0}},
		{text: "d", embedding: []float64{11, 0}},
	}
	r1 := kMeansCluster(items, 2)
	r2 := kMeansCluster(items, 2)
	sizes1 := clusterSizes(r1)
	sizes2 := clusterSizes(r2)
	if len(sizes1) != len(sizes2) {
		t.Fatalf("different number of clusters: %d vs %d", len(sizes1), len(sizes2))
	}
	for i := range sizes1 {
		if sizes1[i] != sizes2[i] {
			t.Errorf("cluster %d: size %d vs %d", i, sizes1[i], sizes2[i])
		}
	}
}

func TestKMeansCluster_SumEqualsTotal(t *testing.T) {
	items := []embeddedItem{
		{text: "a", embedding: []float64{1, 0}},
		{text: "b", embedding: []float64{2, 0}},
		{text: "c", embedding: []float64{3, 0}},
		{text: "d", embedding: []float64{10, 0}},
		{text: "e", embedding: []float64{11, 0}},
	}
	clusters := kMeansCluster(items, 3)
	total := 0
	for _, c := range clusters {
		total += len(c.members)
	}
	if total != len(items) {
		t.Errorf("expected total members == %d, got %d", len(items), total)
	}
}

func TestKMeansCluster_ClusteredData(t *testing.T) {
	// Two clearly separated groups: near (0,0) and near (100,100)
	items := []embeddedItem{
		{text: "a1", embedding: []float64{0, 0}},
		{text: "a2", embedding: []float64{1, 0}},
		{text: "a3", embedding: []float64{0, 1}},
		{text: "b1", embedding: []float64{100, 100}},
		{text: "b2", embedding: []float64{101, 100}},
		{text: "b3", embedding: []float64{100, 101}},
	}
	clusters := kMeansCluster(items, 2)
	sizes := clusterSizes(clusters)
	got := countNonEmpty(clusters)
	if got != 2 {
		t.Fatalf("expected 2 non-empty clusters, got %d (sizes: %v)", got, sizes)
	}
	for _, c := range clusters {
		if len(c.members) != 0 && len(c.members) != 3 {
			t.Errorf("expected cluster sizes of 3, got %d", len(c.members))
		}
	}
}

func TestNearestToCentroid_Correct(t *testing.T) {
	centroid := []float64{0.5, 0.5}
	members := []embeddedItem{
		{text: "far", embedding: []float64{0.0, 0.0}},   // dist = 0.5
		{text: "near", embedding: []float64{0.4, 0.6}},  // dist ≈ 0.141
		{text: "medium", embedding: []float64{0.1, 0.9}}, // dist ≈ 0.566
	}
	result := nearestToCentroid(centroid, members)
	if result != "near" {
		t.Errorf("expected 'near', got %q", result)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func clusterSizes(clusters []clusterResult) []int {
	sizes := make([]int, len(clusters))
	for i, c := range clusters {
		sizes[i] = len(c.members)
	}
	return sizes
}

func countNonEmpty(clusters []clusterResult) int {
	n := 0
	for _, c := range clusters {
		if len(c.members) > 0 {
			n++
		}
	}
	return n
}
