// Package benchmarking implements automatic operational benchmarking of LLM models.
// It measures latency, success rate, token usage and estimated cost for each
// configured chat-capable model by periodically sending a lightweight probe request.
package benchmarking

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)


// Runner executes a single round of benchmarks across eligible models.
type Runner struct {
	cfg             *config.Config
	store           storage.Storage
	registry        *providers.Registry
	log             *slog.Logger
	globalCfgCache  *config.GlobalConfigCache
}

// NewRunner creates a Runner.
func NewRunner(cfg *config.Config, store storage.Storage, registry *providers.Registry, log *slog.Logger, globalCfgCache *config.GlobalConfigCache) *Runner {
	return &Runner{cfg: cfg, store: store, registry: registry, log: log, globalCfgCache: globalCfgCache}
}

// Run benchmarks all eligible models, respecting max_concurrency.
// Errors from individual model benchmarks are logged but do not abort the run.
func (r *Runner) Run(ctx context.Context) {
	models := r.eligibleModels()
	if len(models) == 0 {
		r.log.InfoContext(ctx, "model benchmark started: no eligible models")
		return
	}

	r.log.InfoContext(ctx, "model benchmark started", "eligible_models", len(models))

	maxConc := r.cfg.Benchmarking.MaxConcurrency
	if maxConc <= 0 {
		maxConc = 2
	}

	sem := make(chan struct{}, maxConc)
	done := make(chan struct{}, len(models))

	for _, m := range models {
		m := m // capture
		sem <- struct{}{}
		go func() {
			defer func() {
				<-sem
				done <- struct{}{}
			}()
			r.benchmarkModel(ctx, m)
		}()
	}

	// Wait for all goroutines to finish.
	for range models {
		<-done
	}

	r.log.InfoContext(ctx, "model benchmark completed", "eligible_models", len(models))
}

// eligibleModels returns chat-capable models that have a provider registered.
// It prefers the static YAML config; when that is empty (dynamic-config deployments)
// it falls back to the GlobalConfig models resolved from the cache/DB.
func (r *Runner) eligibleModels() []config.ModelConfig {
	models := r.cfg.Models
	if len(models) == 0 && r.globalCfgCache != nil {
		ctx := context.Background()
		gc, err := r.cfg.ResolveGlobalConfig(ctx, r.globalCfgCache, r.store, r.store, r.store, r.log)
		if err == nil && gc != nil {
			models = gc.Models
		}
	}

	var out []config.ModelConfig
	for _, m := range models {
		if m.Type == "embedding" {
			continue
		}
		if m.Mock.Enabled {
			// Skip mock-only models in production benchmarking.
			continue
		}
		if _, ok := r.registry.Get(m.Provider); !ok {
			// No live provider available — skip.
			continue
		}
		out = append(out, m)
	}
	return out
}

// providerForModel resolves the correct Provider for a model, honouring per-model BaseURL overrides.
// When m.BaseURL is set it builds a one-off provider instance (same credentials, different endpoint)
// so that multiple local models on different ports are benchmarked correctly.
func (r *Runner) providerForModel(ctx context.Context, m config.ModelConfig) (providers.Provider, bool) {
	if m.BaseURL == "" {
		return r.registry.Get(m.Provider)
	}
	// Resolve provider config: global (DB) first, then YAML.
	var pc *config.ProviderConfig
	if r.globalCfgCache != nil {
		gc, err := r.cfg.ResolveGlobalConfig(ctx, r.globalCfgCache, r.store, r.store, r.store, r.log)
		if err == nil && gc != nil {
			if found, ok := gc.Providers[m.Provider]; ok {
				pc = &found
			}
		}
	}
	if pc == nil {
		if found, ok := r.cfg.Providers[m.Provider]; ok {
			pc = &found
		}
	}
	if pc == nil {
		return nil, false
	}
	overridden := *pc
	overridden.BaseURL = m.BaseURL
	p, _, err := providers.ProviderPairFromConfig(overridden)
	if err != nil || p == nil {
		return nil, false
	}
	return p, true
}

// benchmarkModel runs one benchmark request for the given model and persists the result.
func (r *Runner) benchmarkModel(ctx context.Context, m config.ModelConfig) {
	timeoutMs := r.cfg.Benchmarking.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	bCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	prov, ok := r.providerForModel(ctx, m)
	if !ok {
		r.log.WarnContext(ctx, "model benchmark failed: provider not found",
			"model", m.Name, "provider", m.Provider)
		return
	}

	msgs := r.buildMessages()
	req := providers.ChatRequest{
		Model:    m.Name,
		Messages: msgs,
	}

	start := time.Now()
	resp, err := prov.ChatCompletion(bCtx, req)
	latencyMs := time.Since(start).Milliseconds()

	row := storage.ModelBenchmarkRow{
		ID:            uuid.New(),
		Ts:            time.Now().UTC(),
		Provider:      m.Provider,
		Model:         m.Name,
		LatencyMs:     latencyMs,
		BenchmarkName: "default",
	}

	if err != nil {
		row.Success = false
		row.ErrorType = classifyError(err)
		r.log.WarnContext(ctx, "model benchmark failed",
			"model", m.Name, "provider", m.Provider,
			"latency_ms", latencyMs, "error", err)
	} else {
		row.Success = true
		if resp != nil {
			row.PromptTokens = int64(resp.Usage.PromptTokens)
			row.CompletionTokens = int64(resp.Usage.CompletionTokens)
			row.TotalTokens = int64(resp.Usage.TotalTokens)
			row.CostUSD = estimateCost(m.Pricing, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
		}
	}

	if storeErr := r.store.InsertModelBenchmark(ctx, row); storeErr != nil {
		r.log.ErrorContext(ctx, "model benchmark: failed to store result",
			"model", m.Name, "error", storeErr)
		return
	}

	if row.Success {
		r.log.InfoContext(ctx, "model benchmark completed",
			"model", m.Name, "provider", m.Provider,
			"latency_ms", latencyMs,
			"prompt_tokens", row.PromptTokens,
			"cost_usd", fmt.Sprintf("%.6f", row.CostUSD))
	}
}

// buildMessages converts the YAML benchmark messages into provider messages.
func (r *Runner) buildMessages() []providers.ChatMessage {
	var out []providers.ChatMessage
	for _, m := range r.cfg.Benchmarking.Request.Messages {
		out = append(out, providers.ChatMessage{Role: m.Role, Content: m.Content})
	}
	if len(out) == 0 {
		out = []providers.ChatMessage{{Role: "user", Content: "Say hello in one short sentence."}}
	}
	return out
}

// classifyError maps an error to a normalized category.
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case contains(msg, "timeout") || contains(msg, "context deadline exceeded") || contains(msg, "deadline"):
		return "timeout"
	case contains(msg, "401") || contains(msg, "403") || contains(msg, "unauthorized") || contains(msg, "authentication"):
		return "auth_error"
	case contains(msg, "429") || contains(msg, "rate limit") || contains(msg, "too many requests"):
		return "rate_limit"
	case contains(msg, "502") || contains(msg, "503") || contains(msg, "504") || contains(msg, "upstream"):
		return "upstream_error"
	default:
		return "unknown"
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// estimateCost computes USD cost from token counts and model pricing.
func estimateCost(p config.Pricing, promptTokens, completionTokens int) float64 {
	return float64(promptTokens)/1_000_000*p.PromptPer1M +
		float64(completionTokens)/1_000_000*p.CompletionPer1M
}
