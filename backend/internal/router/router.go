package router

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// Request holds the inputs the router needs to select a model.
type Request struct {
	TenantID          string
	Strategy          string
	Candidates        []config.ModelConfig // pre-filtered allowed models
	ForcedModel       string               // from request body "model" or X-Model header
	RouteGroup        string               // from X-Route-Group header (explicit override)
	DefaultRouteGroup string               // from tenant.Routing.RouteGroup (default pool; overridden by semantic anchor)
	FailModels        map[string]bool      // models to skip (debug / fallback simulation)
	RouteGroups       map[string][]string  // tenant's route_groups config

	// For smart routing
	SmartConfig *config.SmartConfig
	Messages    []string

	// For semantic routing (nil means disabled; only set when stages contain semantic_similarity rules)
	Ctx                      context.Context
	EmbeddingFunc            func(ctx context.Context, text string) ([]float64, error)
	SemanticLookup           SemanticLookupFunc
	SemanticThresholdDefault float64 // tenant.Routing.Semantic.ThresholdDefault; 0 means "not set"

	// Smart stage error context (populated by smartBased if error occurs)
	SmartBlockReason         string
	SmartBlockStage          string
	SmartBlockIsPromptLength bool
	SmartBlockPromptLength   int
	SmartBlockHasCustomReason bool
	SmartFilteredAll         bool
	SmartFilterContext       SmartStageResult
	SmartDefaultGroupBlocked bool // set by smartBased when DefaultRouteGroup yields empty pool

	// For cost-aware routing (optional; zero values mean "not set / fail-open")
	BudgetPressure float64 // current_spend / monthly_budget; 0 means unknown
	MaxTokens      *int    // from request max_tokens; used for completion cost estimation
}

// Result is the ordered list of candidate models the router produced.
type Result struct {
	Selected    string
	Candidates  []string          // ordered, including fallbacks
	SmartResult *SmartStageResult // non-nil when strategy=smart and at least one stage matched
}

// Router selects models based on tenant routing strategy.
type Router struct {
	mu           sync.Mutex
	rrIndex      map[string]int   // tenant -> round-robin index
	metricsStore MetricsStore     // pluggable EWMA + counter store
	statsCache   *ModelStatsCache // DB-based stats cache (nil for tests)
}

const ewmaAlpha = 0.3

// New creates a router with an in-memory MetricsStore and no DB stats cache.
// Used for tests and when no database is available.
func New() *Router {
	return &Router{
		rrIndex:      make(map[string]int),
		metricsStore: NewInMemoryMetricsStore(),
	}
}

// NewWithStorage creates a router with an in-memory MetricsStore and a DB-backed
// stats cache. This preserves the existing behavior when no Redis is configured.
func NewWithStorage(store storage.Storage) *Router {
	return &Router{
		rrIndex:      make(map[string]int),
		metricsStore: NewInMemoryMetricsStore(),
		statsCache:   NewModelStatsCache(store, 30*time.Second, 7),
	}
}

// NewWithMetricsStore creates a router with a caller-supplied MetricsStore
// (e.g. RedisMetricsStore) and an optional DB stats cache (pass nil to skip).
func NewWithMetricsStore(store storage.Storage, ms MetricsStore) *Router {
	var cache *ModelStatsCache
	if store != nil {
		cache = NewModelStatsCache(store, 30*time.Second, 7)
	}
	return &Router{
		rrIndex:      make(map[string]int),
		metricsStore: ms,
		statsCache:   cache,
	}
}

