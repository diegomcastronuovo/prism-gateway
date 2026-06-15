package router

import (
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// PlannedAttempt represents a single attempt in the execution plan
type PlannedAttempt struct {
	Model      string
	Provider   string
	TimeoutMs  int
	Rank       int    // 1-based rank in the plan
	Reason     string // Why this model at this position
}

// AttemptPlan represents the deterministic execution plan
type AttemptPlan struct {
	Attempts       []PlannedAttempt
	MaxAttempts    int
	TotalTimeoutMs int
}

// AttemptPlanner builds deterministic execution plans
type AttemptPlanner struct {
	circuitBreaker *CircuitBreaker
	fallbackConfig config.FallbackConfig
}

// NewAttemptPlanner creates an attempt planner
func NewAttemptPlanner(circuitBreaker *CircuitBreaker, fallbackConfig config.FallbackConfig) *AttemptPlanner {
	return &AttemptPlanner{
		circuitBreaker: circuitBreaker,
		fallbackConfig: fallbackConfig,
	}
}

// BuildPlan creates an ordered list of attempts from candidates
func (ap *AttemptPlanner) BuildPlan(candidates []config.ModelConfig, alreadyTried map[string]bool) AttemptPlan {
	plan := AttemptPlan{
		Attempts: []PlannedAttempt{},
	}

	// Filter circuit-broken providers
	filtered := ap.circuitBreaker.FilterCandidates(candidates)

	// Filter already-tried models (for retry scenarios)
	if len(alreadyTried) > 0 {
		untried := make([]config.ModelConfig, 0)
		for _, c := range filtered {
			if !alreadyTried[c.Name] {
				untried = append(untried, c)
			}
		}
		filtered = untried
	}

	// Determine max attempts
	maxAttempts := len(filtered)
	if ap.fallbackConfig.Enabled && ap.fallbackConfig.MaxAttempts > 0 {
		if ap.fallbackConfig.MaxAttempts < maxAttempts {
			maxAttempts = ap.fallbackConfig.MaxAttempts
		}
	} else if !ap.fallbackConfig.Enabled {
		maxAttempts = 1
	}

	plan.MaxAttempts = maxAttempts

	// Build attempts
	timeoutMs := ap.fallbackConfig.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = 30000 // Default 30s
	}

	for i := 0; i < maxAttempts && i < len(filtered); i++ {
		model := filtered[i]

		reason := ""
		if i == 0 {
			reason = "rank:1|primary"
		} else {
			reason = "rank:" + string(rune('0'+i+1)) + "|fallback"
		}

		plan.Attempts = append(plan.Attempts, PlannedAttempt{
			Model:     model.Name,
			Provider:  model.Provider,
			TimeoutMs: timeoutMs,
			Rank:      i + 1,
			Reason:    reason,
		})

		plan.TotalTimeoutMs += timeoutMs
	}

	return plan
}
