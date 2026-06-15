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

// ── fakeStorage override ──────────────────────────────────────────────────────

type similarityFakeStorage struct {
	fakeStorage
	rows    []storage.SemanticAnchorRow
	rowsErr error
}

func (s *similarityFakeStorage) ListSemanticAnchorsSorted(_ context.Context, _ string, _ []float64, _ int, _ string) ([]storage.SemanticAnchorRow, error) {
	return s.rows, s.rowsErr
}

// ── helpers ──────────────────────────────────────────────────────────────────

func similarityTestHandlers(store *similarityFakeStorage) *Handlers {
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
		store:          store,
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
}

// tenantWithEmbedModel is the default tenant used for similarity tests.
// It explicitly sets routing.semantic.embedding_model (Phase 2 strict requirement).
var tenantWithEmbedModel = &config.TenantConfig{
	ID: "tenant_a",
	Routing: config.RoutingConfig{
		Semantic: config.SemanticRoutingConfig{EmbeddingModel: "embed-mock"},
	},
}

func postSimilarityTest(h *Handlers, body string) *httptest.ResponseRecorder {
	return postSimilarityTestWithTenant(h, body, tenantWithEmbedModel)
}

func postSimilarityTestWithTenant(h *Handlers, body string, tenant *config.TenantConfig) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/semantic/anchors/similarity-test?tenant_id=tenant_a",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithTenant(req.Context(), tenant)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.SemanticSimilarityTest(rec, req)
	return rec
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestSemanticSimilarityTest_MissingText(t *testing.T) {
	h := similarityTestHandlers(&similarityFakeStorage{})
	rec := postSimilarityTest(h, `{"top_k": 5}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSemanticSimilarityTest_InvalidTopK(t *testing.T) {
	h := similarityTestHandlers(&similarityFakeStorage{})
	rec := postSimilarityTest(h, `{"text": "hello", "top_k": 300}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for top_k>200, got %d", rec.Code)
	}
}