// Select returns an ordered list of candidate models based on the routing strategy.
func (rt *Router) Select(req Request) (Result, error) {
	candidates := req.Candidates

	// P-explicit: Explicit route group from header/alias — highest priority.
	// req.Candidates is updated so that smartBased also sees the filtered slice.
	if req.RouteGroup != "" && req.RouteGroups != nil {
		groupModels, ok := req.RouteGroups[req.RouteGroup]
		if ok {
			candidates = filterByNames(candidates, groupModels)
			req.Candidates = candidates // propagate to smartBased
		}
		// If group not found, keep all candidates (no restriction)
	}

	// P-semantic: Pre-strategy semantic anchor evaluation for non-smart strategies.
	// For smart: handled inside smartBased() as part of stage scoring.
	// For round_robin / cost / latency / header: evaluate semantic stages here so the
	// anchor route_group overrides the candidate pool before strategy dispatch.
	// Fail-open: any error (embedding call, DB lookup) leaves candidates unchanged.
	preSemanticAnchor := "" // name of matched anchor; "" means no match

	if req.Strategy != "smart" && req.EmbeddingFunc != nil && req.SemanticLookup != nil &&
		req.SmartConfig != nil && req.RouteGroup == "" {

		ctx := req.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		evaluator := NewStageEvaluator(*req.SmartConfig, req.Messages)
		evaluator.SetSemanticThresholdDefault(req.SemanticThresholdDefault)
		evaluator.SetSemanticDeps(ctx, req.TenantID, req.EmbeddingFunc, req.SemanticLookup)
		stageResult := evaluator.Evaluate()
		req.SmartFilterContext = stageResult
		preSemanticAnchor = stageResult.SemanticAnchor

		if stageResult.AnchorRouteGroup != "" && req.RouteGroups != nil {
			if groupModels, ok := req.RouteGroups[stageResult.AnchorRouteGroup]; ok {
				candidates = filterByNames(candidates, groupModels)
				req.Candidates = candidates
			}
			// Group not found: fail-open, keep current candidates.
		}
	}

	// P-default: Default route group from tenant routing.route_group.
	// Applied for non-smart strategies when no explicit header group is set
	// AND no semantic anchor matched (semantic match takes priority over default pool).
	// Smart strategy applies this inside smartBased() AFTER semantic anchor evaluation.
	if req.RouteGroup == "" && preSemanticAnchor == "" && req.DefaultRouteGroup != "" &&
		req.RouteGroups != nil && req.Strategy != "smart" {
		groupModels := req.RouteGroups[req.DefaultRouteGroup]
		filtered := filterByNames(candidates, groupModels)
		if len(filtered) == 0 {
			return Result{}, &ErrDefaultRouteGroupEmpty{RouteGroup: req.DefaultRouteGroup}
		}
		candidates = filtered
		req.Candidates = candidates
	}

	// If a model is forced (body or header), validate and return it
	if req.ForcedModel != "" {
		found := false
		for _, c := range candidates {
			if c.Name == req.ForcedModel {
				found = true
				break
			}
		}
		if !found {
			return Result{}, fmt.Errorf("model '%s' is not available in candidate set", req.ForcedModel)
		}
		// Forced model is primary; rest are fallback in original order
		ordered := []string{req.ForcedModel}
		for _, c := range candidates {
			if c.Name != req.ForcedModel {
				ordered = append(ordered, c.Name)
			}
		}
		ordered = removeFailModels(ordered, req.FailModels)
		if len(ordered) == 0 {
			return Result{}, fmt.Errorf("all candidate models are unavailable")
		}
		return Result{Selected: ordered[0], Candidates: ordered}, nil
	}

	if len(candidates) == 0 {
		return Result{}, fmt.Errorf("no candidate models available")
	}

	var ordered []string

	switch req.Strategy {
	case "round_robin":
		ordered = rt.roundRobin(req.TenantID, candidates)
	case "latency":
		ordered = rt.latencyBased(req.TenantID, candidates)
	case "cost":
		ordered = costBased(candidates)
	case "smart":
		ordered = rt.smartBased(&req)
	case "header":
		// header strategy without a forced model just uses original order
		ordered = modelNames(candidates)
	default:
		ordered = modelNames(candidates)
	}

	ordered = removeFailModels(ordered, req.FailModels)
	if len(ordered) == 0 {
		// Distinguish between different empty scenarios
		if req.SmartDefaultGroupBlocked {
			return Result{}, &ErrDefaultRouteGroupEmpty{RouteGroup: req.DefaultRouteGroup}
		}
		if req.SmartBlockReason != "" {
			// Populate SmartResult in the error Result so callers can log stage evaluation
			// details (e.g. RulesMatched) even when routing fails (SPEC_150).
			sc := req.SmartFilterContext
			return Result{SmartResult: &sc}, &ErrBlockedBySmartStage{
				Reason:          req.SmartBlockReason,
				Stage:           req.SmartBlockStage,
				IsPromptLength:  req.SmartBlockIsPromptLength,
				PromptLength:    req.SmartBlockPromptLength,
				HasCustomReason: req.SmartBlockHasCustomReason,
			}
		}
		if req.SmartFilteredAll {
			sc := req.SmartFilterContext
			return Result{SmartResult: &sc}, &ErrNoCandidatesAfterSmartStages{
				PreferredModels: req.SmartFilterContext.PreferredModels,
				BannedModels:    req.SmartFilterContext.BannedModels,
				ConstraintsUsed: req.SmartFilterContext.Constraints.MaxCostPer1M != nil ||
					req.SmartFilterContext.Constraints.MaxLatencyMs != nil,
			}
		}
		return Result{}, fmt.Errorf("all candidate models are unavailable")
	}

	// Propagate stage evaluation to Result for snapshot logging.
	// Set when strategy=smart (stage scored) OR any strategy with a semantic anchor match.
	var smartResult *SmartStageResult
	if (req.Strategy == "smart" &&
		(len(req.SmartFilterContext.StagesEvaluated) > 0 || req.SmartFilterContext.CostOptimizerApplied)) ||
		req.SmartFilterContext.SemanticAnchor != "" {
		sc := req.SmartFilterContext
		smartResult = &sc
	}

	return Result{Selected: ordered[0], Candidates: ordered, SmartResult: smartResult}, nil
}

