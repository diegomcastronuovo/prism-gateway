package router

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// SmartRuleMatch records a single rule that matched during stage evaluation.
// Only populated for rules that actually matched (not evaluated-but-skipped rules).
type SmartRuleMatch struct {
	Stage     string                 `json:"stage"`
	Condition string                 `json:"condition"`
	Value     interface{}            `json:"value"`
	Action    map[string]interface{} `json:"action"`
	Reason    string                 `json:"reason,omitempty"`
}

// SmartStageResult holds the outcome of stage evaluation
type SmartStageResult struct {
	Blocked          bool
	BlockReason      string
	PreferredModels  []string
	BannedModels     []string
	Constraints      SmartConstraintsResult
	StagesEvaluated  []string
	LegacyExclusive  bool // If true, filter to ONLY preferred models (backwards compat)
	PromptLength             int  // Total character count of all messages
	PromptLengthBlock        bool // True when block was triggered by a prompt_length condition
	PromptLengthCustomReason bool // True when rule.Action.Reason was explicitly set
	// Semantic routing results (populated when a semantic_similarity rule matches)
	SemanticAnchor     string
	SemanticSimilarity float64
	SemanticDistance   float64
	AnchorRouteGroup   string // route_group set by use_anchor; applied by smartBased
	// Rule match observability (SPEC_149)
	RulesMatched []SmartRuleMatch

	// Cost optimizer results (populated by smartBased when pricing is available)
	EstimatedCostsUSD    map[string]float64
	BudgetPressure       float64
	CostOptimizerApplied bool

	// Ranking explainability (SPEC_145): populated by smartBased after scoring
	RankingDetails      []SmartCandidateExplain
	EffectiveWeights    map[string]float64
	EffectiveCostWeight float64
}

// SmartConstraintsResult holds accumulated constraints from stages
type SmartConstraintsResult struct {
	MaxCostPer1M *float64
	MaxLatencyMs *float64
}

// defaultSemanticThreshold is used when neither the rule nor the tenant config defines a threshold.
const defaultSemanticThreshold = 0.60

// StageEvaluator evaluates smart routing stages in order
type StageEvaluator struct {
	config   config.SmartConfig
	messages []string

	// Semantic routing dependencies (optional; nil means semantic rules are skipped)
	ctx            context.Context
	tenantID       string
	embeddingFn    func(ctx context.Context, text string) ([]float64, error)
	semanticLookup SemanticLookupFunc

	// Tenant-level semantic threshold fallback (0 = not set; falls back to defaultSemanticThreshold)
	semanticThresholdDefault float64

	// Lazy-computed prompt embedding (computed at most once per request)
	cachedEmbedding   []float64
	embeddingComputed bool
}

// NewStageEvaluator creates an evaluator for smart routing
func NewStageEvaluator(smartConfig config.SmartConfig, messages []string) *StageEvaluator {
	return &StageEvaluator{
		config:   smartConfig,
		messages: messages,
	}
}

// SetSemanticDeps wires the dependencies needed for semantic_similarity rule evaluation.
// Must be called before Evaluate() when any stage contains a semantic_similarity condition.
func (se *StageEvaluator) SetSemanticDeps(
	ctx context.Context,
	tenantID string,
	embFn func(ctx context.Context, text string) ([]float64, error),
	lookup SemanticLookupFunc,
) {
	se.ctx = ctx
	se.tenantID = tenantID
	se.embeddingFn = embFn
	se.semanticLookup = lookup
}

// SetSemanticThresholdDefault sets the tenant-level fallback threshold used when a
// semantic_similarity rule does not define its own threshold (i.e., threshold == 0).
// A value of 0 means "not set" and the global default (0.60) will be used instead.
func (se *StageEvaluator) SetSemanticThresholdDefault(v float64) {
	se.semanticThresholdDefault = v
}

