package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// FeatureRuntimeConfig holds the runtime enable flag for a named feature.
// Used in GlobalConfig.Features to gate features like decision_ops at runtime.
type FeatureRuntimeConfig struct {
	Enabled bool `json:"enabled"`
}

// GlobalConfig is the dynamic subset of Config that lives in Postgres.
// It mirrors the YAML structure for auth, providers, models, circuit_breaker,
// rate_limit and smart_routing — intentionally excludes server settings and tenant configs.
type GlobalConfig struct {
	Auth                *AuthConfig                         `json:"auth,omitempty"`
	Providers           map[string]ProviderConfig           `json:"providers"`
	Models              []ModelConfig                       `json:"models"`
	CircuitBreaker      CircuitBreakerConfig                `json:"circuit_breaker"`
	RateLimit           GlobalRateLimitConfig               `json:"rate_limit"`
	SmartRouting        SmartRoutingConfig                  `json:"smart_routing"`
	ConversationLogging ConversationLoggingConfig           `json:"conversation_logging"`
	ClaudeCodePricing   map[string]ClaudeCodeFamilyPricing `json:"claude_code_pricing,omitempty"`
	// Features holds per-feature runtime enable flags (e.g. decision_ops).
	Features map[string]FeatureRuntimeConfig `json:"features,omitempty"`
	// WorkflowConversationTTLSeconds is the inactivity TTL for workflow_conversations rows (SPEC_169).
	// Default: 3600 (1 hour). Changes take effect within the GlobalConfigCache TTL (≤5s).
	WorkflowConversationTTLSeconds int `json:"workflow_conversation_ttl_seconds,omitempty"`
	// WorkflowSnapshotRetentionDays controls how long workflow_conversation_snapshots are kept (SPEC_173).
	// Default: 90 days. Set to 0 to disable snapshot retention cleanup.
	WorkflowSnapshotRetentionDays int `json:"workflow_snapshot_retention_days,omitempty"`
	// ToolPricing is loaded at cache-refresh time from tool_catalog. Not stored in DB.
	// Keyed by "provider/id" (e.g. "openai/web_search_standard").
	ToolPricing map[string]ToolPricingEntry `json:"-"`
}

// ToolPricingEntry holds the pricing for a single tool catalog entry at inference time.
type ToolPricingEntry struct {
	ToolType     string  // e.g. "web_search", "container", "function"
	PricePerUnit float64 // price per single use (call/session/gb_day)
	Unit         string  // "call" | "session" | "gb_day"
}

// ToolCatalogEnricher loads tool catalog pricing for GlobalConfig enrichment.
type ToolCatalogEnricher interface {
	ListToolCatalogPricing(ctx context.Context) ([]ToolCatalogPricingRow, error)
}

// ToolCatalogPricingRow is a minimal projection of tool_catalog for inference-time pricing.
type ToolCatalogPricingRow struct {
	Provider     string
	ID           string
	ToolType     string
	PricePerUnit float64
	Unit         string
}

// ClaudeCodeFamilyPricing holds per-million-token prices for a Claude model family.
// Used for pricing configuration of the Claude Code integration (SPEC_160).
type ClaudeCodeFamilyPricing struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// GlobalConfigStorage is the minimal storage interface needed for global config resolution.
// Kept separate from config.Storage to allow independent evolution.
type GlobalConfigStorage interface {
	GetGlobalConfig(ctx context.Context) (json.RawMessage, int, bool, error)
}

// CatalogPricingRow is a minimal projection of model_catalog used for pricing
// enrichment at config-load time. Uses only primitive types to avoid import cycles.
type CatalogPricingRow struct {
	Provider                    string
	ID                          string
	CachedInputPer1M            float64
	LongContext                 bool
	LongContextStartTokens      int
	LongContextPromptPer1M      float64
	LongContextCachedInputPer1M float64
	LongContextCompletionPer1M  float64
	CacheWrite5mPer1M           float64
	CacheWrite1hPer1M           float64
	GeoMultiplierUS             float64
}

