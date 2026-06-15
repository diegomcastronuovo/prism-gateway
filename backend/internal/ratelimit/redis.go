package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
)

// luaScript implements token bucket algorithm atomically in Redis
// KEYS[1] = rate limit key (e.g., "rl:tenant:acme-corp")
// ARGV[1] = max_tokens (burst capacity)
// ARGV[2] = refill_rate (tokens per second, as float)
// ARGV[3] = now (current Unix timestamp in seconds)
//
// Returns: {allowed (0|1), remaining_tokens, reset_at_timestamp}
const luaScript = `
local key = KEYS[1]
local max_tokens = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

-- Get current state or initialize
local data = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

-- Initialize if first access
if tokens == nil then
    tokens = max_tokens
    last_refill = now
end

-- Calculate token refill
local elapsed = now - last_refill
local refill = elapsed * refill_rate
tokens = math.min(max_tokens, tokens + refill)

-- Try to consume 1 token
local allowed = 0
local remaining = tokens
if tokens >= 1 then
    tokens = tokens - 1
    allowed = 1
    remaining = tokens
end

-- Update state in Redis
redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)

-- Set expiry (2 minutes of inactivity cleans up keys)
redis.call('EXPIRE', key, 120)

-- Calculate reset time (when bucket refills to max)
local tokens_to_refill = max_tokens - tokens
local reset_at = now + math.ceil(tokens_to_refill / refill_rate)

return {allowed, remaining, reset_at}
`

// RedisLimiter implements distributed rate limiting using Redis with token bucket algorithm
type RedisLimiter struct {
	client    *redis.Client
	keyPrefix string
	failOpen  bool
	opTimeout time.Duration
	log       *slog.Logger
}

// NewRedisLimiter creates a new Redis-based rate limiter
func NewRedisLimiter(cfg config.RedisLimiterConfig, log *slog.Logger) (*RedisLimiter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  time.Duration(cfg.DialTimeoutMs) * time.Millisecond,
		ReadTimeout:  time.Duration(cfg.OpTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(cfg.OpTimeoutMs) * time.Millisecond,
		PoolSize:     50, // Handle ~500 req/s concurrency
		MinIdleConns: 10, // Keep warm connections
		MaxRetries:   2,  // Retry failed operations
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.DialTimeoutMs)*time.Millisecond)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	log.Info("redis limiter connected",
		"addr", cfg.Addr,
		"db", cfg.DB,
		"fail_open", cfg.FailOpen,
	)

	return &RedisLimiter{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
		failOpen:  cfg.FailOpen,
		opTimeout: time.Duration(cfg.OpTimeoutMs) * time.Millisecond,
		log:       log,
	}, nil
}

// Allow implements the Limiter interface for Redis backend
func (r *RedisLimiter) Allow(ctx context.Context, bucketKey string, rpm, burst int) (bool, int, time.Time) {
	start := time.Now()

	// Use provided context with timeout
	opCtx, cancel := context.WithTimeout(ctx, r.opTimeout)
	defer cancel()

	// bucketKey is already computed by middleware (tenant/api_key/jwt_sub)
	key := r.keyPrefix + bucketKey

	refillRate := float64(rpm) / 60.0 // tokens per second
	now := time.Now().Unix()

	// Check if client is available (fail-open/fail-closed behavior)
	if r.client == nil {
		gatewayotel.RateLimitRedisErrorCounter.WithLabelValues("client_nil").Inc()

		if r.failOpen {
			r.log.WarnContext(ctx, "redis client nil, failing open",
				"bucket_hash", hashKey(bucketKey),
			)
			return true, burst, time.Now().Add(60 * time.Second)
		}

		r.log.ErrorContext(ctx, "redis client nil, failing closed",
			"bucket_hash", hashKey(bucketKey),
		)
		return false, 0, time.Now().Add(60 * time.Second)
	}

	// Execute Lua script atomically (use EVAL for MVP)
	result, err := r.client.Eval(opCtx, luaScript,
		[]string{key},
		burst, refillRate, now,
	).Result()

	// Record latency metric
	latency := time.Since(start).Milliseconds()
	gatewayotel.RateLimitCheckLatency.WithLabelValues("redis").Observe(float64(latency))

	if err != nil {
		// Record error metric
		gatewayotel.RateLimitRedisErrorCounter.WithLabelValues("eval_error").Inc()

		// Handle Redis errors based on fail mode
		if r.failOpen {
			r.log.WarnContext(ctx, "redis error, failing open",
				"error", err,
				"key_hash", hashKey(key),
				"bucket_hash", hashKey(bucketKey),
			)
			return true, burst, time.Now().Add(60 * time.Second)
		}

		r.log.ErrorContext(ctx, "redis error, failing closed",
			"error", err,
			"key_hash", hashKey(key),
			"bucket_hash", hashKey(bucketKey),
		)
		return false, 0, time.Now().Add(60 * time.Second)
	}

	// Parse Lua script result: {allowed, remaining, reset_at}
	vals, ok := result.([]interface{})
	if !ok || len(vals) != 3 {
		gatewayotel.RateLimitRedisErrorCounter.WithLabelValues("parse_error").Inc()
		r.log.ErrorContext(ctx, "redis: invalid response format",
			"result", result,
			"bucket_hash", hashKey(bucketKey),
		)
		if r.failOpen {
			return true, burst, time.Now().Add(60 * time.Second)
		}
		return false, 0, time.Now().Add(60 * time.Second)
	}

	allowed := vals[0].(int64) == 1
	remaining := int(vals[1].(int64))
	resetAt := time.Unix(vals[2].(int64), 0)

	return allowed, remaining, resetAt
}

// hashKey for logging (privacy - do not log raw keys)
func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:8])
}

// HashSub hashes a JWT sub claim for use in rate-limit bucket keys.
// Only the first 8 bytes (16 hex chars) are stored — never the raw sub.
func HashSub(sub string) string {
	h := sha256.Sum256([]byte(sub))
	return hex.EncodeToString(h[:8])
}

// Close closes the Redis client connection
func (r *RedisLimiter) Close() error {
	return r.client.Close()
}
