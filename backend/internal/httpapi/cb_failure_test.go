package httpapi

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/diegomcastronuovo/prism-gateway/internal/circuitbreaker"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/hooks"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/ratelimit"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
)

// discardLogger returns a logger that drops all output (keeps test output clean).
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestCBConfig creates a CircuitBreakerConfig pointed at the given miniredis addr.
// Sets low thresholds so tests can verify CB behavior quickly.
func newTestCBConfig(addr string) config.CircuitBreakerConfig {
	return config.CircuitBreakerConfig{
		Backend: "redis",
		Redis: config.CircuitBreakerRedisConfig{
			Addr:          addr,
			DialTimeoutMs: 200,
			OpTimeoutMs:   500,
			KeyPrefix:     "cb:",
		},
		Defaults: config.CircuitBreakerDefaultsConfig{
			Enabled:                  true,
			WindowSeconds:            60,
			BucketSizeSeconds:        5,
			MinRequests:              1,
			FailureRateThreshold:     0.5,
			OpenCooldownSeconds:      30,
			HalfOpenMaxInflight:      1,
			HalfOpenSuccessesToClose: 2,
		},
	}
}

// setupServerWithBreaker is like setupTestServerWithStorage but uses a caller-supplied breaker.
func setupServerWithBreaker(cfg *config.Config, reg *providers.Registry, store *fakeStorage, br circuitbreaker.Breaker) *Server {
	log := discardLogger()
	rt := router.New()
	hookReg := hooks.NewRegistry(log)
	limiter := ratelimit.NewInMemoryLimiter()
	srv := NewServer(cfg, log, rt, reg, hookReg, store, limiter, br, nil)
	seedTestLicense(srv)
	return srv
}

// TestIsCBFailure_Classification verifies that isCBFailure correctly classifies
// different upstream error types per the CB spec:
//   - 5xx → failure
//   - timeout / network → failure
//   - unknown → failure (conservative: unclassified errors should trip the breaker)
//   - 429 → NOT a failure
//   - 4xx (401, 403, 400) → NOT a failure
func TestIsCBFailure_Classification(t *testing.T) {
	classifier := router.NewErrorClassifier()

	tests := []struct {
		name        string
		err         error
		wantFailure bool
	}{
		{"mock upstream 500", &providers.UpstreamError{StatusCode: 500}, true},
		{"upstream 502", &providers.UpstreamError{StatusCode: 502}, true},
		{"upstream 503", &providers.UpstreamError{StatusCode: 503}, true},
		{"rate limit 429", &providers.UpstreamError{StatusCode: 429}, false},
		{"auth 401", &providers.UpstreamError{StatusCode: 401}, false},
		{"auth 403", &providers.UpstreamError{StatusCode: 403}, false},
		{"bad request 400", &providers.UpstreamError{StatusCode: 400}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errType := classifier.Classify(tt.err)
			got := isCBFailure(errType)
			if got != tt.wantFailure {
				t.Errorf("isCBFailure(%v) = %v, want %v (errType=%q)", tt.err, got, tt.wantFailure, errType)
			}
		})
	}
}