// CatalogEnricher is implemented by *storage.PostgresStorage and used to enrich
// ModelConfig.Pricing from model_catalog at cache-load time (not per request).
type CatalogEnricher interface {
	ListCatalogPricing(ctx context.Context) ([]CatalogPricingRow, error)
}

// GlobalConfigFromYAML builds a GlobalConfig from the loaded YAML Config.
// Auth is included only when the YAML defines it (non-empty mode).
func GlobalConfigFromYAML(c *Config) *GlobalConfig {
	var auth *AuthConfig
	if c.Auth.Mode != "" {
		a := c.Auth
		auth = &a
	}
	return &GlobalConfig{
		Auth:           auth,
		Providers:      c.Providers,
		Models:         c.Models,
		CircuitBreaker: c.CircuitBreaker,
		RateLimit:      c.RateLimit,
		SmartRouting:   c.SmartRouting,
		ConversationLogging: c.ConversationLogging,
	}
}

// ResolveGlobalConfig resolves the global config using cache → DB → YAML fallback.
// DB is only consulted when DynamicConfig.Enabled is true.
// Never returns nil; always falls back to YAML.
// enricher is optional (nil = skip catalog pricing enrichment).
// toolEnricher is optional (nil = skip tool pricing enrichment).
func (c *Config) ResolveGlobalConfig(
	ctx context.Context,
	cache *GlobalConfigCache,
	store GlobalConfigStorage,
	enricher CatalogEnricher,
	toolEnricher ToolCatalogEnricher,
	log *slog.Logger,
) (*GlobalConfig, error) {
	// 1. Try cache (hot path)
	if cache != nil {
		if gc, _, ok := cache.Get(); ok {
			return gc, nil
		}
	}

	// 2. Try DB (only when dynamic config is enabled and a store is provided)
	if store != nil && c.DynamicConfig.Enabled {
		raw, version, exists, err := store.GetGlobalConfig(ctx)
		if err != nil {
			if log != nil {
				log.WarnContext(ctx, "global config db error, falling back to YAML", "error", err)
			}
		} else if exists {
			var gc GlobalConfig
			if err := json.Unmarshal(raw, &gc); err != nil {
				return nil, fmt.Errorf("unmarshal global config: %w", err)
			}

			warnings := ValidateGlobalConfig(&gc)
			if log != nil {
				for _, w := range warnings {
					log.WarnContext(ctx, "global config validation warning", "warning", w)
				}
			}

			if enricher != nil {
				enrichPricingFromCatalog(ctx, &gc, enricher)
			}

			if toolEnricher != nil {
				gc.ToolPricing = buildToolPricingIndex(ctx, toolEnricher)
			}

			if cache != nil {
				cache.Set(&gc, version)
			}
			return &gc, nil
		}
	}

	// 3. Fallback: build from YAML
	gc := GlobalConfigFromYAML(c)
	if enricher != nil {
		enrichPricingFromCatalog(ctx, gc, enricher)
	}
	if toolEnricher != nil {
		gc.ToolPricing = buildToolPricingIndex(ctx, toolEnricher)
	}
	return gc, nil
}

// buildToolPricingIndex builds a map from "provider/id" to ToolPricingEntry.
func buildToolPricingIndex(ctx context.Context, enricher ToolCatalogEnricher) map[string]ToolPricingEntry {
	rows, err := enricher.ListToolCatalogPricing(ctx)
	if err != nil || len(rows) == 0 {
		return nil
	}
	index := make(map[string]ToolPricingEntry, len(rows))
	for _, r := range rows {
		index[r.Provider+"/"+r.ID] = ToolPricingEntry{
			ToolType:     r.ToolType,
			PricePerUnit: r.PricePerUnit,
			Unit:         r.Unit,
		}
	}
	return index
}

