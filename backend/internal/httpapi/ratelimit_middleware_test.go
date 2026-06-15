package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/ratelimit"
)

// mockLimiter for testing middleware.
type mockLimiter struct {
	mu           sync.Mutex
	allowedCount int
	allowed      bool
	remaining    int
	resetAt      time.Time
	lastKey      string // last bucket key passed to Allow
}

func (m *mockLimiter) Allow(ctx context.Context, key string, rpm, burst int) (bool, int, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowedCount++
	m.lastKey = key
	return m.allowed, m.remaining, m.resetAt
}

func TestRateLimitMiddleware_Allowed(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].RateLimit = config.RateLimitConfig{RPM: 60, Burst: 10}

	limiter := &mockLimiter{
		allowed:   true,
		remaining: 9,
		resetAt:   time.Unix(1700000000, 0),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %s", w.Body.String())
	}

	// Check rate limit headers
	if w.Header().Get("X-RateLimit-Limit") != "60" {
		t.Errorf("X-RateLimit-Limit = %s, want 60", w.Header().Get("X-RateLimit-Limit"))
	}
	if w.Header().Get("X-RateLimit-Remaining") != "9" {
		t.Errorf("X-RateLimit-Remaining = %s, want 9", w.Header().Get("X-RateLimit-Remaining"))
	}
	if w.Header().Get("X-RateLimit-Reset") != "1700000000" {
		t.Errorf("X-RateLimit-Reset = %s, want 1700000000", w.Header().Get("X-RateLimit-Reset"))
	}
}

func TestRateLimitMiddleware_Blocked(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].RateLimit = config.RateLimitConfig{RPM: 60, Burst: 10}

	limiter := &mockLimiter{
		allowed:   false,
		remaining: 0,
		resetAt:   time.Unix(1700000000, 0),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when rate limited")
	})

	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "rate_limited" {
		t.Errorf("expected error type 'rate_limited', got %s", resp.Error.Type)
	}
	if resp.Error.Message != "rate limit exceeded" {
		t.Errorf("expected message 'rate limit exceeded', got %s", resp.Error.Message)
	}

	// Check rate limit headers
	if w.Header().Get("X-RateLimit-Limit") != "60" {
		t.Errorf("X-RateLimit-Limit = %s, want 60", w.Header().Get("X-RateLimit-Limit"))
	}
	if w.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("X-RateLimit-Remaining = %s, want 0", w.Header().Get("X-RateLimit-Remaining"))
	}
	if w.Header().Get("X-RateLimit-Reset") != "1700000000" {
		t.Errorf("X-RateLimit-Reset = %s, want 1700000000", w.Header().Get("X-RateLimit-Reset"))
	}
}

func TestRateLimitMiddleware_NoRateLimitConfigured(t *testing.T) {
	cfg := testConfig()
	// No rate limit configured (rpm=0)

	limiter := &mockLimiter{
		allowed: true,
	}

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called when rate limit not configured")
	}
	if limiter.allowedCount != 0 {
		t.Error("limiter should not be called when rpm=0")
	}
}

func TestRateLimitMiddleware_NoTenant(t *testing.T) {
	cfg := testConfig()

	limiter := &mockLimiter{}

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	// No tenant in context
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called when no tenant in context")
	}
	if limiter.allowedCount != 0 {
		t.Error("limiter should not be called when tenant not resolved")
	}
}

func TestRateLimitMiddleware_DefaultBurst(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].RateLimit = config.RateLimitConfig{RPM: 60, Burst: 0} // burst=0 means default to rpm

	limiter := &mockLimiter{
		allowed:   true,
		remaining: 59,
		resetAt:   time.Unix(1700000000, 0),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}


// mockClock provides deterministic time for testing (same as ratelimit package).
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

// Integration test with real in-memory limiter
func TestRateLimitMiddleware_IntegrationBurstThenBlock(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimit.NewInMemoryLimiterWithClock(clock)

	cfg := testConfig()
	cfg.Tenants[0].RateLimit = config.RateLimitConfig{RPM: 60, Burst: 5}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	// First 5 requests should succeed (burst)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d should succeed (burst), got %d", i+1, w.Code)
		}

		remaining, _ := strconv.Atoi(w.Header().Get("X-RateLimit-Remaining"))
		expectedRemaining := 5 - i - 1
		if remaining != expectedRemaining {
			t.Errorf("request %d: remaining = %d, want %d", i+1, remaining, expectedRemaining)
		}
	}

	// 6th request should be blocked
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("6th request should be blocked, got %d", w.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error.Type != "rate_limited" {
		t.Errorf("expected rate_limited error, got %s", resp.Error.Type)
	}
}

func TestRateLimitMiddleware_IntegrationRefill(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimit.NewInMemoryLimiterWithClock(clock)

	cfg := testConfig()
	cfg.Tenants[0].RateLimit = config.RateLimitConfig{RPM: 60, Burst: 5} // 1 token/second

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	// Exhaust burst
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)
	}

	// Should be blocked now
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Error("should be blocked after burst exhausted")
	}

	// Advance 1 second -> refill 1 token
	clock.Advance(1 * time.Second)

	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("should be allowed after refill, got %d", w.Code)
	}

	// Should be blocked again
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Error("should be blocked again after consuming refilled token")
	}
}