// getFirstStage extracts the first stage name from evaluated stages
func getFirstStage(stages []string) string {
	if len(stages) > 0 {
		return stages[0]
	}
	return "unknown"
}

// RecordLatency updates the EWMA latency for a (tenant, model) pair.
func (rt *Router) RecordLatency(tenantID, model string, latencyMs float64) {
	_ = rt.metricsStore.UpdateLatencyEWMA(context.Background(), tenantID, model, latencyMs)
}

// GetLatency returns the current EWMA latency for a (tenant, model) pair.
// Returns (0, false) if no data is available.
func (rt *Router) GetLatency(tenantID, model string) (float64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	m, err := rt.metricsStore.GetLatencyEWMA(ctx, tenantID, []string{model})
	if err != nil {
		return 0, false
	}
	v, ok := m[model]
	return v, ok
}

// UpdateModelStats records a request outcome (success or error) for smart routing.
func (rt *Router) UpdateModelStats(tenantID, model string, success bool) {
	_ = rt.metricsStore.IncRequest(context.Background(), tenantID, model, !success)
}

func (rt *Router) roundRobin(tenantID string, candidates []config.ModelConfig) []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	idx := rt.rrIndex[tenantID] % len(candidates)
	rt.rrIndex[tenantID] = idx + 1

	// Build ordered list starting from idx
	ordered := make([]string, 0, len(candidates))
	for i := 0; i < len(candidates); i++ {
		ordered = append(ordered, candidates[(idx+i)%len(candidates)].Name)
	}
	return ordered
}

func (rt *Router) latencyBased(tenantID string, candidates []config.ModelConfig) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	names := modelNames(candidates)
	latencies, _ := rt.metricsStore.GetLatencyEWMA(ctx, tenantID, names)

	type entry struct {
		name    string
		latency float64
	}

	entries := make([]entry, 0, len(candidates))
	for _, c := range candidates {
		lat := math.MaxFloat64 // unknown = sort last
		if v, ok := latencies[c.Name]; ok {
			lat = v
		}
		entries = append(entries, entry{name: c.Name, latency: lat})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].latency < entries[j].latency
	})

	ordered := make([]string, len(entries))
	for i, e := range entries {
		ordered[i] = e.name
	}
	return ordered
}

