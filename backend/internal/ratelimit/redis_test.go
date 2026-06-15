package ratelimit

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestRedisLimiter(t *testing.T, addr string) *RedisLimiter {
	cfg := config.RedisLimiterConfig{
		Addr:          addr,
		Password:      "",
		DB:            0,
		DialTimeoutMs: 200,
		OpTimeoutMs:   100,
		KeyPrefix:     "rl:",
		FailOpen:      false,
	}

	limiter, err := NewRedisLimiter(cfg, testLogger())
	if err != nil {
		t.Fatalf("failed to create redis limiter: %v", err)
	}

	return limiter
}

func TestRedisLimiter_BurstAllowance(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	ctx := context.Background()

	// First 5 requests should succeed (burst=5)
	for i := 0; i < 5; i++ {
		allowed, _, _ := limiter.Allow(ctx, "tenant1", 60, 5)
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied (burst exhausted)
	allowed, _, _ := limiter.Allow(ctx, "tenant1", 60, 5)
	if allowed {
		t.Error("request should be denied after burst exhausted")
	}
}

func TestRedisLimiter_TokenRefill(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	ctx := context.Background()

	// Exhaust burst
	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, "tenant1", 60, 5)
	}

	// Wait for real time to pass (miniredis FastForward doesn't affect time.Now() in Lua script)
	time.Sleep(1100 * time.Millisecond) // 1.1 seconds to ensure 1 token refills

	// 1 token should have refilled (60 RPM = 1 token/sec)
	allowed, _, _ := limiter.Allow(ctx, "tenant1", 60, 5)
	if !allowed {
		t.Error("token should have refilled after 1 second")
	}

	// Next request should be denied (only 1 token refilled)
	allowed, _, _ = limiter.Allow(ctx, "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied after consuming refilled token")
	}
}

func TestRedisLimiter_PerTenantIsolation(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	ctx := context.Background()

	// Exhaust tenant1
	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, "tenant1", 60, 5)
	}

	// tenant2 should still have full burst
	allowed, _, _ := limiter.Allow(ctx, "tenant2", 60, 5)
	if !allowed {
		t.Error("tenant2 should not be affected by tenant1 exhaustion")
	}
}

func TestRedisLimiter_FailOpen(t *testing.T) {
	// Create a limiter with a non-existent Redis (simulate connection failure during operation)
	limiter := &RedisLimiter{
		client:    nil, // Simulate disconnected client
		keyPrefix: "rl:",
		failOpen:  true,
		opTimeout: 100 * time.Millisecond,
		log:       testLogger(),
	}

	ctx := context.Background()
	allowed, _, _ := limiter.Allow(ctx, "tenant1", 60, 10)
	if !allowed {
		t.Error("should allow when fail_open=true and Redis is down")
	}
}

func TestRedisLimiter_FailClosed(t *testing.T) {
	limiter := &RedisLimiter{
		client:    nil, // Simulate disconnected client
		keyPrefix: "rl:",
		failOpen:  false,
		opTimeout: 100 * time.Millisecond,
		log:       testLogger(),
	}

	ctx := context.Background()
	allowed, _, _ := limiter.Allow(ctx, "tenant1", 60, 10)
	if allowed {
		t.Error("should deny when fail_open=false and Redis is down")
	}
}

func TestRedisLimiter_ConcurrentRequests(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	ctx := context.Background()

	// Simulate 20 concurrent requests (burst=10)
	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _, _ := limiter.Allow(ctx, "tenant1", 60, 10)
			if allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Only 10 should be allowed (burst capacity)
	if allowedCount != 10 {
		t.Errorf("expected 10 allowed requests, got %d", allowedCount)
	}
}

func TestRedisLimiter_ScopedKeying_Tenant(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	// Create context with tenant config (scope: tenant)
	tenant := &config.TenantConfig{
		ID: "tenant1",
		RateLimit: config.RateLimitConfig{
			RPM:   60,
			Burst: 5,
			Scope: "tenant",
		},
	}

	ctx := context.Background()
	ctx = auth.WithTenant(ctx, tenant)

	// Exhaust rate limit
	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, "tenant1", 60, 5)
	}

	// Next request should be denied
	allowed, _, _ := limiter.Allow(ctx, "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied after exhausting tenant-level rate limit")
	}
}

func TestRedisLimiter_ScopedKeying_JWTSub(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	ctx := context.Background()

	// The middleware pre-computes the bucket key before calling Allow.
	// Simulate what the middleware produces for two different users.
	user1Key := "tenant:tenant1:user:" + HashSub("user1")
	user2Key := "tenant:tenant1:user:" + HashSub("user2")

	// Exhaust user1's rate limit
	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, user1Key, 60, 5)
	}

	// user1 should be denied
	allowed, _, _ := limiter.Allow(ctx, user1Key, 60, 5)
	if allowed {
		t.Error("user1 should be denied after exhausting their rate limit")
	}

	// user2 has an independent bucket and must still be allowed
	allowed, _, _ = limiter.Allow(ctx, user2Key, 60, 5)
	if !allowed {
		t.Error("user2 should not be affected by user1's rate limit")
	}
}

func TestRedisLimiter_ScopedKeying_JWTSub_FallbackToTenant(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	// Create context with tenant config (scope: jwt_sub) but API key auth (no sub)
	tenant := &config.TenantConfig{
		ID: "tenant1",
		RateLimit: config.RateLimitConfig{
			RPM:   60,
			Burst: 5,
			Scope: "jwt_sub",
		},
	}

	ctx := context.Background()
	ctx = auth.WithTenant(ctx, tenant)
	ctx = auth.WithAuthType(ctx, "api_key") // No JWT sub

	// Should fallback to tenant-level keying
	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, "tenant1", 60, 5)
	}

	// Next request should be denied (using tenant-level bucket)
	allowed, _, _ := limiter.Allow(ctx, "tenant1", 60, 5)
	if allowed {
		t.Error("should be denied after exhausting tenant-level rate limit (fallback)")
	}
}

func TestRedisLimiter_ResetTime(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	ctx := context.Background()

	// Exhaust burst
	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, "tenant1", 60, 5)
	}

	// Check reset time on denied request
	_, _, resetAt := limiter.Allow(ctx, "tenant1", 60, 5)

	// Reset time should be in the future (approximately 5 seconds for 5 tokens at 1 token/sec)
	resetIn := time.Until(resetAt)
	if resetIn < 4*time.Second || resetIn > 6*time.Second {
		t.Errorf("reset time should be ~5 seconds in the future, got %v", resetIn)
	}
}

func TestRedisLimiter_DifferentRPM(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	limiter := newTestRedisLimiter(t, mr.Addr())
	defer limiter.Close()

	ctx := context.Background()

	// Test with 120 RPM (2 tokens/sec)
	// Exhaust burst
	for i := 0; i < 10; i++ {
		limiter.Allow(ctx, "tenant1", 120, 10)
	}

	// Wait for real time to pass (1.1 seconds)
	time.Sleep(1100 * time.Millisecond)

	// Should have 2 tokens refilled
	allowed, _, _ := limiter.Allow(ctx, "tenant1", 120, 10)
	if !allowed {
		t.Error("should be allowed after refilling 2 tokens")
	}

	allowed, _, _ = limiter.Allow(ctx, "tenant1", 120, 10)
	if !allowed {
		t.Error("should be allowed for 2nd token")
	}

	// 3rd should be denied
	allowed, _, _ = limiter.Allow(ctx, "tenant1", 120, 10)
	if allowed {
		t.Error("should be denied after consuming 2 refilled tokens")
	}
}
