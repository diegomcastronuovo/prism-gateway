package router

import (
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func TestCircuitBreaker(t *testing.T) {
	t.Run("record rate limit error opens circuit", func(t *testing.T) {
		cb := NewCircuitBreaker()

		cb.RecordError("openai", ErrorTypeRateLimited)

		if !cb.IsOpen("openai") {
			t.Error("expected circuit to be open after rate limit error")
		}
	})

	t.Run("non-rate-limit errors do not open circuit", func(t *testing.T) {
		cb := NewCircuitBreaker()

		cb.RecordError("openai", ErrorTypeTimeout)
		cb.RecordError("openai", ErrorTypeUpstream5xx)
		cb.RecordError("openai", ErrorTypeNetwork)

		if cb.IsOpen("openai") {
			t.Error("expected circuit to remain closed for non-rate-limit errors")
		}
	})

	t.Run("circuit auto-closes after cooldown", func(t *testing.T) {
		cb := NewCircuitBreaker()
		cb.cooldown = 100 * time.Millisecond // Short cooldown for testing

		cb.RecordError("openai", ErrorTypeRateLimited)

		if !cb.IsOpen("openai") {
			t.Error("expected circuit to be open immediately after error")
		}

		// Wait for cooldown to elapse
		time.Sleep(150 * time.Millisecond)

		if cb.IsOpen("openai") {
			t.Error("expected circuit to auto-close after cooldown")
		}
	})

	t.Run("filter candidates removes open circuit providers", func(t *testing.T) {
		cb := NewCircuitBreaker()

		candidates := []config.ModelConfig{
			{Name: "gpt-4o-mini", Provider: "openai"},
			{Name: "claude-sonnet-4-6", Provider: "anthropic"},
			{Name: "gpt-4o", Provider: "openai"},
			{Name: "gemini-2.5-flash", Provider: "google"},
		}

		// Open circuit for openai
		cb.RecordError("openai", ErrorTypeRateLimited)

		filtered := cb.FilterCandidates(candidates)

		if len(filtered) != 2 {
			t.Errorf("expected 2 candidates after filtering, got %d", len(filtered))
		}

		// Verify openai models are excluded
		for _, c := range filtered {
			if c.Provider == "openai" {
				t.Errorf("expected openai models to be filtered out, found %s", c.Name)
			}
		}
	})

	t.Run("get open providers returns list", func(t *testing.T) {
		cb := NewCircuitBreaker()

		cb.RecordError("openai", ErrorTypeRateLimited)
		cb.RecordError("anthropic", ErrorTypeRateLimited)

		openProviders := cb.GetOpenProviders()

		if len(openProviders) != 2 {
			t.Errorf("expected 2 open providers, got %d", len(openProviders))
		}

		providerSet := make(map[string]bool)
		for _, p := range openProviders {
			providerSet[p] = true
		}

		if !providerSet["openai"] || !providerSet["anthropic"] {
			t.Errorf("expected openai and anthropic in open providers, got %v", openProviders)
		}
	})

	t.Run("reset clears all circuits", func(t *testing.T) {
		cb := NewCircuitBreaker()

		cb.RecordError("openai", ErrorTypeRateLimited)
		cb.RecordError("anthropic", ErrorTypeRateLimited)

		if len(cb.GetOpenProviders()) != 2 {
			t.Error("expected 2 open circuits before reset")
		}

		cb.Reset()

		if len(cb.GetOpenProviders()) != 0 {
			t.Error("expected 0 open circuits after reset")
		}

		if cb.IsOpen("openai") || cb.IsOpen("anthropic") {
			t.Error("expected all circuits to be closed after reset")
		}
	})

	t.Run("multiple errors for same provider extend circuit", func(t *testing.T) {
		cb := NewCircuitBreaker()
		cb.cooldown = 100 * time.Millisecond

		// First error opens circuit
		cb.RecordError("openai", ErrorTypeRateLimited)
		firstOpenTime := time.Now()

		// Wait a bit
		time.Sleep(50 * time.Millisecond)

		// Second error resets the timer
		cb.RecordError("openai", ErrorTypeRateLimited)

		// Circuit should still be open even after original cooldown would have elapsed
		time.Sleep(75 * time.Millisecond) // Total: 125ms from first error

		if !cb.IsOpen("openai") {
			t.Error("expected circuit to still be open due to second error")
		}

		// Wait for second error's cooldown
		time.Sleep(50 * time.Millisecond)

		if cb.IsOpen("openai") {
			t.Error("expected circuit to close after second error's cooldown")
		}

		_ = firstOpenTime // Suppress unused warning
	})
}
