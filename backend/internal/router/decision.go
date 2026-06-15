package router

import (
	"encoding/json"
	"time"
)

// PIISnapshot captures PII-aware routing decision details
type PIISnapshot struct {
	Decision    string `json:"decision"`
	TargetModel string `json:"target_model,omitempty"`
}

// WorkflowSnapshot captures per-request workflow policy evaluation for observability.
// Stored inside DecisionSnapshot so every request_log row has its own policy state.
type WorkflowSnapshot struct {
	WorkflowID       string `json:"workflow_id"`
	ConversationID   string `json:"conversation_id"`
	TierName         string `json:"tier_name"`         // tier after this request ("premium"|"normal"|"low")
	AccumulatedSteps int    `json:"accumulated_steps"` // steps INCLUDING this request
	StepsAction      string `json:"steps_action"`      // steps-policy outcome: "none"|"degrade"|"block"
	CostAction       string `json:"cost_action"`       // cost-policy outcome:  "none"|"degrade"|"block"
	Action           string `json:"action"`            // combined (most severe): "none"|"degrade"|"block"
	Blocked          bool   `json:"blocked"`           // true after a block action
}

// DecisionSnapshot captures the full context of a routing decision for observability
type DecisionSnapshot struct {
	Precedence PrecedenceDecision `json:"precedence"`
	Routing    RoutingDecision    `json:"routing"`
	Smart      *SmartDecision     `json:"smart,omitempty"`
	Fallback   *FallbackDecision  `json:"fallback,omitempty"`
	Plan       []string           `json:"plan"`
	PII        *PIISnapshot       `json:"pii,omitempty"`
	Workflow   *WorkflowSnapshot  `json:"workflow,omitempty"` // populated for decision_ops requests
}

// RoutingDecision captures tenant routing configuration used for this request
type RoutingDecision struct {
	Strategy string `json:"strategy"`
}

// PrecedenceDecision captures P0-P5 resolution details
type PrecedenceDecision struct {
	RequestedSource string `json:"requested_source"` // "none" | "body" | "header"
	RequestedModel  string `json:"requested_model"`
	RouteGroup      string `json:"route_group"`
	PoolSize        int    `json:"pool_size"` // Number of candidates after filtering
}

// SmartCandidateRaw holds the raw (pre-normalization) metric values used to score a candidate.
type SmartCandidateRaw struct {
	CostUSD   float64 `json:"cost_usd"`
	LatencyMs float64 `json:"latency_ms"`
	ErrorRate float64 `json:"error_rate"`
}

// SmartCandidateNormalized holds the normalized [0,1] values fed into the scoring formula.
type SmartCandidateNormalized struct {
	Cost    float64 `json:"cost"`
	Latency float64 `json:"latency"`
	Errors  float64 `json:"errors"`
}

// SmartCandidateScoreComponents holds the per-dimension contribution to the final score.
// Each value equals (1 - normalized) * effective_weight for that dimension.
type SmartCandidateScoreComponents struct {
	Cost    float64 `json:"cost"`
	Latency float64 `json:"latency"`
	Errors  float64 `json:"errors"`
}

// SmartCandidateExplain captures the full scoring breakdown for one candidate model.
type SmartCandidateExplain struct {
	Model           string                         `json:"model"`
	Raw             SmartCandidateRaw              `json:"raw"`
	Normalized      SmartCandidateNormalized       `json:"normalized"`
	ScoreComponents SmartCandidateScoreComponents  `json:"score_components"`
	FinalScore      float64                        `json:"final_score"`
	// MetricSources indicates the origin of each metric: "estimated", "db_stats",
	// "ewma_store", "rate_sum_fallback", or "default_no_history".
	MetricSources map[string]string `json:"metric_sources"`
	// UsedDefaults is true when any metric fell back to a static default value
	// (e.g. 2000 ms latency for a model with no history).
	UsedDefaults bool `json:"used_defaults"`
}

// SmartEvaluationSnapshot consolidates stage evaluation observability (SPEC_149).
// Included in SmartDecision only when at least one rule matched.
type SmartEvaluationSnapshot struct {
	StagesEvaluated []string         `json:"stages_evaluated"`
	RulesMatched    []SmartRuleMatch `json:"rules_matched,omitempty"`
}

