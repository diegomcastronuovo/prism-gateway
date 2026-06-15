package httpapi

import (
	"context"
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// embModelHandlers builds a Handlers for embedding model resolution tests.
// models is the set of models in global config; the caller sets up the tenant.
func embModelHandlers(models []config.ModelConfig) *Handlers {
	reg := providers.NewRegistry()
	reg.RegisterEmbedding("local", fakeEmbeddingProvider{vec: []float64{0.1, 0.2}})

	cfg := &config.Config{
		Models: models,
	}
	return &Handlers{
		cfg:            cfg,
		log:            testLogger(),
		store:          storage.NopStorage{},
		tenantCache:    config.NewTenantConfigCache(0),
		globalCfgCache: config.NewGlobalConfigCache(0),
		registry:       reg,
	}
}

func embeddingModel(name string) config.ModelConfig {
	return config.ModelConfig{Name: name, Provider: "local", Type: "embedding"}
}

// ── resolveTextEmbeddingModel tests ──────────────────────────────────────────

// TestResolveTextEmbeddingModel_ExplicitTenantModel verifies that when
// routing.semantic.embedding_model is set it is used and returned.
func TestResolveTextEmbeddingModel_ExplicitTenantModel(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
		embeddingModel("tenant-embed"),
	})
	tenant := &config.TenantConfig{
		ID: "t1",
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{EmbeddingModel: "tenant-embed"},
		},
	}

	got, err := h.resolveTextEmbeddingModel(context.Background(), tenant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected model, got nil")
	}
	if got.Name != "tenant-embed" {
		t.Errorf("expected tenant-embed, got %q", got.Name)
	}
}

// TestResolveTextEmbeddingModel_MissingConfigReturnsError verifies Phase 2 strict mode:
// when routing.semantic.embedding_model is empty, an error is returned (no global fallback).
func TestResolveTextEmbeddingModel_MissingConfigReturnsError(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
	})
	tenant := &config.TenantConfig{
		ID:      "t1",
		Routing: config.RoutingConfig{Semantic: config.SemanticRoutingConfig{}},
	}

	got, err := h.resolveTextEmbeddingModel(context.Background(), tenant)
	if got != nil {
		t.Errorf("expected nil model, got %q", got.Name)
	}
	if err == nil {
		t.Fatal("expected error for missing embedding_model config, got nil")
	}
	if err.Error() != "embedding_model not configured for tenant" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

// TestResolveTextEmbeddingModel_ConfiguredModelNotFoundReturnsError verifies Phase 2 strict mode:
// when the configured model name doesn't exist in config an error is returned (no fallback).
func TestResolveTextEmbeddingModel_ConfiguredModelNotFoundReturnsError(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
	})
	tenant := &config.TenantConfig{
		ID: "t1",
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{EmbeddingModel: "does-not-exist"},
		},
	}

	got, err := h.resolveTextEmbeddingModel(context.Background(), tenant)
	if got != nil {
		t.Errorf("expected nil model, got %q", got.Name)
	}
	if err == nil {
		t.Fatal("expected error for invalid embedding model, got nil")
	}
	if err.Error() != "invalid embedding model" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

// TestResolveTextEmbeddingModel_ConfiguredModelWrongTypeReturnsError verifies that a model
// with the right name but wrong type (not "embedding") returns an error.
func TestResolveTextEmbeddingModel_ConfiguredModelWrongTypeReturnsError(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		{Name: "gpt-4o", Provider: "openai", Type: "chat"},
	})
	tenant := &config.TenantConfig{
		ID: "t1",
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{EmbeddingModel: "gpt-4o"},
		},
	}

	got, err := h.resolveTextEmbeddingModel(context.Background(), tenant)
	if got != nil {
		t.Errorf("expected nil model, got %q", got.Name)
	}
	if err == nil || err.Error() != "invalid embedding model" {
		t.Errorf("expected 'invalid embedding model' error, got: %v", err)
	}
}

// TestResolveTextEmbeddingModel_NilTenantReturnsError verifies Phase 2 strict mode:
// nil tenant → error (no global fallback).
func TestResolveTextEmbeddingModel_NilTenantReturnsError(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
	})

	got, err := h.resolveTextEmbeddingModel(context.Background(), nil)
	if got != nil {
		t.Errorf("expected nil model, got %q", got.Name)
	}
	if err == nil {
		t.Fatal("expected error for nil tenant, got nil")
	}
}

// ── embeddingModelForModality tests ──────────────────────────────────────────

// TestEmbeddingModelForModality_RoutingSemanticOverride verifies that
// routing.semantic.embedding_model is used when no SemanticModalities override is set.
func TestEmbeddingModelForModality_RoutingSemanticOverride(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
		embeddingModel("routing-embed"),
	})
	tenant := &config.TenantConfig{
		ID: "t1",
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{EmbeddingModel: "routing-embed"},
		},
	}

	got, err := h.embeddingModelForModality(context.Background(), tenant, "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "routing-embed" {
		t.Errorf("expected routing-embed, got %v", got)
	}
}

