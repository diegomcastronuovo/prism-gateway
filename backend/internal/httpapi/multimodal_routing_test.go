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
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ── multimodal fakeStorage ────────────────────────────────────────────────────

// multimodalFakeStorage extends fakeStorage to track modality on anchor operations.
type multimodalFakeStorage struct {
	fakeStorage
	lastUpsertModality string
	lastUpsertAnchorText *string
	lastLookupModality string
	anchorFound        bool
	anchorName         string
	anchorRouteGroup   string
	anchorDistance     float64
}

func (s *multimodalFakeStorage) UpsertSemanticAnchor(
	_ context.Context, _, _ string, _ []float64, _ string, _ []string, anchorText *string, modality string,
) error {
	s.lastUpsertModality = modality
	s.lastUpsertAnchorText = anchorText
	return nil
}

func (s *multimodalFakeStorage) GetNearestSemanticAnchor(
	_ context.Context, _ string, _ []float64, modality string,
) (name, routeGroup string, preferredModels []string, distance float64, found bool, err error) {
	s.lastLookupModality = modality
	if s.anchorFound {
		return s.anchorName, s.anchorRouteGroup, nil, s.anchorDistance, true, nil
	}
	return "", "", nil, 0, false, nil
}

func (s *multimodalFakeStorage) ListSemanticAnchorsSorted(
	_ context.Context, _ string, _ []float64, _ int, modality string,
) ([]storage.SemanticAnchorRow, error) {
	s.lastLookupModality = modality
	return []storage.SemanticAnchorRow{}, nil
}

// ── test helpers ─────────────────────────────────────────────────────────────

// multimodalHandlers builds a Handlers with two embedding models:
// one for "text" and one for "image", configured via SemanticModalities.
func multimodalHandlers(store *multimodalFakeStorage) (*Handlers, *config.TenantConfig) {
	textVec := make([]float64, 4)
	textVec[0] = 0.1
	imageVec := make([]float64, 4)
	imageVec[0] = 0.9

	reg := providers.NewRegistry()
	reg.RegisterEmbedding("openai", fakeEmbeddingProvider{vec: textVec})
	reg.RegisterEmbedding("clip", fakeEmbeddingProvider{vec: imageVec})

	tenant := &config.TenantConfig{
		ID: "t1",
		SemanticModalities: map[string]config.ModalityEmbeddingConfig{
			"text":  {EmbeddingModel: "text-embed"},
			"image": {EmbeddingModel: "image-embed"},
		},
	}

	cfg := &config.Config{
		Tenants: []config.TenantConfig{*tenant},
		Models: []config.ModelConfig{
			{Name: "text-embed", Provider: "openai", Type: "embedding",
				Mock: config.MockConfig{Enabled: false}},
			{Name: "image-embed", Provider: "clip", Type: "embedding",
				Mock: config.MockConfig{Enabled: false}},
		},
	}

	h := &Handlers{
		cfg:            cfg,
		store:          store,
		log:            testLogger(),
		registry:       reg,
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
	}
	return h, tenant
}

func doCreateAnchor(h *Handlers, tenant *config.TenantConfig, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/semantic/anchors?tenant_id=t1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithTenant(req.Context(), tenant)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.CreateSemanticAnchor(rec, req)
	return rec
}