// EstimateTokens provides a simple character-based token estimation.
func EstimateTokens(text string) int {
	// ~4 chars per token is a common rough heuristic
	t := len(text) / 4
	if t == 0 && len(text) > 0 {
		t = 1
	}
	return t
}

func costBased(candidates []config.ModelConfig) []string {
	type entry struct {
		name string
		cost float64
	}

	entries := make([]entry, 0, len(candidates))
	for _, c := range candidates {
		// Combined cost rate as a sorting key (prompt + completion per 1M tokens)
		cost := c.Pricing.PromptPer1M + c.Pricing.CompletionPer1M
		entries = append(entries, entry{name: c.Name, cost: cost})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].cost < entries[j].cost
	})

	ordered := make([]string, len(entries))
	for i, e := range entries {
		ordered[i] = e.name
	}
	return ordered
}

func modelNames(models []config.ModelConfig) []string {
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.Name
	}
	return names
}

func filterByNames(models []config.ModelConfig, names []string) []config.ModelConfig {
	allowed := make(map[string]bool, len(names))
	for _, n := range names {
		allowed[n] = true
	}
	var result []config.ModelConfig
	for _, m := range models {
		if allowed[m.Name] {
			result = append(result, m)
		}
	}
	return result
}

func removeFailModels(ordered []string, fail map[string]bool) []string {
	if len(fail) == 0 {
		return ordered
	}
	var result []string
	for _, m := range ordered {
		if !fail[m] {
			result = append(result, m)
		}
	}
	return result
}