// ---------- rateLimitBucketKey unit tests ----------

func TestRateLimitBucketKey_ScopeTenant(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: "tenant"},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	key, keyType := rateLimitBucketKey(req, tenant)

	if key != "tenant:acme" {
		t.Errorf("scope=tenant: expected bucket key 'tenant:acme', got %q", key)
	}
	if keyType != "tenant" {
		t.Errorf("scope=tenant: expected keyType 'tenant', got %q", keyType)
	}
}

func TestRateLimitBucketKey_ScopeTenant_EmptyScope(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: ""},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	key, keyType := rateLimitBucketKey(req, tenant)

	if key != "tenant:acme" {
		t.Errorf("empty scope: expected 'tenant:acme', got %q", key)
	}
	if keyType != "tenant" {
		t.Errorf("empty scope: expected keyType 'tenant', got %q", keyType)
	}
}

func TestRateLimitBucketKey_ScopeAPIKey_WithHeader(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: "api_key"},
	}
	apiKey := "rk_live_supersecret"
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("X-API-Key", apiKey)

	key, keyType := rateLimitBucketKey(req, tenant)

	sum := sha256.Sum256([]byte(apiKey))
	expectedSuffix := hex.EncodeToString(sum[:])
	expectedKey := "tenant:acme:api_key:" + expectedSuffix

	if key != expectedKey {
		t.Errorf("scope=api_key: expected %q, got %q", expectedKey, key)
	}
	if keyType != "api_key" {
		t.Errorf("scope=api_key: expected keyType 'api_key', got %q", keyType)
	}
	// Raw key must not appear in the bucket key.
	if strings.Contains(key, apiKey) {
		t.Error("bucket key must not contain the raw API key")
	}
}

func TestRateLimitBucketKey_ScopeAPIKey_TwoKeysDifferentBuckets(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: "api_key"},
	}

	req1 := httptest.NewRequest(http.MethodPost, "/", nil)
	req1.Header.Set("X-API-Key", "key-alice")
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("X-API-Key", "key-bob")

	key1, _ := rateLimitBucketKey(req1, tenant)
	key2, _ := rateLimitBucketKey(req2, tenant)

	if key1 == key2 {
		t.Error("two different API keys must produce different bucket keys")
	}
}

func TestRateLimitBucketKey_ScopeAPIKey_NoHeader_FallsBackToTenant(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: "api_key"},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	// No X-API-Key header set.

	key, keyType := rateLimitBucketKey(req, tenant)

	if key != "tenant:acme" {
		t.Errorf("api_key scope without header: expected 'tenant:acme', got %q", key)
	}
	if keyType != "tenant" {
		t.Errorf("api_key scope without header: expected keyType 'tenant', got %q", keyType)
	}
}

func TestRateLimitBucketKey_ScopeJWTSub_FallsBackToTenant(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: "jwt_sub"},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	key, keyType := rateLimitBucketKey(req, tenant)

	if key != "tenant:acme" {
		t.Errorf("jwt_sub scope: expected 'tenant:acme' fallback, got %q", key)
	}
	if keyType != "tenant" {
		t.Errorf("jwt_sub scope: expected keyType 'tenant', got %q", keyType)
	}
}

// ---------- Integration: per-API-key isolation ----------

// TestRateLimitMiddleware_ScopeAPIKey_TwoKeysDontShareBucket verifies that two
// clients sharing the same tenant but using different API keys exhaust their
// individual rate limit budgets independently.
func TestRateLimitMiddleware_ScopeAPIKey_TwoKeysDontShareBucket(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimit.NewInMemoryLimiterWithClock(clock)

	cfg := testConfig()
	cfg.Tenants[0].RateLimit = config.RateLimitConfig{RPM: 60, Burst: 2, Scope: "api_key"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	sendN := func(apiKey string, n int) []int {
		codes := make([]int, n)
		for i := 0; i < n; i++ {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.Header.Set("X-API-Key", apiKey)
			req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)
			codes[i] = w.Code
		}
		return codes
	}

	// Alice exhausts her burst (2 tokens) — should be 200, 200, 429.
	aliceCodes := sendN("key-alice", 3)
	if aliceCodes[0] != http.StatusOK || aliceCodes[1] != http.StatusOK {
		t.Errorf("alice: first 2 requests should succeed, got %v", aliceCodes[:2])
	}
	if aliceCodes[2] != http.StatusTooManyRequests {
		t.Errorf("alice: 3rd request should be blocked, got %d", aliceCodes[2])
	}

	// Bob has his own bucket — his first request must succeed even though Alice is blocked.
	bobCodes := sendN("key-bob", 1)
	if bobCodes[0] != http.StatusOK {
		t.Errorf("bob: first request should succeed independently of alice, got %d", bobCodes[0])
	}
}

