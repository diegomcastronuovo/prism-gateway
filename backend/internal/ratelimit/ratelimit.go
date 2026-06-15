package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter is the interface for rate limiting implementations.
// Supports both in-memory (per-process) and Redis-based (distributed) backends.
type Limiter interface {
	// Allow checks if a request from the given tenant should be allowed.
	// ctx is used for timeouts, cancellation, and extracting scoped keys (e.g., JWT sub).
	// Returns true if allowed, false if rate limit exceeded.
	// Also returns remaining tokens (approximate) and reset time.
	Allow(ctx context.Context, tenantID string, rpm, burst int) (allowed bool, remaining int, resetAt time.Time)
}

// NopLimiter allows all requests (used when rate limiting is disabled).
type NopLimiter struct{}

func (NopLimiter) Allow(ctx context.Context, tenantID string, rpm, burst int) (bool, int, time.Time) {
	return true, burst, time.Now().Add(60 * time.Second)
}

// InMemoryLimiter implements token bucket rate limiting in memory (per process).
// NOTE: This is NOT distributed - each gateway process has its own buckets.
// For multi-instance deployments, replace with RedisLimiter.
type InMemoryLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*tokenBucket
	clock   Clock
}

// Clock abstraction for testing with deterministic time.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// NewInMemoryLimiter creates a new in-memory rate limiter.
func NewInMemoryLimiter() *InMemoryLimiter {
	return &InMemoryLimiter{
		buckets: make(map[string]*tokenBucket),
		clock:   realClock{},
	}
}

// NewInMemoryLimiterWithClock creates a limiter with an injectable clock (for testing).
func NewInMemoryLimiterWithClock(clock Clock) *InMemoryLimiter {
	return &InMemoryLimiter{
		buckets: make(map[string]*tokenBucket),
		clock:   clock,
	}
}

func (l *InMemoryLimiter) Allow(ctx context.Context, tenantID string, rpm, burst int) (bool, int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[tenantID]
	if !ok {
		bucket = newTokenBucket(rpm, burst, l.clock)
		l.buckets[tenantID] = bucket
	}

	// Update bucket config if changed (supports dynamic config reload)
	bucket.rpm = rpm
	bucket.capacity = float64(burst)

	allowed := bucket.take(l.clock.Now())
	remaining := int(bucket.tokens)
	if remaining < 0 {
		remaining = 0
	}

	// Reset time is 60 seconds from now (simplified, could be more accurate)
	resetAt := l.clock.Now().Add(60 * time.Second)

	return allowed, remaining, resetAt
}

// tokenBucket implements the token bucket algorithm.
type tokenBucket struct {
	mu         sync.Mutex
	rpm        int       // requests per minute
	capacity   float64   // max tokens (burst)
	tokens     float64   // current tokens
	lastRefill time.Time // last refill timestamp
}

func newTokenBucket(rpm, burst int, clock Clock) *tokenBucket {
	return &tokenBucket{
		rpm:        rpm,
		capacity:   float64(burst),
		tokens:     float64(burst), // start full
		lastRefill: clock.Now(),
	}
}

// take attempts to consume 1 token. Returns true if allowed, false if denied.
func (b *tokenBucket) take(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill tokens based on time elapsed
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		// Refill rate = rpm / 60 tokens per second
		refillRate := float64(b.rpm) / 60.0
		tokensToAdd := elapsed * refillRate
		b.tokens += tokensToAdd
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.lastRefill = now
	}

	// Try to consume 1 token
	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

// ContextKey for storing rate limit info in context.
type contextKey string

const rateLimitInfoKey contextKey = "ratelimit_info"

// Info contains rate limit metadata for adding to response headers.
type Info struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// WithInfo attaches rate limit info to the context.
func WithInfo(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, rateLimitInfoKey, info)
}

// InfoFromContext retrieves rate limit info from the context.
func InfoFromContext(ctx context.Context) (Info, bool) {
	info, ok := ctx.Value(rateLimitInfoKey).(Info)
	return info, ok
}
