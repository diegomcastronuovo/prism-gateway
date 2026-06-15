package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockClock provides deterministic time for testing.
type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock(initial time.Time) *mockClock {
	return &mockClock{now: initial}
}

func (m *mockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

func (m *mockClock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}

func TestTokenBucket_InitialBurst(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	// burst=5 should allow 5 requests immediately
	for i := 0; i < 5; i++ {
		allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
		if !allowed {
			t.Errorf("request %d should be allowed (initial burst)", i+1)
		}
	}

	// 6th request should be denied
	allowed, remaining, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
	if allowed {
		t.Error("6th request should be denied (burst exhausted)")
	}
	if remaining != 0 {
		t.Errorf("remaining should be 0, got %d", remaining)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	// rpm=60 = 1 token per second
	// Consume initial burst
	for i := 0; i < 5; i++ {
		limiter.Allow(context.Background(), "tenant1", 60, 5)
	}

	// Should be denied now
	allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied before refill")
	}

	// Advance 1 second -> refill 1 token
	clock.Advance(1 * time.Second)
	allowed, _, _ = limiter.Allow(context.Background(), "tenant1", 60, 5)
	if !allowed {
		t.Error("should be allowed after 1 second (1 token refilled)")
	}

	// Should be denied again
	allowed, _, _ = limiter.Allow(context.Background(), "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied again (only 1 token refilled)")
	}

	// Advance 5 seconds -> refill 5 tokens, but capacity is 5, so should have 5 total
	clock.Advance(5 * time.Second)
	for i := 0; i < 5; i++ {
		allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
		if !allowed {
			t.Errorf("request %d should be allowed after refill", i+1)
		}
	}

	// 6th should be denied
	allowed, _, _ = limiter.Allow(context.Background(), "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied after consuming refilled tokens")
	}
}

func TestTokenBucket_CapacityLimit(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	// Consume 2 tokens
	limiter.Allow(context.Background(), "tenant1", 60, 5)
	limiter.Allow(context.Background(), "tenant1", 60, 5)

	// Advance 10 seconds (would refill 10 tokens, but capacity is 5)
	clock.Advance(10 * time.Second)

	// Should only have 5 tokens (capacity), not 3 + 10 = 13
	count := 0
	for i := 0; i < 10; i++ {
		allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
		if allowed {
			count++
		}
	}
	if count != 5 {
		t.Errorf("expected 5 allowed requests (capacity), got %d", count)
	}
}

func TestTokenBucket_DifferentRates(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	// rpm=120 = 2 tokens per second, burst=10
	// Consume all burst tokens
	for i := 0; i < 10; i++ {
		limiter.Allow(context.Background(), "tenant1", 120, 10)
	}

	// Should be denied now
	allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 120, 10)
	if allowed {
		t.Error("should be denied (burst exhausted)")
	}

	// Advance 1 second -> refill 2 tokens
	clock.Advance(1 * time.Second)

	// After 1 second, should have refilled 2 tokens
	allowed1, _, _ := limiter.Allow(context.Background(), "tenant1", 120, 10)
	allowed2, _, _ := limiter.Allow(context.Background(), "tenant1", 120, 10)
	allowed3, _, _ := limiter.Allow(context.Background(), "tenant1", 120, 10)

	if !allowed1 || !allowed2 {
		t.Error("first 2 requests should be allowed (2 tokens refilled)")
	}
	if allowed3 {
		t.Error("3rd request should be denied (only 2 tokens refilled)")
	}
}

func TestTokenBucket_PerTenantIsolation(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	// Exhaust tenant1's tokens
	for i := 0; i < 5; i++ {
		limiter.Allow(context.Background(), "tenant1", 60, 5)
	}
	allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
	if allowed {
		t.Error("tenant1 should be rate limited")
	}

	// tenant2 should have full burst available
	for i := 0; i < 5; i++ {
		allowed, _, _ := limiter.Allow(context.Background(), "tenant2", 60, 5)
		if !allowed {
			t.Errorf("tenant2 request %d should be allowed (independent bucket)", i+1)
		}
	}
}

func TestTokenBucket_Concurrency(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	const goroutines = 10
	const requestsPerGoroutine = 5
	const burst = 20

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowedCount := 0

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 60, burst)
				if allowed {
					mu.Lock()
					allowedCount++
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	// Total requests = 50, burst = 20, so only 20 should succeed
	if allowedCount != burst {
		t.Errorf("expected %d allowed requests, got %d", burst, allowedCount)
	}
}

func TestTokenBucket_DynamicConfigChange(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	// Start with rpm=60, burst=5
	for i := 0; i < 5; i++ {
		limiter.Allow(context.Background(), "tenant1", 60, 5)
	}

	// Should be denied
	allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied with original config")
	}

	// Change config to rpm=120, burst=10 (effective immediately)
	clock.Advance(1 * time.Second)
	// With rpm=120, should refill 2 tokens per second
	allowed, _, _ = limiter.Allow(context.Background(), "tenant1", 120, 10)
	if !allowed {
		t.Error("should be allowed with new config (refilled)")
	}
}

func TestTokenBucket_ZeroRPM(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	// Consume burst
	for i := 0; i < 5; i++ {
		limiter.Allow(context.Background(), "tenant1", 0, 5)
	}

	// rpm=0 means no refill, so should stay denied forever
	clock.Advance(10 * time.Second)
	allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 0, 5)
	if allowed {
		t.Error("should be denied (rpm=0 means no refill)")
	}
}

func TestTokenBucket_FractionalRefill(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := NewInMemoryLimiterWithClock(clock)

	// rpm=60 = 1 token per second, burst=5
	// Exhaust all tokens
	for i := 0; i < 5; i++ {
		limiter.Allow(context.Background(), "tenant1", 60, 5)
	}

	// Should be denied
	allowed, _, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied (burst exhausted)")
	}

	// Advance 0.5 seconds -> refill 0.5 tokens (not enough for 1 request)
	clock.Advance(500 * time.Millisecond)
	allowed, _, _ = limiter.Allow(context.Background(), "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied (only 0.5 tokens refilled)")
	}

	// Advance another 0.5 seconds -> total 1 token refilled
	clock.Advance(500 * time.Millisecond)
	allowed, _, _ = limiter.Allow(context.Background(), "tenant1", 60, 5)
	if !allowed {
		t.Error("should be allowed (1 token refilled)")
	}
}

func TestNopLimiter(t *testing.T) {
	limiter := NopLimiter{}

	for i := 0; i < 100; i++ {
		allowed, remaining, _ := limiter.Allow(context.Background(), "tenant1", 60, 5)
		if !allowed {
			t.Error("NopLimiter should always allow")
		}
		if remaining != 5 {
			t.Errorf("NopLimiter should return burst as remaining, got %d", remaining)
		}
	}
}