// enrichPricingFromCatalog overlays model_catalog pricing onto ModelConfig.Pricing
// for fields that are not already set in the config.
// Only fields that are zero-valued in the existing Pricing are filled from catalog.
func enrichPricingFromCatalog(ctx context.Context, gc *GlobalConfig, enricher CatalogEnricher) {
	rows, err := enricher.ListCatalogPricing(ctx)
	if err != nil || len(rows) == 0 {
		return
	}
	// Build lookup: "provider/id" → row
	index := make(map[string]CatalogPricingRow, len(rows))
	for _, r := range rows {
		index[r.Provider+"/"+r.ID] = r
	}
	for i := range gc.Models {
		m := &gc.Models[i]
		key := m.Provider + "/" + m.Name
		row, ok := index[key]
		if !ok {
			continue
		}
		// Only fill zero-valued fields; explicit config values take priority.
		if m.Pricing.CachedInputPer1M == 0 {
			m.Pricing.CachedInputPer1M = row.CachedInputPer1M
		}
		if !m.Pricing.LongContext {
			m.Pricing.LongContext = row.LongContext
		}
		if m.Pricing.LongContextStartTokens == 0 {
			m.Pricing.LongContextStartTokens = row.LongContextStartTokens
		}
		if m.Pricing.LongContextPromptPer1M == 0 {
			m.Pricing.LongContextPromptPer1M = row.LongContextPromptPer1M
		}
		if m.Pricing.LongContextCachedInputPer1M == 0 {
			m.Pricing.LongContextCachedInputPer1M = row.LongContextCachedInputPer1M
		}
		if m.Pricing.LongContextCompletionPer1M == 0 {
			m.Pricing.LongContextCompletionPer1M = row.LongContextCompletionPer1M
		}
		if m.Pricing.CacheWrite5mPer1M == 0 {
			m.Pricing.CacheWrite5mPer1M = row.CacheWrite5mPer1M
		}
		if m.Pricing.CacheWrite1hPer1M == 0 {
			m.Pricing.CacheWrite1hPer1M = row.CacheWrite1hPer1M
		}
		if m.Pricing.GeoMultiplierUS == 0 {
			m.Pricing.GeoMultiplierUS = row.GeoMultiplierUS
		}
	}
}

// ValidateGlobalConfig checks cross-config constraints and returns a list of warnings.
// Warnings do not prevent startup; they are logged and the config is used as-is.
func ValidateGlobalConfig(gc *GlobalConfig) []string {
	var warnings []string

	namesSeen := make(map[string]bool, len(gc.Models))
	for _, m := range gc.Models {
		// Unique name check
		if namesSeen[m.Name] {
			warnings = append(warnings, fmt.Sprintf("duplicate model name: %q", m.Name))
		}
		namesSeen[m.Name] = true

		// Provider existence check
		if _, ok := gc.Providers[m.Provider]; !ok {
			warnings = append(warnings, fmt.Sprintf("model %q references unknown provider %q", m.Name, m.Provider))
		}

		// Pricing required for smart scoring
		if m.Pricing.PromptPer1M == 0 && m.Pricing.CompletionPer1M == 0 {
			warnings = append(warnings, fmt.Sprintf("model %q has no pricing (smart scoring will use 0)", m.Name))
		}

		// Type default
		if m.Type == "" {
			// Not a warning — empty means "chat", handled transparently
		}
	}

	return warnings
}

// ModelByName returns the ModelConfig for the given name, or nil if not found.
func (gc *GlobalConfig) ModelByName(name string) *ModelConfig {
	for i := range gc.Models {
		if gc.Models[i].Name == name {
			return &gc.Models[i]
		}
	}
	return nil
}

// AllowedModelsForTenant returns the global model configs that are in the tenant's allowed list.
func (gc *GlobalConfig) AllowedModelsForTenant(t *TenantConfig) []ModelConfig {
	allowed := make(map[string]bool, len(t.AllowedModels))
	for _, m := range t.AllowedModels {
		allowed[m] = true
	}
	var result []ModelConfig
	for _, m := range gc.Models {
		if allowed[m.Name] {
			result = append(result, m)
		}
	}
	return result
}