// smartBased implements score-based routing with configurable weights and stages.
func (rt *Router) smartBased(req *Request) []string {
	type scored struct {
		name  string
		score float64
	}

	candidates := req.Candidates
	weights := req.SmartConfig.Weights

	// Apply stage evaluation (v2) or legacy rules
	evaluator := NewStageEvaluator(*req.SmartConfig, req.Messages)
	evaluator.SetSemanticThresholdDefault(req.SemanticThresholdDefault)
	if req.EmbeddingFunc != nil && req.SemanticLookup != nil {
		ctx := req.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		evaluator.SetSemanticDeps(ctx, req.TenantID, req.EmbeddingFunc, req.SemanticLookup)
	}
	stageResult := evaluator.Evaluate()

	// Always store stageResult so Select can propagate it to Result for snapshot logging.
	req.SmartFilterContext = stageResult

	// If blocked by stage, store error for caller
	if stageResult.Blocked {
		req.SmartBlockReason = stageResult.BlockReason
		req.SmartBlockStage = getFirstStage(stageResult.StagesEvaluated)
		req.SmartBlockIsPromptLength = stageResult.PromptLengthBlock
		req.SmartBlockPromptLength = stageResult.PromptLength
		req.SmartBlockHasCustomReason = stageResult.PromptLengthCustomReason
		return []string{}
	}

	// Apply route_group from semantic anchor (if set by use_anchor action).
	// Header route group (req.RouteGroup) takes precedence: only apply anchor group when no
	// explicit route group was provided via header. Select() already pre-filtered candidates
	// by the header group, so applying a different anchor group on top would incorrectly
	// intersect the two sets.
	if stageResult.AnchorRouteGroup != "" && req.RouteGroup == "" && req.RouteGroups != nil {
		if groupModels, ok := req.RouteGroups[stageResult.AnchorRouteGroup]; ok {
			candidates = filterByNames(candidates, groupModels)
		}
		// If group not found or RouteGroups nil: fail open, keep all candidates.
	} else if stageResult.SemanticAnchor == "" && req.DefaultRouteGroup != "" &&
		req.RouteGroup == "" && req.RouteGroups != nil {
		// No semantic anchor matched AND no explicit header group → apply default strategy route group.
		// NOTE: we check SemanticAnchor (not AnchorRouteGroup) so that a semantic match with
		// use_anchor=false (or an anchor without a route_group) does NOT incorrectly restrict
		// candidates to the default pool — the semantic result should still influence routing
		// via preferred_models. DefaultRouteGroup only applies when semantic produced nothing.
		// This is the SPEC_148 default pool: only candidates in routing.route_group ∩ allowed_models.
		groupModels := req.RouteGroups[req.DefaultRouteGroup]
		filtered := filterByNames(candidates, groupModels)
		if len(filtered) == 0 {
			// Config error: the default route group has no allowed candidates.
			req.SmartDefaultGroupBlocked = true
			return []string{}
		}
		candidates = filtered
	}

	// Apply stage filters (bans, constraints, preferred)
	originalCount := len(candidates)
	candidates = ApplyStageResult(candidates, stageResult)

	// If smart stages filtered all candidates, store context for error
	if len(candidates) == 0 && originalCount > 0 {
		req.SmartFilteredAll = true
		req.SmartFilterContext = stageResult
		return []string{}
	}

	// Single candidate: no scoring needed
	if len(candidates) == 1 {
		return []string{candidates[0].Name}
	}

	// Load DB-based stats (with cache)
	var dbStats map[string]*AggregatedStats
	if rt.statsCache != nil {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		var err error
		dbStats, err = rt.statsCache.GetStats(dbCtx, req.TenantID)
		dbCancel()
		if err != nil {
			dbStats = make(map[string]*AggregatedStats)
		}
	}

	// Load metrics from MetricsStore (in-memory or Redis) for models without DB stats.
	names := modelNames(candidates)
	meCtx, meCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	storeLatencies, _ := rt.metricsStore.GetLatencyEWMA(meCtx, req.TenantID, names)
	storeErrors, _ := rt.metricsStore.GetErrorStats(meCtx, req.TenantID, names)
	meCancel()

	// ── Cost optimizer: estimate actual request cost per candidate ───────────────
	// Estimate prompt tokens once for all candidates.
	promptTokens := EstimateTokens(strings.Join(req.Messages, " "))

	// Completion token estimate: prefer max_tokens from request, then config default, then 300.
	completionEst := 300
	if cfgEst := req.SmartConfig.CostOptimizer.DefaultCompletionTokensEst; cfgEst > 0 {
		completionEst = cfgEst
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		completionEst = *req.MaxTokens
	}

	// Budget pressure multiplier scales the cost weight when tenant is close to budget limits.
	pressureMult := budgetPressureMultiplier(req.BudgetPressure)
	effectiveCostWeight := weights.Cost * pressureMult

	// Pre-compute estimated costs for observability and scoring.
	estimatedCostsUSD := make(map[string]float64, len(candidates))
	costOptimizerApplied := false
	for _, c := range candidates {
		est := estimateRequestCostUSD(c.Pricing, promptTokens, completionEst)
		estimatedCostsUSD[c.Name] = est
		if est > 0 {
			costOptimizerApplied = true
		}
	}

	// Collect metrics for normalization
	type metrics struct {
		cost    float64
		latency float64
		errors  float64
	}
	type metricSrc struct {
		cost    string
		latency string
		errors  string
	}

	modelMetrics := make(map[string]metrics)
	modelSources := make(map[string]metricSrc, len(candidates))
	var costs, latencies, errorRates []float64

	for _, c := range candidates {
		m := metrics{
			// Use estimated request cost when available; fall back to rate sum (fail-open).
			cost:    estimatedCostsUSD[c.Name],
			latency: 2000.0, // default when no data
			errors:  0.0,
		}
		src := metricSrc{latency: "default_no_history", errors: "default_no_history"}

		// If no pricing configured (cost=0), fall back to rate sum for relative ordering.
		if m.cost == 0 {
			m.cost = c.Pricing.PromptPer1M + c.Pricing.CompletionPer1M
			src.cost = "rate_sum_fallback"
		} else {
			src.cost = "estimated"
		}

		// DB stats take priority (richer, aggregated data).
		if dbStat, ok := dbStats[c.Name]; ok && dbStat.RequestCount > 0 {
			m.errors = dbStat.ErrorRate
			src.errors = "db_stats"
			if dbStat.AvgLatencyMs > 0 {
				m.latency = dbStat.AvgLatencyMs
				src.latency = "db_stats"
			}
		} else {
			// Fall back to MetricsStore (EWMA + counters).
			if lat, ok := storeLatencies[c.Name]; ok {
				m.latency = lat
				src.latency = "ewma_store"
			}
			if es, ok := storeErrors[c.Name]; ok && es.RequestCount > 0 {
				m.errors = es.ErrorRate
				src.errors = "ewma_store"
			}
		}

		modelMetrics[c.Name] = m
		modelSources[c.Name] = src
		costs = append(costs, m.cost)
		latencies = append(latencies, m.latency)
		errorRates = append(errorRates, m.errors)
	}

	// Safe normalization
	costNorm := normalizeMetrics(costs)
	latencyNorm := normalizeMetrics(latencies)
	errorNorm := normalizeMetrics(errorRates)

	// Score each candidate (lower cost/latency/errors → higher score).
	// effectiveCostWeight amplifies cost sensitivity under budget pressure.
	// Also build per-candidate explain entries (SPEC_145).
	explainByModel := make(map[string]SmartCandidateExplain, len(candidates))
	var entries []scored
	for i, c := range candidates {
		costTerm := (1.0 - costNorm[i]) * effectiveCostWeight
		latTerm := (1.0 - latencyNorm[i]) * weights.Latency
		errTerm := (1.0 - errorNorm[i]) * weights.Errors
		finalScore := costTerm + latTerm + errTerm

		src := modelSources[c.Name]
		m := modelMetrics[c.Name]
		explainByModel[c.Name] = SmartCandidateExplain{
			Model: c.Name,
			Raw: SmartCandidateRaw{
				CostUSD:   m.cost,
				LatencyMs: m.latency,
				ErrorRate: m.errors,
			},
			Normalized: SmartCandidateNormalized{
				Cost:    costNorm[i],
				Latency: latencyNorm[i],
				Errors:  errorNorm[i],
			},
			ScoreComponents: SmartCandidateScoreComponents{
				Cost:    costTerm,
				Latency: latTerm,
				Errors:  errTerm,
			},
			FinalScore: finalScore,
			MetricSources: map[string]string{
				"cost":    src.cost,
				"latency": src.latency,
				"errors":  src.errors,
			},
			UsedDefaults: src.latency == "default_no_history" || src.errors == "default_no_history",
		}
		entries = append(entries, scored{name: c.Name, score: finalScore})
	}

	// Store cost optimizer metadata for observability.
	req.SmartFilterContext.EstimatedCostsUSD = estimatedCostsUSD
	req.SmartFilterContext.BudgetPressure = req.BudgetPressure
	req.SmartFilterContext.CostOptimizerApplied = costOptimizerApplied

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	// Preferred models (from stages) go first, non-preferred follow.
	// Within each group the score-based order established above is preserved.
	preferredSet := make(map[string]bool, len(stageResult.PreferredModels))
	for _, m := range stageResult.PreferredModels {
		preferredSet[m] = true
	}

	ordered := make([]string, 0, len(entries))
	if len(preferredSet) > 0 {
		for _, e := range entries {
			if preferredSet[e.name] {
				ordered = append(ordered, e.name)
			}
		}
		for _, e := range entries {
			if !preferredSet[e.name] {
				ordered = append(ordered, e.name)
			}
		}
	} else {
		for _, e := range entries {
			ordered = append(ordered, e.name)
		}
	}

	// Populate ranking explainability in plan order (SPEC_145).
	rankingDetails := make([]SmartCandidateExplain, 0, len(ordered))
	for _, name := range ordered {
		if ex, ok := explainByModel[name]; ok {
			rankingDetails = append(rankingDetails, ex)
		}
	}
	req.SmartFilterContext.RankingDetails = rankingDetails
	req.SmartFilterContext.EffectiveWeights = map[string]float64{
		"cost":    effectiveCostWeight,
		"latency": weights.Latency,
		"errors":  weights.Errors,
	}
	req.SmartFilterContext.EffectiveCostWeight = effectiveCostWeight

	return ordered
}

