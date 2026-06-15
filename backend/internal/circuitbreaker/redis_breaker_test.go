package circuitbreaker

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newTestBreaker creates a RedisBreaker connected to a miniredis instance.
func newTestBreaker(t *testing.T, addr string, overrides ...func(*config.CircuitBreakerConfig)) *RedisBreaker {
	t.Helper()
	cfg := &config.CircuitBreakerConfig{
		Backend: "redis",
		Redis: config.CircuitBreakerRedisConfig{
			Addr:          addr,
			DialTimeoutMs: 200,
			OpTimeoutMs:   200,
			KeyPrefix:     "cb:",
			FailOpen:      false,
		},
		Defaults: config.CircuitBreakerDefaultsConfig{
			Enabled:                  true,
			WindowSeconds:            60,
			BucketSizeSeconds:        5,
			MinRequests:              5,
			FailureRateThreshold:     0.5,
			OpenCooldownSeconds:      30,
			HalfOpenMaxInflight:      1,
			HalfOpenSuccessesToClose: 2,
		},
	}
	for _, o := range overrides {
		o(cfg)
	}
	b, err := NewRedisBreaker(cfg, testLogger())
	require.NoError(t, err)
	t.Cleanup(func() { b.Close() })
	return b
}

func TestAllow_Closed_Allowed(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())

	allowed, isProbe, err := b.Allow(context.Background(), "openai")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.False(t, isProbe)
}

func TestAllow_Open_BeforeCooldown_Denied(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())
	ctx := context.Background()

	// Manually set state to open with open_until in the future
	stateKey := b.stateKey("openai")
	openUntil := time.Now().Add(30 * time.Second).Unix()
	err := b.client.HMSet(ctx, stateKey,
		"state", "open",
		"open_until", openUntil,
		"inflight", 0,
	).Err()
	require.NoError(t, err)

	allowed, isProbe, err := b.Allow(ctx, "openai")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.False(t, isProbe)
}

func TestAllow_Open_AfterCooldown_AllowedAsProbe(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())
	ctx := context.Background()

	// Set state to open with open_until in the past
	stateKey := b.stateKey("openai")
	openUntil := time.Now().Add(-1 * time.Second).Unix()
	err := b.client.HMSet(ctx, stateKey,
		"state", "open",
		"open_until", openUntil,
		"inflight", 0,
		"successes", 0,
	).Err()
	require.NoError(t, err)

	allowed, isProbe, err := b.Allow(ctx, "openai")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.True(t, isProbe) // should be in half_open now
}

func TestAllow_HalfOpen_InflightGate(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())
	ctx := context.Background()

	// Set state to half_open with inflight == max (1)
	stateKey := b.stateKey("openai")
	err := b.client.HMSet(ctx, stateKey,
		"state", "half_open",
		"inflight", 1, // equals HalfOpenMaxInflight
		"successes", 0,
	).Err()
	require.NoError(t, err)

	allowed, isProbe, err := b.Allow(ctx, "openai")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.False(t, isProbe)
}

func TestReport_ClosedToOpen_FailureRate(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())
	ctx := context.Background()

	// Send min_requests failures to trigger open (MinRequests=5, threshold=0.5)
	// We need at least 5 requests and >= 50% failures
	// Send 5 failures
	for i := 0; i < 5; i++ {
		err := b.Report(ctx, "openai", OutcomeFailure, false)
		require.NoError(t, err)
	}

	// CB should now be open
	stateKey := b.stateKey("openai")
	state, err := b.client.HGet(ctx, stateKey, "state").Result()
	require.NoError(t, err)
	assert.Equal(t, "open", state)
}

func TestReport_HalfOpen_SuccessesToClose(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())
	ctx := context.Background()

	// Put into half_open
	stateKey := b.stateKey("openai")
	err := b.client.HMSet(ctx, stateKey,
		"state", "half_open",
		"inflight", 0,
		"successes", 0,
	).Err()
	require.NoError(t, err)

	// Send HalfOpenSuccessesToClose (2) probe successes
	for i := 0; i < 2; i++ {
		err := b.Report(ctx, "openai", OutcomeSuccess, true)
		require.NoError(t, err)
	}

	// Key should be deleted (closed state = no key)
	exists, err := b.client.Exists(ctx, stateKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func TestReport_HalfOpen_FailureToOpen(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())
	ctx := context.Background()

	stateKey := b.stateKey("openai")
	err := b.client.HMSet(ctx, stateKey,
		"state", "half_open",
		"inflight", 1,
		"successes", 0,
	).Err()
	require.NoError(t, err)

	err = b.Report(ctx, "openai", OutcomeFailure, true)
	require.NoError(t, err)

	state, err := b.client.HGet(ctx, stateKey, "state").Result()
	require.NoError(t, err)
	assert.Equal(t, "open", state)
}

