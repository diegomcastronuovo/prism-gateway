package httpapi

import (
	"context"
	"maps"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// resolveProviderConfig returns provider config from global (DB) first, then YAML.
func (h *Handlers) resolveProviderConfig(ctx context.Context, name string) *config.ProviderConfig {
	gc := h.resolveGlobalConfig(ctx)
	if gc != nil && gc.Providers != nil {
		if pc, ok := gc.Providers[name]; ok {
			return &pc
		}
	}
	if pc, ok := h.cfg.Providers[name]; ok {
		return &pc
	}
	return nil
}

func (h *Handlers) invalidateGlobalConfigCache() {
	if h.globalCfgCache != nil {
		h.globalCfgCache.Invalidate()
	}
	h.clearLazyProviders()
	h.refreshRegistryProvidersFromMergedConfig(context.Background())
}

// refreshRegistryProvidersFromMergedConfig re-instantiates all chat/embedding providers from
// YAML cfg overlaid with the active global config so credential updates (e.g. Bedrock) take
// effect without restarting the process.
func (h *Handlers) refreshRegistryProvidersFromMergedConfig(ctx context.Context) {
	if h.registry == nil {
		return
	}
	gc := h.resolveGlobalConfig(ctx)
	merged := maps.Clone(h.cfg.Providers)
	if merged == nil {
		merged = make(map[string]config.ProviderConfig)
	}
	if gc != nil && gc.Providers != nil {
		for name, pc := range gc.Providers {
			merged[name] = pc
		}
	}
	for name, pc := range merged {
		if err := providers.RegisterOne(h.registry, name, pc); err != nil && h.log != nil {
			h.log.WarnContext(ctx, "registry refresh: re-register provider failed", "provider", name, "error", err)
		}
	}
}

func (h *Handlers) clearLazyProviders() {
	h.lazyProvMu.Lock()
	defer h.lazyProvMu.Unlock()
	h.lazyChatProv = nil
	h.lazyEmbProv = nil
}

// providerForChatModel is like providerForChat but honours the model-level BaseURL override.
// When modelCfg.BaseURL is set it builds a one-off provider instance using the provider's
// type/credentials but with the model's URL, so multiple local models can run on different
// endpoints without requiring a separate provider entry for each one.
func (h *Handlers) providerForChatModel(ctx context.Context, modelCfg *config.ModelConfig) (providers.Provider, bool) {
	if modelCfg.BaseURL == "" {
		return h.providerForChat(ctx, modelCfg.Provider)
	}
	pc := h.resolveProviderConfig(ctx, modelCfg.Provider)
	if pc == nil {
		return nil, false
	}
	overridden := *pc
	overridden.BaseURL = modelCfg.BaseURL
	p, _, err := providers.ProviderPairFromConfig(overridden)
	if err != nil || p == nil {
		return nil, false
	}
	return p, true
}

// providerForChat returns a registered chat provider, or lazy-builds one from merged config
// (for providers added in Postgres after process start).
func (h *Handlers) providerForChat(ctx context.Context, name string) (providers.Provider, bool) {
	if p, ok := h.registry.Get(name); ok {
		return p, true
	}
	pc := h.resolveProviderConfig(ctx, name)
	if pc == nil {
		return nil, false
	}
	h.lazyProvMu.Lock()
	defer h.lazyProvMu.Unlock()
	if h.lazyChatProv != nil {
		if p, ok := h.lazyChatProv[name]; ok {
			return p, true
		}
	}
	p, _, err := providers.ProviderPairFromConfig(*pc)
	if err != nil || p == nil {
		return nil, false
	}
	if h.lazyChatProv == nil {
		h.lazyChatProv = make(map[string]providers.Provider)
	}
	h.lazyChatProv[name] = p
	return p, true
}

// embeddingProviderFor returns a registered embedding provider, or lazy-builds from config.
func (h *Handlers) embeddingProviderFor(ctx context.Context, name string) (providers.EmbeddingProvider, bool) {
	if e, ok := h.registry.GetEmbedding(name); ok {
		return e, true
	}
	pc := h.resolveProviderConfig(ctx, name)
	if pc == nil {
		return nil, false
	}
	h.lazyProvMu.Lock()
	defer h.lazyProvMu.Unlock()
	if h.lazyEmbProv != nil {
		if e, ok := h.lazyEmbProv[name]; ok {
			return e, true
		}
	}
	_, emb, err := providers.ProviderPairFromConfig(*pc)
	if err != nil || emb == nil {
		return nil, false
	}
	if h.lazyEmbProv == nil {
		h.lazyEmbProv = make(map[string]providers.EmbeddingProvider)
	}
	h.lazyEmbProv[name] = emb
	return emb, true
}