// normalizeMetrics safely normalizes values to [0, 1] range.
// Returns all zeros if max == min (no variation) or if only one value.
func normalizeMetrics(values []float64) []float64 {
	if len(values) == 0 {
		return []float64{}
	}

	if len(values) == 1 {
		return []float64{0} // Single value: no differentiation
	}

	min := values[0]
	max := values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	normalized := make([]float64, len(values))

	// No variation: all values identical
	if max == min {
		for i := range normalized {
			normalized[i] = 0
		}
		return normalized
	}

	// Normalize to [0, 1]
	for i, v := range values {
		normalized[i] = (v - min) / (max - min)
	}

	return normalized
}

// budgetPressureMultiplier returns the cost-weight multiplier for a given budget pressure.
// pressure < 0.50 → 1.0 (normal), 0.50-0.80 → 1.5 (medium), >= 0.80 → 2.0 (high).
func budgetPressureMultiplier(pressure float64) float64 {
	switch {
	case pressure >= 0.80:
		return 2.0
	case pressure >= 0.50:
		return 1.5
	default:
		return 1.0
	}
}

// estimateRequestCostUSD computes the estimated cost of a single request in USD.
// Uses promptTokens (estimated from input length) and completionTokens (from max_tokens or default).
// Returns 0 when pricing is not configured (caller falls back to rate sum for fail-open ordering).
func estimateRequestCostUSD(p config.Pricing, promptTokens, completionTokens int) float64 {
	return float64(promptTokens)/1_000_000*p.PromptPer1M +
		float64(completionTokens)/1_000_000*p.CompletionPer1M
}

