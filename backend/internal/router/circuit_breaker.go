package router

import (
	"sync"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// CircuitBreakerState represents the state of a provider's circuit breaker
type CircuitBreakerState struct {
	OpenedAt time.Time
	Cooldown time.Duration
}

// CircuitBreaker tracks rate-limited providers and temporarily blocks them
type CircuitBreaker struct {
	mu           sync.Mutex
	openCircuits map[string]CircuitBreakerState // provider -> state
	cooldown     time.Duration
}

// NewCircuitBreaker creates a circuit breaker with 30s cooldown
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		openCircuits: make(map[string]CircuitBreakerState),
		cooldown:     30 * time.Second,
	}
}

// RecordError records an error for a provider and opens circuit if rate-limited
func (cb *CircuitBreaker) RecordError(provider string, errType ErrorType) {
	if errType != ErrorTypeRateLimited {
		return // Only open circuit for rate limits
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.openCircuits[provider] = CircuitBreakerState{
		OpenedAt: time.Now(),
		Cooldown: cb.cooldown,
	}
}

// IsOpen checks if a provider's circuit is currently open
func (cb *CircuitBreaker) IsOpen(provider string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state, exists := cb.openCircuits[provider]
	if !exists {
		return false
	}

	// Check if cooldown period has elapsed
	if time.Since(state.OpenedAt) > state.Cooldown {
		// Auto-close circuit after cooldown
		delete(cb.openCircuits, provider)
		return false
	}

	return true
}

// FilterCandidates removes models from providers with open circuits
func (cb *CircuitBreaker) FilterCandidates(candidates []config.ModelConfig) []config.ModelConfig {
	filtered := make([]config.ModelConfig, 0, len(candidates))

	for _, c := range candidates {
		if !cb.IsOpen(c.Provider) {
			filtered = append(filtered, c)
		}
	}

	return filtered
}

// Reset clears all circuit breaker state (for testing)
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.openCircuits = make(map[string]CircuitBreakerState)
}

// GetOpenProviders returns a list of providers with open circuits (for observability)
func (cb *CircuitBreaker) GetOpenProviders() []string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	providers := make([]string, 0, len(cb.openCircuits))
	for provider, state := range cb.openCircuits {
		// Only include if still within cooldown
		if time.Since(state.OpenedAt) <= state.Cooldown {
			providers = append(providers, provider)
		}
	}

	return providers
}
