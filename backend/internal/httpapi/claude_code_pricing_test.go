package httpapi

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

const costEps = 1e-12 // tolerance for Claude Code pricing float64 comparisons

func pricingApproxEqual(a, b float64) bool {
	return math.Abs(a-b) < costEps
}

// pricingHandlerWithGlobal builds a minimal Handlers with a pre-seeded GlobalConfigCache
// containing the given ClaudeCodePricing map. testConfig() has DynamicConfig.Enabled=false,
// so resolveGlobalConfig falls back to cfg.GlobalConfig — but we bypass that by seeding
// the cache directly so the pricing is available without a running DB.
func pricingHandlerWithGlobal(pricing map[string]config.ClaudeCodeFamilyPricing) *Handlers {
	h := &Handlers{
		cfg:            testConfig(),
		store:          &fakeStorage{},
		log:            testLogger(),
		globalCfgCache: config.NewGlobalConfigCache(time.Hour),
	}
	gc := &config.GlobalConfig{
		ClaudeCodePricing: pricing,
	}
	h.globalCfgCache.Set(gc, 1)
	return h
}

// ── 1. sonnet model + pricing → cost computed ────────────────────────────────
// Prices are USD per 1,000,000 tokens: input=$3, output=$15.

func TestClaudeCodePricing_Sonnet_CostComputed(t *testing.T) {
	h := pricingHandlerWithGlobal(map[string]config.ClaudeCodeFamilyPricing{
		"sonnet": {Input: 3.0, Output: 15.0},
	})
	model := "claude-sonnet-4-6"
	cost := h.computeAnthropicCost(context.Background(), &model, nil, 1000, 500)
	if cost == nil {
		t.Fatal("expected non-nil cost for sonnet")
	}
	// (1000/1M)*3.0 + (500/1M)*15.0 = 0.000003 + 0.0000075 = 0.0000105
	want := (1000.0/1_000_000.0)*3.0 + (500.0/1_000_000.0)*15.0
	if !pricingApproxEqual(*cost, want) {
		t.Errorf("cost = %v, want %v", *cost, want)
	}
}

// ── 2. opus model + pricing → cost computed ──────────────────────────────────

func TestClaudeCodePricing_Opus_CostComputed(t *testing.T) {
	h := pricingHandlerWithGlobal(map[string]config.ClaudeCodeFamilyPricing{
		"opus": {Input: 15.0, Output: 75.0},
	})
	model := "claude-opus-4-7"
	cost := h.computeAnthropicCost(context.Background(), &model, nil, 2000, 800)
	if cost == nil {
		t.Fatal("expected non-nil cost for opus")
	}
	want := (2000.0/1_000_000.0)*15.0 + (800.0/1_000_000.0)*75.0
	if !pricingApproxEqual(*cost, want) {
		t.Errorf("cost = %v, want %v", *cost, want)
	}
}

// ── 3. haiku model + pricing → cost computed ─────────────────────────────────

func TestClaudeCodePricing_Haiku_CostComputed(t *testing.T) {
	h := pricingHandlerWithGlobal(map[string]config.ClaudeCodeFamilyPricing{
		"haiku": {Input: 0.8, Output: 1.0},
	})
	model := "claude-haiku-4-5-20251001"
	cost := h.computeAnthropicCost(context.Background(), &model, nil, 500, 200)
	if cost == nil {
		t.Fatal("expected non-nil cost for haiku")
	}
	want := (500.0/1_000_000.0)*0.8 + (200.0/1_000_000.0)*1.0
	if !pricingApproxEqual(*cost, want) {
		t.Errorf("cost = %v, want %v", *cost, want)
	}
}

// ── 4. model family not in pricing map → cost NULL ───────────────────────────