// Evaluate applies stage rules in order (or falls back to legacy rules)
func (se *StageEvaluator) Evaluate() SmartStageResult {
	result := SmartStageResult{
		PreferredModels: []string{},
		BannedModels:    []string{},
		Constraints:     SmartConstraintsResult{},
	}

	// Backwards compatibility: if no stages defined, use legacy rules
	if len(se.config.Stages) == 0 && len(se.config.Rules) > 0 {
		return se.evaluateLegacyRules()
	}

	// Prepare input for rule matching
	totalChars := 0
	for _, msg := range se.messages {
		totalChars += len(msg)
	}
	estimatedTokens := totalChars / 4

	combinedText := ""
	for _, msg := range se.messages {
		combinedText += msg + " "
	}
	combinedText = toLower(combinedText)

	result.PromptLength = totalChars

	// Evaluate stages in order
	for _, stage := range se.config.Stages {
		for _, rule := range stage.Rules {
			// Semantic similarity rules are handled separately (lazy embedding + DB lookup).
			if rule.When.SemanticSimilarity != nil {
				// Skip if a previous rule already determined routing preferences.
				// This avoids unnecessary embedding API calls (spec: "cheap rules run first").
				if result.Blocked || len(result.PreferredModels) > 0 || result.AnchorRouteGroup != "" {
					continue
				}
				// Threshold precedence:
				// 1. Rule-level threshold (most specific)
				// 2. Tenant-level SemanticRoutingConfig.ThresholdDefault
				// 3. Global default (0.60)
				threshold := rule.When.SemanticSimilarity.Threshold
				if threshold <= 0 {
					if se.semanticThresholdDefault > 0 {
						threshold = se.semanticThresholdDefault
					} else {
						threshold = defaultSemanticThreshold
					}
				}
				matched, anchor := se.evaluateSemantic(threshold)
				if !matched {
					continue
				}
				result.StagesEvaluated = append(result.StagesEvaluated, stage.Name)
				result.SemanticAnchor = anchor.Name
				result.SemanticSimilarity = 1.0 - anchor.Distance
				result.SemanticDistance = anchor.Distance
				if rule.Action.UseAnchor {
					if anchor.RouteGroup != "" {
						result.AnchorRouteGroup = anchor.RouteGroup
					}
					if len(anchor.PreferredModels) > 0 {
						result.PreferredModels = append(result.PreferredModels, anchor.PreferredModels...)
					}
				}
				// Record match for observability (SPEC_149)
				result.RulesMatched = append(result.RulesMatched, buildSemanticRuleMatch(
					stage.Name, rule, result.SemanticSimilarity,
				))
				continue // semantic rules never block; pipeline continues
			}

			// Standard condition evaluation (contains, prompt_length, max_prompt_tokens)
			matched := se.evaluateCondition(rule.When, estimatedTokens, combinedText, totalChars)
			if !matched {
				continue
			}

			result.StagesEvaluated = append(result.StagesEvaluated, stage.Name)

			// Record match for observability (SPEC_149)
			result.RulesMatched = append(result.RulesMatched, buildStandardRuleMatch(
				stage.Name, rule, totalChars,
			))

			// Apply action
			if rule.Action.Block {
				result.Blocked = true
				result.BlockReason = rule.Action.Reason
				if result.BlockReason == "" && rule.When.PromptLength != nil {
					result.BlockReason = promptLengthBlockReason(rule.When.PromptLength)
				}
				result.PromptLengthBlock = rule.When.PromptLength != nil
				result.PromptLengthCustomReason = rule.When.PromptLength != nil && rule.Action.Reason != ""
				return result
			}

			if len(rule.Action.PreferModels) > 0 {
				result.PreferredModels = append(result.PreferredModels, rule.Action.PreferModels...)
			}

			if len(rule.Action.BanModels) > 0 {
				result.BannedModels = append(result.BannedModels, rule.Action.BanModels...)
			}

			if rule.Action.SetConstraints != nil {
				// Merge constraints (take most restrictive)
				if rule.Action.SetConstraints.MaxCostPer1M != nil {
					if result.Constraints.MaxCostPer1M == nil || *rule.Action.SetConstraints.MaxCostPer1M < *result.Constraints.MaxCostPer1M {
						result.Constraints.MaxCostPer1M = rule.Action.SetConstraints.MaxCostPer1M
					}
				}
				if rule.Action.SetConstraints.MaxLatencyMs != nil {
					if result.Constraints.MaxLatencyMs == nil || *rule.Action.SetConstraints.MaxLatencyMs < *result.Constraints.MaxLatencyMs {
						result.Constraints.MaxLatencyMs = rule.Action.SetConstraints.MaxLatencyMs
					}
				}
			}
		}
	}

	return result
}

