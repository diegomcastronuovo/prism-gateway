package circuitbreaker

import "context"

// Outcome represents the result of an upstream call.
type Outcome int

const (
	OutcomeSuccess Outcome = iota
	OutcomeFailure
)

// Breaker is the circuit breaker interface.
type Breaker interface {
	// Allow returns (allowed, isProbe, err).
	// isProbe=true when in HALF_OPEN — caller must always call Report.
	Allow(ctx context.Context, provider string) (allowed bool, isProbe bool, err error)
	// Report records the outcome of a completed upstream call.
	Report(ctx context.Context, provider string, outcome Outcome, isProbe bool) error
}

// NoopBreaker always allows — used when backend=in_memory.
type NoopBreaker struct{}

func (NoopBreaker) Allow(_ context.Context, _ string) (bool, bool, error) {
	return true, false, nil
}

func (NoopBreaker) Report(_ context.Context, _ string, _ Outcome, _ bool) error {
	return nil
}
