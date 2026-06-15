package circuitbreaker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
)

// RedisBreaker is a distributed 3-state circuit breaker backed by Redis.
type RedisBreaker struct {
	client       *redis.Client
	cfg          *config.CircuitBreakerConfig
	keyPrefix    string
	failOpen     bool
	opTimeout    time.Duration
	log          *slog.Logger
	lastErrLog   time.Time
	lastErrLogMu sync.Mutex
}

// NewRedisBreaker creates a new RedisBreaker and verifies the connection.
func NewRedisBreaker(cfg *config.CircuitBreakerConfig, log *slog.Logger) (*RedisBreaker, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  time.Duration(cfg.Redis.DialTimeoutMs) * time.Millisecond,
		ReadTimeout:  time.Duration(cfg.Redis.OpTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(cfg.Redis.OpTimeoutMs) * time.Millisecond,
		PoolSize:     50,
		MinIdleConns: 10,
		MaxRetries:   2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Redis.DialTimeoutMs)*time.Millisecond)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("circuit breaker redis ping failed: %w", err)
	}

	return &RedisBreaker{
		client:    client,
		cfg:       cfg,
		keyPrefix: cfg.Redis.KeyPrefix,
		failOpen:  cfg.Redis.FailOpen,
		opTimeout: time.Duration(cfg.Redis.OpTimeoutMs) * time.Millisecond,
		log:       log,
	}, nil
}

// stateKey returns the Redis hash key for a provider's CB state.
// Uses hash tag {provider} to guarantee same cluster slot as window keys.
func (b *RedisBreaker) stateKey(provider string) string {
	return b.keyPrefix + "{" + provider + "}:state"
}

// winKey returns the Redis hash key for a time-bucket window counter.
func (b *RedisBreaker) winKey(provider string, bucketTS int64) string {
	return b.keyPrefix + "{" + provider + "}:win:" + strconv.FormatInt(bucketTS, 10)
}

// baseKey returns the key prefix with hash tag (used by the Lua Report script).
func (b *RedisBreaker) baseKey(provider string) string {
	return b.keyPrefix + "{" + provider + "}"
}

// Allow implements Breaker. Returns (allowed, isProbe, err).
func (b *RedisBreaker) Allow(ctx context.Context, provider string) (bool, bool, error) {
	pcfg := b.cfg.ProviderConfig(provider)
	if !pcfg.Enabled {
		return true, false, nil
	}

	if b.client == nil {
		gatewayotel.CBRedisErrorCounter.WithLabelValues("client_nil").Inc()
		if b.failOpen {
			return true, false, fmt.Errorf("circuit breaker redis client nil")
		}
		return false, false, fmt.Errorf("circuit breaker redis client nil")
	}

	opCtx, cancel := context.WithTimeout(ctx, b.opTimeout)
	defer cancel()

	now := time.Now().Unix()
	result, err := b.client.Eval(opCtx, luaCBAllow,
		[]string{b.stateKey(provider)},
		now,
		pcfg.HalfOpenMaxInflight,
	).Result()

	if err != nil {
		gatewayotel.CBRedisErrorCounter.WithLabelValues("allow_eval").Inc()
		b.throttledErrLog(ctx, "circuit breaker allow eval error", provider, err)
		if b.failOpen {
			return true, false, err
		}
		return false, false, err
	}

	vals, ok := result.([]interface{})
	if !ok || len(vals) != 3 {
		gatewayotel.CBRedisErrorCounter.WithLabelValues("allow_parse").Inc()
		if b.failOpen {
			return true, false, fmt.Errorf("circuit breaker: invalid allow response")
		}
		return false, false, fmt.Errorf("circuit breaker: invalid allow response")
	}

	allowed := vals[0].(int64) == 1
	stateStr := vals[1].(string)
	isProbe := vals[2].(int64) == 1

	if !allowed {
		gatewayotel.CBDeniedCounter.WithLabelValues(provider, stateStr).Inc()
	}

	return allowed, isProbe, nil
}

// Report implements Breaker. Records the outcome of an upstream call.
func (b *RedisBreaker) Report(ctx context.Context, provider string, outcome Outcome, isProbe bool) error {
	pcfg := b.cfg.ProviderConfig(provider)
	if !pcfg.Enabled {
		return nil
	}

	if b.client == nil {
		gatewayotel.CBRedisErrorCounter.WithLabelValues("client_nil").Inc()
		return fmt.Errorf("circuit breaker redis client nil")
	}

	now := time.Now().Unix()
	bucketSize := int64(pcfg.BucketSizeSeconds)
	bucketTS := (now / bucketSize) * bucketSize
	windowBuckets := pcfg.WindowSeconds / pcfg.BucketSizeSeconds
	bucketTTL := int64(pcfg.WindowSeconds) + bucketSize // a bit more than window

	isProbeInt := 0
	if isProbe {
		isProbeInt = 1
	}
	outcomeInt := 0
	if outcome == OutcomeFailure {
		outcomeInt = 1
	}

	opCtx, cancel := context.WithTimeout(ctx, b.opTimeout)
	defer cancel()

	result, err := b.client.Eval(opCtx, luaCBReport,
		[]string{b.stateKey(provider), b.winKey(provider, bucketTS)},
		now,
		outcomeInt,
		isProbeInt,
		pcfg.OpenCooldownSeconds,
		pcfg.HalfOpenSuccessesToClose,
		bucketTTL,
		windowBuckets,
		bucketSize,
		pcfg.MinRequests,
		strconv.FormatFloat(pcfg.FailureRateThreshold, 'f', -1, 64),
		b.baseKey(provider),
	).Result()

	if err != nil {
		gatewayotel.CBRedisErrorCounter.WithLabelValues("report_eval").Inc()
		b.throttledErrLog(ctx, "circuit breaker report eval error", provider, err)
		return err
	}

	vals, ok := result.([]interface{})
	if !ok || len(vals) != 4 {
		gatewayotel.CBRedisErrorCounter.WithLabelValues("report_parse").Inc()
		return fmt.Errorf("circuit breaker: invalid report response")
	}

	transitioned := vals[0].(int64) == 1
	if transitioned {
		fromState := vals[1].(string)
		toState := vals[2].(string)
		reason := vals[3].(string)
		gatewayotel.CBTransitionsCounter.WithLabelValues(provider, fromState, toState, reason).Inc()
		b.log.Info("circuit breaker state transition",
			slog.String("provider", provider),
			slog.String("from", fromState),
			slog.String("to", toState),
			slog.String("reason", reason),
		)
	}

	return nil
}

// Close closes the underlying Redis client.
func (b *RedisBreaker) Close() error {
	return b.client.Close()
}

// throttledErrLog logs Redis errors at most once per 5 seconds to avoid log spam.
func (b *RedisBreaker) throttledErrLog(ctx context.Context, msg, provider string, err error) {
	b.lastErrLogMu.Lock()
	defer b.lastErrLogMu.Unlock()
	if time.Since(b.lastErrLog) < 5*time.Second {
		return
	}
	b.lastErrLog = time.Now()
	b.log.WarnContext(ctx, msg,
		slog.String("provider", provider),
		slog.String("error", err.Error()),
	)
}