// evaluateSemantic computes the prompt embedding (lazily, once per request) and queries
// the nearest anchor. Returns (matched, anchor). On any error it returns (false, {}) (fail open).
func (se *StageEvaluator) evaluateSemantic(threshold float64) (bool, SemanticAnchorResult) {

	if se.embeddingFn == nil || se.semanticLookup == nil {
		return false, SemanticAnchorResult{} // no deps configured → fail open
	}

	// Compute the prompt embedding at most once per request (lazy).
	if !se.embeddingComputed {
		se.embeddingComputed = true
		combined := strings.Join(se.messages, " ")
		emb, err := se.embeddingFn(se.ctx, combined)
		if err == nil {
			se.cachedEmbedding = emb
		}
		// On error: cachedEmbedding stays nil → fail open below.
	}

	if se.cachedEmbedding == nil {
		return false, SemanticAnchorResult{} // embedding failed → fail open
	}

	anchor, found, err := se.semanticLookup(se.ctx, se.tenantID, se.cachedEmbedding)
	if err != nil || !found {
		return false, SemanticAnchorResult{} // no anchors or DB error → fail open
	}

	similarity := 1.0 - anchor.Distance

	if math.IsNaN(similarity) || math.IsInf(similarity, 0) {
		return false, SemanticAnchorResult{} // invalid similarity → fail open
	}

	return similarity >= threshold, anchor
}

// evaluateLegacyRules handles backwards compatibility with flat rules
func (se *StageEvaluator) evaluateLegacyRules() SmartStageResult {
	result := SmartStageResult{
		PreferredModels: []string{},
		BannedModels:    []string{},
		LegacyExclusive: false, // Will be set to true if rule matches
	}

	totalChars := 0
	for _, msg := range se.messages {
		totalChars += len(msg)
	}
	estimatedTokens := totalChars / 4

	combinedText := ""
	for _, msg := range se.messages {
		combinedText += msg + " "
	}
	combinedText = toLower(combinedText)

	for _, rule := range se.config.Rules {
		matched := se.evaluateCondition(rule.When, estimatedTokens, combinedText, totalChars)
		if matched && len(rule.PreferModels) > 0 {
			result.PreferredModels = rule.PreferModels
			result.StagesEvaluated = append(result.StagesEvaluated, "legacy:"+rule.Name)
			result.LegacyExclusive = true // Legacy: filter exclusively to preferred models
			return result                 // First match wins (legacy behavior)
		}
	}

	return result
}

// evaluateCondition checks if a rule condition matches the input
func (se *StageEvaluator) evaluateCondition(cond config.SmartRuleCondition, estimatedTokens int, combinedText string, promptLen int) bool {
	matched := true

	if cond.MaxPromptTokens != nil {
		if estimatedTokens > *cond.MaxPromptTokens {
			matched = false
		}
	}

	if len(cond.Contains) > 0 {
		foundAny := false
		for _, keyword := range cond.Contains {
			if contains(combinedText, toLower(keyword)) {
				foundAny = true
				break
			}
		}
		if !foundAny {
			matched = false
		}
	}

	if cond.PromptLength != nil {
		pl := cond.PromptLength
		if pl.GT != nil && !(promptLen > *pl.GT) {
			matched = false
		}
		if pl.GTE != nil && !(promptLen >= *pl.GTE) {
			matched = false
		}
		if pl.LT != nil && !(promptLen < *pl.LT) {
			matched = false
		}
		if pl.LTE != nil && !(promptLen <= *pl.LTE) {
			matched = false
		}
	}

	return matched
}

// promptLengthBlockReason generates an auto block reason from a PromptLengthCondition.
func promptLengthBlockReason(pl *config.PromptLengthCondition) string {
	if pl.GT != nil {
		return fmt.Sprintf("prompt_length_gt_%d", *pl.GT)
	}
	if pl.GTE != nil {
		return fmt.Sprintf("prompt_length_gte_%d", *pl.GTE)
	}
	if pl.LT != nil {
		return fmt.Sprintf("prompt_length_lt_%d", *pl.LT)
	}
	if pl.LTE != nil {
		return fmt.Sprintf("prompt_length_lte_%d", *pl.LTE)
	}
	return "prompt_length_exceeded"
}