// TestRateLimitMiddleware_ScopeAPIKey_BucketKeyNotRawKey verifies that the
// raw API key value is never passed as the bucket key to the limiter.
func TestRateLimitMiddleware_ScopeAPIKey_BucketKeyNotRawKey(t *testing.T) {
	cfg := testConfig()
	cfg.Tenants[0].RateLimit = config.RateLimitConfig{RPM: 60, Burst: 10, Scope: "api_key"}

	rawKey := "rk_live_plaintext_secret"
	limiter := &mockLimiter{allowed: true, remaining: 9, resetAt: time.Unix(1700000000, 0)}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("X-API-Key", rawKey)
	req = req.WithContext(auth.WithTenant(req.Context(), &cfg.Tenants[0]))
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	if strings.Contains(limiter.lastKey, rawKey) {
		t.Errorf("raw API key must not appear in bucket key; got %q", limiter.lastKey)
	}
	if !strings.HasPrefix(limiter.lastKey, "tenant:") {
		t.Errorf("bucket key must start with 'tenant:'; got %q", limiter.lastKey)
	}
}

// ---------- rateLimitBucketKey unit tests — jwt_sub scope ----------

func TestRateLimitBucketKey_ScopeJWTSub_WithJWT(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: "jwt_sub"},
	}
	sub := "user-abc-123"
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(auth.WithSub(auth.WithAuthType(req.Context(), "jwt"), sub))

	key, keyType := rateLimitBucketKey(req, tenant)

	expectedKey := "tenant:acme:user:" + ratelimit.HashSub(sub)
	if key != expectedKey {
		t.Errorf("scope=jwt_sub: expected %q, got %q", expectedKey, key)
	}
	if keyType != "jwt_sub" {
		t.Errorf("scope=jwt_sub: expected keyType 'jwt_sub', got %q", keyType)
	}
	// Raw sub must not appear anywhere in the bucket key.
	if strings.Contains(key, sub) {
		t.Error("bucket key must not contain the raw JWT sub")
	}
}

func TestRateLimitBucketKey_ScopeJWTSub_NoJWT_Fallback(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: "jwt_sub"},
	}
	// API-key auth — no JWT sub in context.
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(auth.WithAuthType(req.Context(), "api_key"))

	key, keyType := rateLimitBucketKey(req, tenant)

	if key != "tenant:acme" {
		t.Errorf("jwt_sub scope, api_key auth: expected 'tenant:acme' fallback, got %q", key)
	}
	if keyType != "tenant" {
		t.Errorf("jwt_sub scope, api_key auth: expected keyType 'tenant', got %q", keyType)
	}
}

func TestRateLimitBucketKey_ScopeJWTSub_EmptySub_Fallback(t *testing.T) {
	tenant := &config.TenantConfig{
		ID:        "acme",
		RateLimit: config.RateLimitConfig{RPM: 60, Scope: "jwt_sub"},
	}
	// JWT auth but sub is empty string.
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(auth.WithSub(auth.WithAuthType(req.Context(), "jwt"), ""))

	key, keyType := rateLimitBucketKey(req, tenant)

	if key != "tenant:acme" {
		t.Errorf("jwt_sub scope, empty sub: expected 'tenant:acme' fallback, got %q", key)
	}
	if keyType != "tenant" {
		t.Errorf("jwt_sub scope, empty sub: expected keyType 'tenant', got %q", keyType)
	}
}

// ---------- Integration: per-JWT-user isolation ----------

// TestRateLimitMiddleware_ScopeJWTSub_TwoUsersIndependent verifies that two
// JWT users within the same tenant each have their own independent token bucket.
func TestRateLimitMiddleware_ScopeJWTSub_TwoUsersIndependent(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	limiter := ratelimit.NewInMemoryLimiterWithClock(clock)

	cfg := testConfig()
	cfg.Tenants[0].RateLimit = config.RateLimitConfig{RPM: 60, Burst: 2, Scope: "jwt_sub"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(cfg, testLogger(), limiter)
	wrapped := middleware(handler)

	sendNAsUser := func(sub string, n int) []int {
		codes := make([]int, n)
		for i := 0; i < n; i++ {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			ctx := auth.WithSub(auth.WithAuthType(req.Context(), "jwt"), sub)
			req = req.WithContext(auth.WithTenant(ctx, &cfg.Tenants[0]))
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)
			codes[i] = w.Code
		}
		return codes
	}

	// Alice exhausts her burst (2 tokens) — should be 200, 200, 429.
	aliceCodes := sendNAsUser("alice@example.com", 3)
	if aliceCodes[0] != http.StatusOK || aliceCodes[1] != http.StatusOK {
		t.Errorf("alice: first 2 requests should succeed, got %v", aliceCodes[:2])
	}
	if aliceCodes[2] != http.StatusTooManyRequests {
		t.Errorf("alice: 3rd request should be blocked, got %d", aliceCodes[2])
	}

	// Bob has his own bucket — his first request must succeed even though Alice is blocked.
	bobCodes := sendNAsUser("bob@example.com", 1)
	if bobCodes[0] != http.StatusOK {
		t.Errorf("bob: first request should succeed independently of alice, got %d", bobCodes[0])
	}
}