// SmartDecision captures smart routing stage evaluation
type SmartDecision struct {
	StagesEvaluated    []string `json:"stages_evaluated"`
	PreferredModels    []string `json:"preferred_models"`
	BannedModels       []string `json:"banned_models"`
	Blocked            bool     `json:"blocked"`
	BlockReason        string   `json:"block_reason,omitempty"`
	PromptLength       int      `json:"prompt_length,omitempty"`
	SemanticAnchor     string   `json:"semantic_anchor,omitempty"`
	SemanticSimilarity float64  `json:"semantic_similarity,omitempty"`
	SemanticDistance   float64  `json:"semantic_distance,omitempty"`
	AnchorRouteGroup   string   `json:"anchor_route_group,omitempty"`
	// Cost optimizer metadata
	EstimatedCostsUSD    map[string]float64 `json:"estimated_costs_usd,omitempty"`
	BudgetPressure       float64            `json:"budget_pressure,omitempty"`
	CostOptimizerApplied bool               `json:"cost_optimizer_applied,omitempty"`
	// Ranking explainability (SPEC_145)
	// EffectiveWeights are the actual weights applied at scoring time (cost already
	// multiplied by budget-pressure multiplier).
	EffectiveWeights    map[string]float64      `json:"effective_weights,omitempty"`
	EffectiveCostWeight float64                 `json:"effective_cost_weight,omitempty"`
	RankingDetails      []SmartCandidateExplain `json:"ranking_details,omitempty"`
	// Rule match observability (SPEC_149): populated when at least one rule matched
	SmartEvaluation *SmartEvaluationSnapshot `json:"smart_evaluation,omitempty"`
}

// FallbackDecision captures fallback execution details
type FallbackDecision struct {
	Enabled        bool     `json:"enabled"`
	MaxAttempts    int      `json:"max_attempts"`
	ActualAttempts int      `json:"actual_attempts"`
	ErrorTypes     []string `json:"error_types"` // Per-attempt error classification
}

// TrafficSplitSnapshotEntry is one candidate recorded in a routing snapshot.
type TrafficSplitSnapshotEntry struct {
	Model  string `json:"model"`
	Weight int    `json:"weight"`
}

// RoutingSnapshot is the flat, human-readable routing decision stored per successful
// request for forensic debugging and deterministic replay.
// Never contains prompts, responses, API keys, or PII.
type RoutingSnapshot struct {
	RoutingStrategy  string    `json:"routing_strategy"`
	SemanticAnchor   string    `json:"semantic_anchor,omitempty"`
	Similarity       float64   `json:"similarity,omitempty"`
	Threshold        float64   `json:"threshold,omitempty"`
	RouteGroup       string    `json:"route_group,omitempty"`
	CandidateModels  []string  `json:"candidate_models"`
	SelectedModel    string    `json:"selected_model"`
	Provider         string    `json:"provider"`
	FallbackAttempts int       `json:"fallback_attempts"`
	Timestamp        time.Time `json:"timestamp"`
	// Cost optimizer metadata (populated when smart routing with pricing)
	EstimatedCostsUSD    map[string]float64 `json:"estimated_costs_usd,omitempty"`
	BudgetPressure       float64            `json:"budget_pressure,omitempty"`
	CostOptimizerApplied bool               `json:"cost_optimizer_applied,omitempty"`
	// Traffic split / canary metadata
	TrafficSplitApplied    bool                        `json:"traffic_split_applied,omitempty"`
	TrafficSplitKey        string                      `json:"traffic_split_key,omitempty"`
	TrafficSplitCandidates []TrafficSplitSnapshotEntry `json:"traffic_split_candidates,omitempty"`
}

func (rs *RoutingSnapshot) ToJSON() (json.RawMessage, error) {
	data, err := json.Marshal(rs)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// ToJSON serializes decision snapshot to JSON
func (ds *DecisionSnapshot) ToJSON() (json.RawMessage, error) {
	data, err := json.Marshal(ds)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