// ApplyStageResult filters candidates based on stage evaluation result
func ApplyStageResult(candidates []config.ModelConfig, result SmartStageResult) []config.ModelConfig {
	if result.Blocked {
		return []config.ModelConfig{}
	}

	// Build banned set
	bannedSet := make(map[string]bool)
	for _, m := range result.BannedModels {
		bannedSet[m] = true
	}

	// Remove banned models
	filtered := make([]config.ModelConfig, 0)
	for _, c := range candidates {
		if !bannedSet[c.Name] {
			filtered = append(filtered, c)
		}
	}

	// Apply constraints
	if result.Constraints.MaxCostPer1M != nil || result.Constraints.MaxLatencyMs != nil {
		constrainedCandidates := make([]config.ModelConfig, 0)
		for _, c := range filtered {
			totalCost := c.Pricing.PromptPer1M + c.Pricing.CompletionPer1M

			// Check cost constraint
			if result.Constraints.MaxCostPer1M != nil && totalCost > *result.Constraints.MaxCostPer1M {
				continue
			}

			// Note: MaxLatencyMs constraint requires runtime data, applied in scoring phase
			constrainedCandidates = append(constrainedCandidates, c)
		}
		filtered = constrainedCandidates
	}

	// If preferred models specified, reorder or filter
	if len(result.PreferredModels) > 0 {
		preferredSet := make(map[string]bool)
		for _, m := range result.PreferredModels {
			preferredSet[m] = true
		}

		preferred := make([]config.ModelConfig, 0)
		rest := make([]config.ModelConfig, 0)

		for _, c := range filtered {
			if preferredSet[c.Name] {
				preferred = append(preferred, c)
			} else {
				rest = append(rest, c)
			}
		}

		// Legacy mode: filter exclusively to preferred models
		if result.LegacyExclusive {
			filtered = preferred
		} else {
			// New stages mode: preferred models first, then rest
			filtered = append(preferred, rest...)
		}
	}

	return filtered
}

// buildStandardRuleMatch constructs a SmartRuleMatch for contains/prompt_length/max_prompt_tokens rules.
func buildStandardRuleMatch(stageName string, rule config.SmartStageRule, promptLen int) SmartRuleMatch {
	m := SmartRuleMatch{Stage: stageName, Reason: rule.Action.Reason}

	if len(rule.When.Contains) > 0 {
		m.Condition = "contains"
		m.Value = rule.When.Contains
	} else if rule.When.PromptLength != nil {
		m.Condition = "prompt_length"
		m.Value = promptLen
	} else if rule.When.MaxPromptTokens != nil {
		m.Condition = "max_prompt_tokens"
		m.Value = promptLen / 4 // same estimate as evaluateCondition
	}

	action := map[string]interface{}{}
	if rule.Action.Block {
		action["block"] = true
	}
	if rule.Action.Reason != "" {
		action["reason"] = rule.Action.Reason
	}
	if len(rule.Action.PreferModels) > 0 {
		action["prefer_models"] = rule.Action.PreferModels
	}
	if len(rule.Action.BanModels) > 0 {
		action["ban_models"] = rule.Action.BanModels
	}
	if rule.Action.SetConstraints != nil {
		if rule.Action.SetConstraints.MaxCostPer1M != nil {
			action["max_cost_per_1m"] = *rule.Action.SetConstraints.MaxCostPer1M
		}
		if rule.Action.SetConstraints.MaxLatencyMs != nil {
			action["max_latency_ms"] = *rule.Action.SetConstraints.MaxLatencyMs
		}
	}
	m.Action = action
	return m
}

// buildSemanticRuleMatch constructs a SmartRuleMatch for semantic_similarity rules.
func buildSemanticRuleMatch(stageName string, rule config.SmartStageRule, similarity float64) SmartRuleMatch {
	action := map[string]interface{}{
		"use_anchor": rule.Action.UseAnchor,
	}
	if len(rule.Action.PreferModels) > 0 {
		action["prefer_models"] = rule.Action.PreferModels
	}
	return SmartRuleMatch{
		Stage:     stageName,
		Condition: "semantic_similarity",
		Value:     similarity,
		Action:    action,
	}
}