// TestEmbeddingModelForModality_ModalityOverrideTakesPriority verifies that
// SemanticModalities takes precedence over routing.semantic.embedding_model.
func TestEmbeddingModelForModality_ModalityOverrideTakesPriority(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("modality-embed"),
		embeddingModel("routing-embed"),
	})
	tenant := &config.TenantConfig{
		ID: "t1",
		SemanticModalities: map[string]config.ModalityEmbeddingConfig{
			"text": {EmbeddingModel: "modality-embed"},
		},
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{EmbeddingModel: "routing-embed"},
		},
	}

	got, err := h.embeddingModelForModality(context.Background(), tenant, "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != "modality-embed" {
		t.Errorf("expected modality-embed (higher priority), got %v", got)
	}
}

// TestEmbeddingModelForModality_NilTenantReturnsError verifies Phase 2 strict mode:
// nil tenant → error (no global fallback).
func TestEmbeddingModelForModality_NilTenantReturnsError(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
	})

	got, err := h.embeddingModelForModality(context.Background(), nil, "text")
	if got != nil {
		t.Errorf("expected nil model, got %q", got.Name)
	}
	if err == nil {
		t.Fatal("expected error for nil tenant, got nil")
	}
}

// TestEmbeddingModelForModality_MissingTextConfigReturnsError verifies Phase 2:
// text modality with no routing.semantic.embedding_model → error.
func TestEmbeddingModelForModality_MissingTextConfigReturnsError(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
	})
	tenant := &config.TenantConfig{
		ID:      "t1",
		Routing: config.RoutingConfig{Semantic: config.SemanticRoutingConfig{}},
	}

	got, err := h.embeddingModelForModality(context.Background(), tenant, "text")
	if got != nil {
		t.Errorf("expected nil, got %q", got.Name)
	}
	if err == nil || err.Error() != "embedding_model not configured for tenant" {
		t.Errorf("expected 'embedding_model not configured for tenant', got: %v", err)
	}
}

// TestEmbeddingModelForModality_NonTextNilIsOK verifies that a non-text modality
// with no SemanticModalities entry returns (nil, nil) — not an error.
func TestEmbeddingModelForModality_NonTextNilIsOK(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
	})
	tenant := &config.TenantConfig{ID: "t1"}

	got, err := h.embeddingModelForModality(context.Background(), tenant, "image")
	if err != nil {
		t.Errorf("unexpected error for non-text modality: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unconfigured non-text modality, got %q", got.Name)
	}
}

// ── makeCacheEmbedFn tests ────────────────────────────────────────────────────

// TestMakeCacheEmbedFn_UsesCacheEmbeddingModelFirst verifies that
// SemanticCache.EmbeddingModel takes priority in makeCacheEmbedFn.
func TestMakeCacheEmbedFn_UsesCacheEmbeddingModelFirst(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("cache-embed"),
		embeddingModel("routing-embed"),
	})
	tenant := &config.TenantConfig{
		ID: "t1",
		SemanticCache: config.SemanticCacheConfig{
			Enabled:        true,
			EmbeddingModel: "cache-embed",
		},
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{EmbeddingModel: "routing-embed"},
		},
	}

	fn := h.makeCacheEmbedFn(context.Background(), tenant, func(_, _ int, _ *config.ModelConfig) {})
	if fn == nil {
		t.Fatal("expected embedding function, got nil")
	}
}

// TestMakeCacheEmbedFn_FallsBackToRoutingSemanticModel verifies that when
// SemanticCache.EmbeddingModel is empty, routing.semantic.embedding_model is used.
func TestMakeCacheEmbedFn_FallsBackToRoutingSemanticModel(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("routing-embed"),
	})
	tenant := &config.TenantConfig{
		ID: "t1",
		SemanticCache: config.SemanticCacheConfig{
			Enabled:        true,
			EmbeddingModel: "", // not set
		},
		Routing: config.RoutingConfig{
			Semantic: config.SemanticRoutingConfig{EmbeddingModel: "routing-embed"},
		},
	}

	fn := h.makeCacheEmbedFn(context.Background(), tenant, func(_, _ int, _ *config.ModelConfig) {})
	if fn == nil {
		t.Fatal("expected embedding function from routing.semantic.embedding_model, got nil")
	}
}

// TestMakeCacheEmbedFn_NilWhenNeitherConfigured verifies Phase 2:
// no cache embedding model AND no routing embedding model → nil function (no global fallback).
func TestMakeCacheEmbedFn_NilWhenNeitherConfigured(t *testing.T) {
	h := embModelHandlers([]config.ModelConfig{
		embeddingModel("global-embed"),
	})
	tenant := &config.TenantConfig{
		ID: "t1",
		SemanticCache: config.SemanticCacheConfig{
			Enabled:        true,
			EmbeddingModel: "",
		},
		// routing.semantic.embedding_model not set
	}

	fn := h.makeCacheEmbedFn(context.Background(), tenant, func(_, _ int, _ *config.ModelConfig) {})
	if fn != nil {
		t.Error("expected nil function when neither cache nor routing embedding model is configured")
	}
}