func TestSemanticSimilarityTest_TopKZero(t *testing.T) {
	h := similarityTestHandlers(&similarityFakeStorage{})
	rec := postSimilarityTest(h, `{"text": "hello", "top_k": 0}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for top_k=0, got %d", rec.Code)
	}
}

func TestSemanticSimilarityTest_EmptyAnchors_Returns200(t *testing.T) {
	h := similarityTestHandlers(&similarityFakeStorage{rows: []storage.SemanticAnchorRow{}})
	rec := postSimilarityTest(h, `{"text": "hello world"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp similarityTestResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Object != "semantic_anchor_similarity_test" {
		t.Errorf("unexpected object: %s", resp.Object)
	}
	if resp.TopMatch != nil {
		t.Error("expected top_match=null for empty anchors")
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty data, got %d items", len(resp.Data))
	}
}

func TestSemanticSimilarityTest_ResponseShape(t *testing.T) {
	rows := []storage.SemanticAnchorRow{
		{Name: "politics", RouteGroup: "politics", PreferredModels: []string{"gpt-4"}, Distance: 0.28, VectorDims: 1536},
		{Name: "math", RouteGroup: "math", PreferredModels: []string{"gemini"}, Distance: 0.80, VectorDims: 1536},
	}
	h := similarityTestHandlers(&similarityFakeStorage{rows: rows})
	rec := postSimilarityTest(h, `{"text": "political tensions"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp similarityTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if resp.Object != "semantic_anchor_similarity_test" {
		t.Errorf("wrong object: %s", resp.Object)
	}
	if resp.TenantID != "tenant_a" {
		t.Errorf("wrong tenant_id: %s", resp.TenantID)
	}
	if resp.Threshold <= 0 {
		t.Errorf("threshold must be > 0, got %v", resp.Threshold)
	}
	if resp.Input.Text != "political tensions" {
		t.Errorf("wrong input text: %s", resp.Input.Text)
	}
	if resp.TopMatch == nil {
		t.Fatal("expected top_match to be set")
	}
	if resp.TopMatch.Name != "politics" {
		t.Errorf("top_match should be closest anchor (politics), got %s", resp.TopMatch.Name)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Data))
	}
	if resp.Data[1].AnchorVectorDims != 1536 {
		t.Errorf("expected anchor_vector_dims=1536, got %d", resp.Data[1].AnchorVectorDims)
	}

	// similarity = 1 - distance
	wantSim0 := 1.0 - 0.28
	if abs64(resp.Data[0].Similarity-wantSim0) > 1e-9 {
		t.Errorf("Data[0].Similarity = %v, want %v", resp.Data[0].Similarity, wantSim0)
	}
}

func TestSemanticSimilarityTest_MatchedFlag(t *testing.T) {
	// default threshold = 0.60
	// distance 0.28 → similarity 0.72 → matched=true
	// distance 0.80 → similarity 0.20 → matched=false
	rows := []storage.SemanticAnchorRow{
		{Name: "close", RouteGroup: "a", PreferredModels: []string{}, Distance: 0.28, VectorDims: 1536},
		{Name: "far", RouteGroup: "b", PreferredModels: []string{}, Distance: 0.80, VectorDims: 1536},
	}
	h := similarityTestHandlers(&similarityFakeStorage{rows: rows})
	rec := postSimilarityTest(h, `{"text": "test"}`)

	var resp similarityTestResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	if !resp.Data[0].Matched {
		t.Errorf("expected Data[0] (sim=0.72) matched=true with default threshold 0.60")
	}
	if resp.Data[1].Matched {
		t.Errorf("expected Data[1] (sim=0.20) matched=false with default threshold 0.60")
	}
}

func TestSemanticSimilarityTest_ThresholdFromTenantConfig(t *testing.T) {
	want := 0.75
	tenantWithThreshold := &config.TenantConfig{
		ID: "tenant_a",
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{EmbeddingModel: "embed-mock"},
			Smart: config.SmartConfig{
				Stages: []config.SmartStage{{
					Name: "semantic_intent",
					Rules: []config.SmartStageRule{{
						When:   config.SmartRuleCondition{SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: want}},
						Action: config.SmartAction{UseAnchor: true},
					}},
				}},
			},
		},
	}

	rows := []storage.SemanticAnchorRow{
		{Name: "anchor1", RouteGroup: "g", PreferredModels: []string{}, Distance: 0.20, VectorDims: 1536},
	}
	cfg := &config.Config{
		Tenants: []config.TenantConfig{*tenantWithThreshold},
		Models: []config.ModelConfig{
			{Name: "embed-mock", Provider: "mock", Type: "embedding",
				Mock: config.MockConfig{Enabled: true}},
		},
	}
	h := &Handlers{
		cfg:            cfg,
		store:          &similarityFakeStorage{rows: rows},
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}

	rec := postSimilarityTestWithTenant(h, `{"text": "test"}`, tenantWithThreshold)

	var resp similarityTestResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if abs64(resp.Threshold-want) > 1e-9 {
		t.Errorf("expected threshold=%v from tenant config, got %v", want, resp.Threshold)
	}
}

func TestSemanticSimilarityTest_IncludeAnchorTextFalse_NilText(t *testing.T) {
	rows := []storage.SemanticAnchorRow{
		{Name: "anchor1", RouteGroup: "g", PreferredModels: []string{}, Distance: 0.1, VectorDims: 1536},
	}
	h := similarityTestHandlers(&similarityFakeStorage{rows: rows})
	rec := postSimilarityTest(h, `{"text": "hello"}`)

	var resp similarityTestResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Data[0].AnchorText != nil {
		t.Errorf("expected anchor_text=null when include_anchor_text=false")
	}
}

func TestSemanticSimilarityTest_NoTenant_Returns400(t *testing.T) {
	h := similarityTestHandlers(&similarityFakeStorage{})
	req := httptest.NewRequest(http.MethodPost, "/v1/semantic/anchors/similarity-test",
		strings.NewReader(`{"text": "hello"}`))
	rec := httptest.NewRecorder()
	h.SemanticSimilarityTest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 with no tenant, got %d", rec.Code)
	}
}

// TestSemanticSimilarityTest_NoEmbeddingModel_Returns400 verifies Phase 2 strict mode:
// when routing.semantic.embedding_model is not configured, the endpoint returns 400.
func TestSemanticSimilarityTest_NoEmbeddingModel_Returns400(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{{ID: "tenant_a"}},
		Models:  []config.ModelConfig{}, // no embedding model
	}
	h := &Handlers{
		cfg:            cfg,
		store:          &similarityFakeStorage{},
		log:            testLogger(),
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
	// Tenant with no routing.semantic.embedding_model configured.
	tenant := &config.TenantConfig{ID: "tenant_a"}
	rec := postSimilarityTestWithTenant(h, `{"text": "hello"}`, tenant)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when embedding_model not configured, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error.Type != "invalid_request_error" {
		t.Errorf("expected invalid_request_error, got %q", resp.Error.Type)
	}
}

// ── resolveSemanticThreshold unit tests ──────────────────────────────────────

func TestResolveSemanticThreshold_Default(t *testing.T) {
	tenant := &config.TenantConfig{}
	got := resolveSemanticThreshold(tenant, nil)
	if abs64(got-defaultSemanticThreshold) > 1e-9 {
		t.Errorf("expected default %v, got %v", defaultSemanticThreshold, got)
	}
}

// TestResolveSemanticThreshold_ExplicitOverride verifies that a non-nil request
// override beats everything else, including a calibrated ThresholdDefault.
func TestResolveSemanticThreshold_ExplicitOverride(t *testing.T) {
	overrideVal := 0.42
	tenant := &config.TenantConfig{
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{ThresholdDefault: 0.34},
			Smart: config.SmartConfig{
				Rules: []config.SmartRule{{
					When: config.SmartRuleCondition{
						SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: 0.60},
					},
				}},
			},
		},
	}
	got := resolveSemanticThreshold(tenant, &overrideVal)
	if abs64(got-overrideVal) > 1e-9 {
		t.Errorf("expected override %v, got %v", overrideVal, got)
	}
}

// TestResolveSemanticThreshold_TenantDefaultBeatsRuleThreshold is the key regression
// test: a calibrated ThresholdDefault must win over a static per-rule threshold.
func TestResolveSemanticThreshold_TenantDefaultBeatsRuleThreshold(t *testing.T) {
	tenantDefault := 0.34
	ruleThreshold := 0.60
	tenant := &config.TenantConfig{
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{ThresholdDefault: tenantDefault},
			Smart: config.SmartConfig{
				Stages: []config.SmartStage{{
					Rules: []config.SmartStageRule{{
						When: config.SmartRuleCondition{
							SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: ruleThreshold},
						},
					}},
				}},
				Rules: []config.SmartRule{{
					When: config.SmartRuleCondition{
						SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: ruleThreshold},
					},
				}},
			},
		},
	}
	got := resolveSemanticThreshold(tenant, nil)
	if abs64(got-tenantDefault) > 1e-9 {
		t.Errorf("ThresholdDefault should beat rule threshold: expected %v, got %v", tenantDefault, got)
	}
}

// TestResolveSemanticThreshold_FromStages verifies that a stage rule threshold is used
// when no ThresholdDefault is set.
func TestResolveSemanticThreshold_FromStages(t *testing.T) {
	want := 0.75
	tenant := &config.TenantConfig{
		Routing: config.RoutingConfig{
			Smart: config.SmartConfig{
				Stages: []config.SmartStage{{
					Rules: []config.SmartStageRule{{
						When: config.SmartRuleCondition{
							SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: want},
						},
					}},
				}},
			},
		},
	}
	got := resolveSemanticThreshold(tenant, nil)
	if abs64(got-want) > 1e-9 {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestResolveSemanticThreshold_StagesTakePrecedenceOverLegacy(t *testing.T) {
	stageTh := 0.75
	legacyTh := 0.55
	tenant := &config.TenantConfig{
		Routing: config.RoutingConfig{
			Smart: config.SmartConfig{
				Stages: []config.SmartStage{{
					Rules: []config.SmartStageRule{{
						When: config.SmartRuleCondition{
							SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: stageTh},
						},
					}},
				}},
				Rules: []config.SmartRule{{
					When: config.SmartRuleCondition{
						SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: legacyTh},
					},
				}},
			},
		},
	}
	got := resolveSemanticThreshold(tenant, nil)
	if abs64(got-stageTh) > 1e-9 {
		t.Errorf("expected stages threshold %v, got %v", stageTh, got)
	}
}

func TestResolveSemanticThreshold_FromLegacyRules(t *testing.T) {
	want := 0.55
	tenant := &config.TenantConfig{
		Routing: config.RoutingConfig{
			Smart: config.SmartConfig{
				Rules: []config.SmartRule{{
					When: config.SmartRuleCondition{
						SemanticSimilarity: &config.SemanticSimilarityCondition{Threshold: want},
					},
				}},
			},
		},
	}
	got := resolveSemanticThreshold(tenant, nil)
	if abs64(got-want) > 1e-9 {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