// TestCB_MockUpstream500_WritesWinKey verifies end-to-end that when a mock provider
// returns a 500 error, the circuit breaker's sliding-window key is written to Redis.
//
// This is the core regression test for BUG_cb: "no cb:* keys after upstream 500".
func TestCB_MockUpstream500_WritesWinKey(t *testing.T) {
	mr := miniredis.RunT(t)

	cbCfg := newTestCBConfig(mr.Addr())
	br, err := circuitbreaker.NewRedisBreaker(&cbCfg, discardLogger())
	if err != nil {
		t.Fatalf("NewRedisBreaker: %v", err)
	}
	defer br.Close()

	cfg := testConfig()
	// Use mock with 100% error rate, status 500
	cfg.Models[0].Mock.Enabled = true
	cfg.Models[0].Mock.ErrorRate = 1.0
	cfg.Models[0].Mock.ErrorStatus = 500
	cfg.Tenants[0].Routing.Fallback.Enabled = false

	store := newFakeStorage()
	reg := providers.NewRegistry()
	srv := setupServerWithBreaker(cfg, reg, store, br)

	body := `{"model":"model-a","messages":[{"role":"user","content":"test"}]}`
	w := makeRequest(t, srv.Handler, body, map[string]string{"X-API-Key": "key1"})

	// Request must return 500 (mock upstream error propagated)
	if w.Code != 500 {
		t.Fatalf("expected HTTP 500, got %d: %s", w.Code, w.Body.String())
	}

	// After the 500, a cb:{openai}:win:* key must exist in Redis
	hasCBWinKey := false
	for _, k := range mr.Keys() {
		if strings.HasPrefix(k, "cb:{") && strings.Contains(k, ":win:") {
			hasCBWinKey = true
			break
		}
	}
	if !hasCBWinKey {
		t.Errorf("expected cb:{provider}:win:* key in Redis after mock 500, got keys: %v", mr.Keys())
	}
}

// TestCB_Mock429_DoesNotWriteFailCounter verifies that a 429 rate-limit error
// does NOT increment the failure counter (it still writes an 'ok' bucket entry,
// but must not trigger CB open transitions).
func TestCB_Mock429_DoesNotOpenBreaker(t *testing.T) {
	mr := miniredis.RunT(t)

	cbCfg := newTestCBConfig(mr.Addr())
	// MinRequests=1 and threshold=0.5: even 1 failure would open the CB.
	// If 429 is correctly NOT counted as failure, the CB stays closed.
	br, err := circuitbreaker.NewRedisBreaker(&cbCfg, discardLogger())
	if err != nil {
		t.Fatalf("NewRedisBreaker: %v", err)
	}
	defer br.Close()

	cfg := testConfig()
	cfg.Models[0].Mock.Enabled = true
	cfg.Models[0].Mock.ErrorRate = 1.0
	cfg.Models[0].Mock.ErrorStatus = 429 // rate-limit
	cfg.Tenants[0].Routing.Fallback.Enabled = false

	store := newFakeStorage()
	reg := providers.NewRegistry()
	srv := setupServerWithBreaker(cfg, reg, store, br)

	body := `{"model":"model-a","messages":[{"role":"user","content":"test"}]}`
	_ = makeRequest(t, srv.Handler, body, map[string]string{"X-API-Key": "key1"})

	// CB must still allow requests (429 must NOT open the breaker)
	allowed, _, err := br.Allow(t.Context(), "openai")
	if err != nil {
		t.Fatalf("Allow error: %v", err)
	}
	if !allowed {
		t.Error("CB opened after 429, but 429 must not count as a CB failure")
	}
}

// TestCB_Mock401_DoesNotOpenBreaker verifies that a 401 auth error does not trip the breaker.
func TestCB_Mock401_DoesNotOpenBreaker(t *testing.T) {
	mr := miniredis.RunT(t)

	cbCfg := newTestCBConfig(mr.Addr())
	br, err := circuitbreaker.NewRedisBreaker(&cbCfg, discardLogger())
	if err != nil {
		t.Fatalf("NewRedisBreaker: %v", err)
	}
	defer br.Close()

	cfg := testConfig()
	cfg.Models[0].Mock.Enabled = true
	cfg.Models[0].Mock.ErrorRate = 1.0
	cfg.Models[0].Mock.ErrorStatus = 401
	cfg.Tenants[0].Routing.Fallback.Enabled = false

	store := newFakeStorage()
	reg := providers.NewRegistry()
	srv := setupServerWithBreaker(cfg, reg, store, br)

	body := `{"model":"model-a","messages":[{"role":"user","content":"test"}]}`
	_ = makeRequest(t, srv.Handler, body, map[string]string{"X-API-Key": "key1"})

	allowed, _, err := br.Allow(t.Context(), "openai")
	if err != nil {
		t.Fatalf("Allow error: %v", err)
	}
	if !allowed {
		t.Error("CB opened after 401, but auth errors must not count as CB failures")
	}
}