// applyRules checks if any rule matches the request and returns preferred models.
func (rt *Router) applyRules(req Request) []string {
	totalChars := 0
	for _, msg := range req.Messages {
		totalChars += len(msg)
	}
	estimatedTokens := totalChars / 4

	combinedText := ""
	for _, msg := range req.Messages {
		combinedText += msg + " "
	}
	combinedText = toLower(combinedText)

	for _, rule := range req.SmartConfig.Rules {
		matched := true

		if rule.When.MaxPromptTokens != nil {
			if estimatedTokens > *rule.When.MaxPromptTokens {
				matched = false
			}
		}

		if len(rule.When.Contains) > 0 {
			foundAny := false
			for _, keyword := range rule.When.Contains {
				if contains(combinedText, toLower(keyword)) {
					foundAny = true
					break
				}
			}
			if !foundAny {
				matched = false
			}
		}

		if matched && len(rule.PreferModels) > 0 {
			return rule.PreferModels
		}
	}

	return nil
}

// toLower converts string to lowercase (simple ASCII implementation).
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		b[i] = c
	}
	return string(b)
}

// contains checks if substring is in string.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOfSubstr(s, substr) >= 0
}

func indexOfSubstr(s, substr string) int {
	n := len(s)
	m := len(substr)
	if m == 0 {
		return 0
	}
	if m > n {
		return -1
	}
	for i := 0; i <= n-m; i++ {
		j := 0
		for j < m && s[i+j] == substr[j] {
			j++
		}
		if j == m {
			return i
		}
	}
	return -1
}