func doSimilarityTest(h *Handlers, tenant *config.TenantConfig, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/semantic/anchors/similarity-test?tenant_id=t1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithTenant(req.Context(), tenant)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.SemanticSimilarityTest(rec, req)
	return rec
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestTextAnchor_Backward_Compat verifies that text anchors work exactly as before
// (modality defaults to "text", stores with modality="text").
func TestTextAnchor_Backward_Compat(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)

	// No modality field — should default to "text"
	rec := doCreateAnchor(h, tenant, `{
		"name": "finance",
		"text": "stock market trends",
		"route_group": "finance"
	}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp anchorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Modality != "text" {
		t.Errorf("expected modality=text, got %q", resp.Modality)
	}
	if store.lastUpsertModality != "text" {
		t.Errorf("expected upsert modality=text, got %q", store.lastUpsertModality)
	}
	if store.lastUpsertAnchorText == nil || *store.lastUpsertAnchorText != "stock market trends" {
		t.Errorf("expected anchor_text persisted from text field, got %v", store.lastUpsertAnchorText)
	}
}

// TestImageAnchor_Create verifies that creating an anchor with modality=image
// and image_url succeeds and stores with the correct modality.
func TestImageAnchor_Create(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)

	rec := doCreateAnchor(h, tenant, `{
		"name": "sunset-photo",
		"modality": "image",
		"image_url": "https://example.com/sunset.jpg",
		"route_group": "photo-routing"
	}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp anchorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Modality != "image" {
		t.Errorf("expected modality=image in response, got %q", resp.Modality)
	}
	if resp.Name != "sunset-photo" {
		t.Errorf("expected name=sunset-photo, got %q", resp.Name)
	}
	if store.lastUpsertModality != "image" {
		t.Errorf("expected upsert modality=image in storage, got %q", store.lastUpsertModality)
	}
	if store.lastUpsertAnchorText != nil {
		t.Errorf("expected no anchor_text for image create without anchor_text, got %q", *store.lastUpsertAnchorText)
	}
}

// TestImageSimilarity_Search verifies that similarity test with modality=image
// passes the modality filter to the storage layer.
func TestImageSimilarity_Search(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)

	rec := doSimilarityTest(h, tenant, `{
		"modality": "image",
		"image_url": "https://example.com/cat.jpg"
	}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastLookupModality != "image" {
		t.Errorf("expected lookup modality=image, got %q", store.lastLookupModality)
	}
}

// TestModality_Isolation_TextNoMatchImage verifies that a text query only searches
// text anchors (modality filter is passed correctly).
func TestModality_Isolation_TextNoMatchImage(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)

	rec := doSimilarityTest(h, tenant, `{
		"text": "describe this picture",
		"modality": "text"
	}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastLookupModality != "text" {
		t.Errorf("expected lookup modality=text (no image match), got %q", store.lastLookupModality)
	}
}

// TestModality_Isolation_ImageNoMatchText verifies that an image anchor creation
// does not interfere with text anchor storage calls.
func TestModality_Isolation_ImageNoMatchText(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)

	// Create image anchor
	rec := doCreateAnchor(h, tenant, `{
		"name": "cat-pic",
		"modality": "image",
		"image_url": "https://example.com/cat.jpg",
		"route_group": "image-route"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("image anchor creation: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.lastUpsertModality != "image" {
		t.Errorf("expected image modality in storage, got %q", store.lastUpsertModality)
	}

	// Now do a text similarity search — should use text modality, not image
	rec = doSimilarityTest(h, tenant, `{"text": "a cat", "modality": "text"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("text similarity: expected 200, got %d", rec.Code)
	}
	if store.lastLookupModality != "text" {
		t.Errorf("text search should use text modality, got %q", store.lastLookupModality)
	}
}

// TestMultimodal_BestRoute_Selected verifies that findBestMultimodalAnchor selects
// the anchor with the best similarity across text and image modalities.
func TestMultimodal_BestRoute_Selected(t *testing.T) {
	// Image anchor has better similarity (lower distance) than text anchor.
	store := &multimodalFakeStorage{
		anchorFound:      true,
		anchorName:       "image-anchor",
		anchorRouteGroup: "visual-route",
		anchorDistance:   0.05, // similarity 0.95
	}
	h, tenant := multimodalHandlers(store)

	// findBestMultimodalAnchor with text + image inputs
	textParts := []string{"describe this image"}
	imageURLs := []string{"https://example.com/photo.jpg"}

	bestAnchor, bestEmb := h.findBestMultimodalAnchor(context.Background(), tenant, textParts, imageURLs)

	if bestAnchor == nil {
		t.Fatal("expected a best anchor to be found, got nil")
	}
	if bestEmb == nil {
		t.Fatal("expected embedding to be non-nil")
	}
	if bestAnchor.Name != "image-anchor" {
		t.Errorf("expected best anchor=image-anchor, got %q", bestAnchor.Name)
	}
	if bestAnchor.RouteGroup != "visual-route" {
		t.Errorf("expected route_group=visual-route, got %q", bestAnchor.RouteGroup)
	}
}

// TestEmbeddingProvider_PerModality verifies that embeddingModelForModality returns
// the correct model for each modality based on SemanticModalities config.
func TestEmbeddingProvider_PerModality(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)

	textModel, textErr := h.embeddingModelForModality(context.Background(), tenant, "text")
	if textErr != nil {
		t.Fatalf("unexpected error for text: %v", textErr)
	}
	if textModel == nil {
		t.Fatal("expected text embedding model, got nil")
	}
	if textModel.Name != "text-embed" {
		t.Errorf("expected text model name=text-embed, got %q", textModel.Name)
	}

	imageModel, imageErr := h.embeddingModelForModality(context.Background(), tenant, "image")
	if imageErr != nil {
		t.Fatalf("unexpected error for image: %v", imageErr)
	}
	if imageModel == nil {
		t.Fatal("expected image embedding model, got nil")
	}
	if imageModel.Name != "image-embed" {
		t.Errorf("expected image model name=image-embed, got %q", imageModel.Name)
	}

	// Unknown modality with no SemanticModalities entry → nil, nil
	unknownModel, unknownErr := h.embeddingModelForModality(context.Background(), tenant, "video")
	if unknownErr != nil {
		t.Errorf("unexpected error for unknown modality: %v", unknownErr)
	}
	if unknownModel != nil {
		t.Errorf("expected nil for unknown modality, got %v", unknownModel.Name)
	}
}

// TestThreshold_Applied verifies that when no anchor is found (nil best anchor),
// the semantic routing is not applied (fail-open behaviour).
func TestThreshold_Applied(t *testing.T) {
	// anchorFound=false → no anchor returned → findBestMultimodalAnchor returns nil
	store := &multimodalFakeStorage{anchorFound: false}
	h, tenant := multimodalHandlers(store)

	textParts := []string{"some text"}
	imageURLs := []string{"https://example.com/img.jpg"}

	bestAnchor, bestEmb := h.findBestMultimodalAnchor(context.Background(), tenant, textParts, imageURLs)

	if bestAnchor != nil {
		t.Errorf("expected nil when no anchor matches, got %+v", bestAnchor)
	}
	if bestEmb != nil {
		t.Errorf("expected nil embedding when no anchor matches")
	}
}

// TestAnchorCreate_ImageMissingURL verifies that creating an image anchor without
// image_url returns 400.
func TestAnchorCreate_ImageMissingURL(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)

	rec := doCreateAnchor(h, tenant, `{
		"name": "bad-anchor",
		"modality": "image",
		"route_group": "test"
	}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for image anchor without image_url, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAnchorCreate_UnsupportedModality verifies that an unsupported modality returns 400.
func TestAnchorCreate_UnsupportedModality(t *testing.T) {
	store := &multimodalFakeStorage{}
	h, tenant := multimodalHandlers(store)

	rec := doCreateAnchor(h, tenant, `{
		"name": "bad-anchor",
		"modality": "video",
		"text": "some text",
		"route_group": "test"
	}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported modality, got %d: %s", rec.Code, rec.Body.String())
	}
}
