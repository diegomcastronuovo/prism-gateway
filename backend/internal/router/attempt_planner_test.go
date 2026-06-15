package router

import (
	"testing"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func TestAttemptPlanner(t *testing.T) {
	models := []config.ModelConfig{
		{Name: "gpt-4o-mini", Provider: "openai"},
		{Name: "claude-sonnet-4-6", Provider: "anthropic"},
		{Name: "gemini-2.5-flash", Provider: "google"},
	}

	t.Run("build plan respects max_attempts", func(t *testing.T) {
		cb := NewCircuitBreaker()
		fallbackConfig := config.FallbackConfig{
			Enabled:     true,
			MaxAttempts: 2,
			TimeoutMs:   5000,
		}

		planner := NewAttemptPlanner(cb, fallbackConfig)
		plan := planner.BuildPlan(models, nil)

		if len(plan.Attempts) != 2 {
			t.Errorf("expected 2 attempts, got %d", len(plan.Attempts))
		}
		if plan.MaxAttempts != 2 {
			t.Errorf("expected MaxAttempts=2, got %d", plan.MaxAttempts)
		}
		if plan.TotalTimeoutMs != 10000 {
			t.Errorf("expected TotalTimeoutMs=10000, got %d", plan.TotalTimeoutMs)
		}
	})

	t.Run("build plan filters circuit-broken providers", func(t *testing.T) {
		cb := NewCircuitBreaker()
		cb.RecordError("openai", ErrorTypeRateLimited) // Open circuit for openai

		fallbackConfig := config.FallbackConfig{
			Enabled:   true,
			TimeoutMs: 5000,
		}

		planner := NewAttemptPlanner(cb, fallbackConfig)
		plan := planner.BuildPlan(models, nil)

		if len(plan.Attempts) != 2 {
			t.Errorf("expected 2 attempts (openai filtered), got %d", len(plan.Attempts))
		}

		// Verify no openai models in plan
		for _, attempt := range plan.Attempts {
			if attempt.Provider == "openai" {
				t.Errorf("expected openai to be filtered out, found %s", attempt.Model)
			}
		}
	})

	t.Run("build plan excludes already-tried models", func(t *testing.T) {
		cb := NewCircuitBreaker()
		fallbackConfig := config.FallbackConfig{
			Enabled:   true,
			TimeoutMs: 5000,
		}

		alreadyTried := map[string]bool{
			"gpt-4o-mini": true,
		}

		planner := NewAttemptPlanner(cb, fallbackConfig)
		plan := planner.BuildPlan(models, alreadyTried)

		if len(plan.Attempts) != 2 {
			t.Errorf("expected 2 attempts (gpt-4o-mini excluded), got %d", len(plan.Attempts))
		}

		// Verify gpt-4o-mini not in plan
		for _, attempt := range plan.Attempts {
			if attempt.Model == "gpt-4o-mini" {
				t.Error("expected gpt-4o-mini to be excluded")
			}
		}
	})

	t.Run("fallback disabled limits to 1 attempt", func(t *testing.T) {
		cb := NewCircuitBreaker()
		fallbackConfig := config.FallbackConfig{
			Enabled:   false,
			TimeoutMs: 5000,
		}

		planner := NewAttemptPlanner(cb, fallbackConfig)
		plan := planner.BuildPlan(models, nil)

		if len(plan.Attempts) != 1 {
			t.Errorf("expected 1 attempt (fallback disabled), got %d", len(plan.Attempts))
		}
		if plan.MaxAttempts != 1 {
			t.Errorf("expected MaxAttempts=1, got %d", plan.MaxAttempts)
		}
	})

	t.Run("plan includes timeout per attempt", func(t *testing.T) {
		cb := NewCircuitBreaker()
		fallbackConfig := config.FallbackConfig{
			Enabled:     true,
			MaxAttempts: 3,
			TimeoutMs:   8000,
		}

		planner := NewAttemptPlanner(cb, fallbackConfig)
		plan := planner.BuildPlan(models, nil)

		for i, attempt := range plan.Attempts {
			if attempt.TimeoutMs != 8000 {
				t.Errorf("attempt %d: expected TimeoutMs=8000, got %d", i, attempt.TimeoutMs)
			}
			if attempt.Rank != i+1 {
				t.Errorf("attempt %d: expected Rank=%d, got %d", i, i+1, attempt.Rank)
			}
		}
	})

	t.Run("empty candidates returns empty plan", func(t *testing.T) {
		cb := NewCircuitBreaker()
		fallbackConfig := config.FallbackConfig{
			Enabled:   true,
			TimeoutMs: 5000,
		}

		planner := NewAttemptPlanner(cb, fallbackConfig)
		plan := planner.BuildPlan([]config.ModelConfig{}, nil)

		if len(plan.Attempts) != 0 {
			t.Errorf("expected 0 attempts for empty candidates, got %d", len(plan.Attempts))
		}
	})

	t.Run("all candidates circuit-broken returns empty plan", func(t *testing.T) {
		cb := NewCircuitBreaker()
		cb.RecordError("openai", ErrorTypeRateLimited)
		cb.RecordError("anthropic", ErrorTypeRateLimited)
		cb.RecordError("google", ErrorTypeRateLimited)

		fallbackConfig := config.FallbackConfig{
			Enabled:   true,
			TimeoutMs: 5000,
		}

		planner := NewAttemptPlanner(cb, fallbackConfig)
		plan := planner.BuildPlan(models, nil)

		if len(plan.Attempts) != 0 {
			t.Errorf("expected 0 attempts (all circuit-broken), got %d", len(plan.Attempts))
		}
	})
}