func TestClaudeCodePricing_FamilyNotInMap_Nil(t *testing.T) {
	// Only haiku is configured; we send a sonnet request.
	h := pricingHandlerWithGlobal(map[string]config.ClaudeCodeFamilyPricing{
		"haiku": {Input: 0.8, Output: 1.0},
	})
	model := "claude-sonnet-4-6"
	cost := h.computeAnthropicCost(context.Background(), &model, nil, 1000, 500)
	if cost != nil {
		t.Errorf("expected nil cost when family not in pricing, got %v", *cost)
	}
}

// ── 5. unknown model (no family substring) → cost NULL ───────────────────────

func TestClaudeCodePricing_UnknownModel_Nil(t *testing.T) {
	h := pricingHandlerWithGlobal(map[string]config.ClaudeCodeFamilyPricing{
		"sonnet": {Input: 3.0, Output: 15.0},
		"opus":   {Input: 15.0, Output: 75.0},
		"haiku":  {Input: 0.8, Output: 1.0},
	})
	model := "gpt-4o-mini"
	cost := h.computeAnthropicCost(context.Background(), &model, nil, 1000, 500)
	if cost != nil {
		t.Errorf("expected nil cost for unknown model, got %v", *cost)
	}
}

// ── 6. tokens missing (both zero) → cost NULL ────────────────────────────────

func TestClaudeCodePricing_ZeroTokens_Nil(t *testing.T) {
	h := pricingHandlerWithGlobal(map[string]config.ClaudeCodeFamilyPricing{
		"sonnet": {Input: 3.0, Output: 15.0},
	})
	model := "claude-sonnet-4-6"
	cost := h.computeAnthropicCost(context.Background(), &model, nil, 0, 0)
	if cost != nil {
		t.Errorf("expected nil cost when tokens are zero, got %v", *cost)
	}
}

// ── 7. pricing values are zero → cost NULL ───────────────────────────────────

func TestClaudeCodePricing_ZeroPricing_Nil(t *testing.T) {
	h := pricingHandlerWithGlobal(map[string]config.ClaudeCodeFamilyPricing{
		"sonnet": {Input: 0, Output: 0},
	})
	model := "claude-sonnet-4-6"
	cost := h.computeAnthropicCost(context.Background(), &model, nil, 1000, 500)
	if cost != nil {
		t.Errorf("expected nil cost when pricing is zero, got %v", *cost)
	}
}

// ── 8. family resolution is case-insensitive ──────────────────────────────────

func TestClaudeCodePricing_FamilyResolution_CaseInsensitive(t *testing.T) {
	h := pricingHandlerWithGlobal(map[string]config.ClaudeCodeFamilyPricing{
		"sonnet": {Input: 3.0, Output: 15.0},
	})
	// Model name has uppercase letters — family detection must still work.
	model := "Claude-Sonnet-4-6"
	cost := h.computeAnthropicCost(context.Background(), &model, nil, 1000, 500)
	if cost == nil {
		t.Fatal("expected non-nil cost for uppercase model name")
	}
	want := (1000.0/1_000_000.0)*3.0 + (500.0/1_000_000.0)*15.0
	if !pricingApproxEqual(*cost, want) {
		t.Errorf("cost = %v, want %v", *cost, want)
	}
}

// ── Unit test: claudeCodeModelFamily ─────────────────────────────────────────

func TestClaudeCodeModelFamily(t *testing.T) {
	cases := []struct {
		model  string
		family string
	}{
		{"claude-sonnet-4-6", "sonnet"},
		{"claude-opus-4-7", "opus"},
		{"claude-haiku-4-5-20251001", "haiku"},
		{"Claude-Sonnet-4-6", "sonnet"}, // uppercase
		{"CLAUDE-HAIKU-3", "haiku"},     // fully uppercase
		{"gpt-4o-mini", ""},             // no family
		{"", ""},                        // empty
	}
	for _, tc := range cases {
		got := claudeCodeModelFamily(tc.model)
		if got != tc.family {
			t.Errorf("claudeCodeModelFamily(%q) = %q, want %q", tc.model, got, tc.family)
		}
	}
}