func TestInflight_ReleasedOnReport(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())
	ctx := context.Background()

	stateKey := b.stateKey("openai")
	err := b.client.HMSet(ctx, stateKey,
		"state", "half_open",
		"inflight", 1,
		"successes", 0,
	).Err()
	require.NoError(t, err)

	// Report a success as probe — inflight should drop by 1 (then successes incremented)
	err = b.Report(ctx, "openai", OutcomeSuccess, true)
	require.NoError(t, err)

	// inflight should have been decremented (from 1 to 0), then successes = 1
	// State is still half_open (need 2 successes to close)
	inflight, err := b.client.HGet(ctx, stateKey, "inflight").Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(0), inflight)
}

func TestTwoProviders_Independent(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr())
	ctx := context.Background()

	// Open openai circuit
	stateKey := b.stateKey("openai")
	openUntil := time.Now().Add(30 * time.Second).Unix()
	err := b.client.HMSet(ctx, stateKey,
		"state", "open",
		"open_until", openUntil,
	).Err()
	require.NoError(t, err)

	// openai should be denied
	allowed, _, err := b.Allow(ctx, "openai")
	require.NoError(t, err)
	assert.False(t, allowed)

	// anthropic should still be allowed (independent)
	allowed, _, err = b.Allow(ctx, "anthropic")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestFailOpen_RedisDown(t *testing.T) {
	b := &RedisBreaker{
		client:    nil, // simulate disconnected
		cfg:       &config.CircuitBreakerConfig{Defaults: config.CircuitBreakerDefaultsConfig{Enabled: true, HalfOpenMaxInflight: 1}},
		keyPrefix: "cb:",
		failOpen:  true,
		opTimeout: 100 * time.Millisecond,
		log:       testLogger(),
	}

	allowed, isProbe, err := b.Allow(context.Background(), "openai")
	assert.Error(t, err)
	assert.True(t, allowed)
	assert.False(t, isProbe)
}

func TestFailClosed_RedisDown(t *testing.T) {
	b := &RedisBreaker{
		client:    nil, // simulate disconnected
		cfg:       &config.CircuitBreakerConfig{Defaults: config.CircuitBreakerDefaultsConfig{Enabled: true, HalfOpenMaxInflight: 1}},
		keyPrefix: "cb:",
		failOpen:  false,
		opTimeout: 100 * time.Millisecond,
		log:       testLogger(),
	}

	allowed, isProbe, err := b.Allow(context.Background(), "openai")
	assert.Error(t, err)
	assert.False(t, allowed)
	assert.False(t, isProbe)
}

func TestAllow_Disabled_AlwaysAllowed(t *testing.T) {
	mr := miniredis.RunT(t)
	b := newTestBreaker(t, mr.Addr(), func(cfg *config.CircuitBreakerConfig) {
		cfg.Providers = map[string]config.CircuitBreakerProviderConfig{
			"openai": {Enabled: boolPtr(false)},
		}
	})

	// Even with an open state key, disabled provider always passes
	stateKey := b.stateKey("openai")
	openUntil := time.Now().Add(30 * time.Second).Unix()
	err := b.client.HMSet(context.Background(), stateKey,
		"state", "open",
		"open_until", openUntil,
	).Err()
	require.NoError(t, err)

	allowed, isProbe, err := b.Allow(context.Background(), "openai")
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.False(t, isProbe)
}

// helper to create breaker directly with a redis.Client (for testing without ping)
func newBreakerWithClient(cfg *config.CircuitBreakerConfig, client *redis.Client) *RedisBreaker {
	return &RedisBreaker{
		client:    client,
		cfg:       cfg,
		keyPrefix: cfg.Redis.KeyPrefix,
		failOpen:  cfg.Redis.FailOpen,
		opTimeout: time.Duration(cfg.Redis.OpTimeoutMs) * time.Millisecond,
		log:       testLogger(),
	}
}

func boolPtr(b bool) *bool { return &b }
