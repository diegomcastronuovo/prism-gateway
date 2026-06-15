package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

const (
	// metricsTTL is the Redis key TTL. Each write renews it.
	metricsTTL = 7 * 24 * 60 * 60 // 7 days in seconds

	// metricsErrLogEvery throttles repeated Redis error logs.
	metricsErrLogEvery = 5 * time.Second
)

// luaEWMA atomically reads the previous EWMA, computes the new value, stores it,
// and renews the key TTL — all in a single Redis round-trip.
//
// KEYS[1] = ewma hash key  (sr:ewma:{tenant})
// ARGV[1] = model field name
// ARGV[2] = new latency sample (float, ms)
// ARGV[3] = alpha (EWMA weight for new sample)
// ARGV[4] = TTL in seconds
const luaEWMA = `
local prev = redis.call('HGET', KEYS[1], ARGV[1])
local latency = tonumber(ARGV[2])
local alpha   = tonumber(ARGV[3])
local ttl     = tonumber(ARGV[4])
local ewma
if prev then
    ewma = alpha * latency + (1 - alpha) * tonumber(prev)
else
    ewma = latency
end
redis.call('HSET',   KEYS[1], ARGV[1], tostring(ewma))
redis.call('EXPIRE', KEYS[1], ttl)
return tostring(ewma)
`

// luaIncRequest atomically increments the request counter (and error counter if
// isError=1) and renews the key TTL.
//
// KEYS[1] = counters hash key  (sr:cnt:{tenant})
// ARGV[1] = model field prefix
// ARGV[2] = is_error flag (0 or 1)
// ARGV[3] = TTL in seconds
const luaIncRequest = `
local key    = KEYS[1]
local model  = ARGV[1]
local isErr  = tonumber(ARGV[2])
local ttl    = tonumber(ARGV[3])
redis.call('HINCRBY', key, model .. ':req', 1)
if isErr == 1 then
    redis.call('HINCRBY', key, model .. ':err', 1)
end
redis.call('EXPIRE', key, ttl)
return 1
`

// RedisMetricsStore implements MetricsStore using Redis hashes.
// Key schema:
//
//	sr:ewma:{tenant_id}   HASH  field={model}          value=float (EWMA ms)
//	sr:cnt:{tenant_id}    HASH  field={model}:req       value=int
//	                            field={model}:err       value=int
type RedisMetricsStore struct {
	client    *redis.Client
	keyPrefix string
	opTimeout time.Duration
	failOpen  bool
	log       *slog.Logger

	// Rate-limited error logging to avoid log spam per request.
	errLogMu   sync.Mutex
	lastErrLog time.Time
}

// NewRedisMetricsStore creates a RedisMetricsStore and verifies connectivity.
func NewRedisMetricsStore(cfg config.RedisLimiterConfig, log *slog.Logger) (*RedisMetricsStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  time.Duration(cfg.DialTimeoutMs) * time.Millisecond,
		ReadTimeout:  time.Duration(cfg.OpTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(cfg.OpTimeoutMs) * time.Millisecond,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.DialTimeoutMs)*time.Millisecond)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "sr:"
	}

	log.Info("smart routing redis metrics store connected",
		"addr", cfg.Addr,
		"db", cfg.DB,
		"fail_open", cfg.FailOpen,
		"key_prefix", prefix,
	)

	return &RedisMetricsStore{
		client:    client,
		keyPrefix: prefix,
		opTimeout: time.Duration(cfg.OpTimeoutMs) * time.Millisecond,
		failOpen:  cfg.FailOpen,
		log:       log,
	}, nil
}

// Close releases the Redis client connection.
func (s *RedisMetricsStore) Close() error {
	return s.client.Close()
}

func (s *RedisMetricsStore) ewmaKey(tenantID string) string {
	return s.keyPrefix + "ewma:" + tenantID
}

func (s *RedisMetricsStore) cntKey(tenantID string) string {
	return s.keyPrefix + "cnt:" + tenantID
}

// logRedisError logs a Redis error at most once every metricsErrLogEvery to
// avoid flooding logs on sustained Redis unavailability.
func (s *RedisMetricsStore) logRedisError(ctx context.Context, op string, err error) {
	s.errLogMu.Lock()
	defer s.errLogMu.Unlock()
	if time.Since(s.lastErrLog) < metricsErrLogEvery {
		return
	}
	s.lastErrLog = time.Now()
	s.log.WarnContext(ctx, "smart routing redis error (degrading to default ordering)",
		"op", op, "error", err)
}

func (s *RedisMetricsStore) UpdateLatencyEWMA(ctx context.Context, tenantID, model string, latencyMs float64) error {
	opCtx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()

	_, err := s.client.Eval(opCtx, luaEWMA,
		[]string{s.ewmaKey(tenantID)},
		model,
		strconv.FormatFloat(latencyMs, 'f', 6, 64),
		strconv.FormatFloat(ewmaAlpha, 'f', 6, 64),
		metricsTTL,
	).Result()
	if err != nil {
		s.logRedisError(ctx, "UpdateLatencyEWMA", err)
		// Always degrade gracefully — do not propagate errors to callers.
		return nil
	}
	return nil
}

func (s *RedisMetricsStore) IncRequest(ctx context.Context, tenantID, model string, isError bool) error {
	opCtx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()

	isErrVal := 0
	if isError {
		isErrVal = 1
	}

	_, err := s.client.Eval(opCtx, luaIncRequest,
		[]string{s.cntKey(tenantID)},
		model,
		isErrVal,
		metricsTTL,
	).Result()
	if err != nil {
		s.logRedisError(ctx, "IncRequest", err)
		return nil
	}
	return nil
}

func (s *RedisMetricsStore) GetLatencyEWMA(ctx context.Context, tenantID string, models []string) (map[string]float64, error) {
	if len(models) == 0 {
		return map[string]float64{}, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()

	key := s.ewmaKey(tenantID)
	pipe := s.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(models))
	for i, model := range models {
		cmds[i] = pipe.HGet(opCtx, key, model)
	}
	_, _ = pipe.Exec(opCtx) // individual cmd errors checked below

	result := make(map[string]float64, len(models))
	var pipelineErr error
	for i, cmd := range cmds {
		val, err := cmd.Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				pipelineErr = err
			}
			continue
		}
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			continue
		}
		result[models[i]] = f
	}
	if pipelineErr != nil {
		s.logRedisError(ctx, "GetLatencyEWMA", pipelineErr)
	}
	return result, nil
}

func (s *RedisMetricsStore) GetErrorStats(ctx context.Context, tenantID string, models []string) (map[string]ErrorStats, error) {
	if len(models) == 0 {
		return map[string]ErrorStats{}, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()

	key := s.cntKey(tenantID)
	pipe := s.client.Pipeline()
	reqCmds := make([]*redis.StringCmd, len(models))
	errCmds := make([]*redis.StringCmd, len(models))
	for i, model := range models {
		reqCmds[i] = pipe.HGet(opCtx, key, model+":req")
		errCmds[i] = pipe.HGet(opCtx, key, model+":err")
	}
	_, _ = pipe.Exec(opCtx)

	result := make(map[string]ErrorStats, len(models))
	var pipelineErr error
	for i, model := range models {
		reqVal, err := reqCmds[i].Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				pipelineErr = err
			}
			continue
		}
		req, err := strconv.Atoi(reqVal)
		if err != nil || req == 0 {
			continue
		}
		errVal, _ := errCmds[i].Result()
		errCount, _ := strconv.Atoi(errVal)

		errorRate := float64(errCount) / float64(req)
		result[model] = ErrorStats{
			RequestCount: req,
			ErrorCount:   errCount,
			ErrorRate:    errorRate,
		}
	}
	if pipelineErr != nil {
		s.logRedisError(ctx, "GetErrorStats", pipelineErr)
	}
	return result, nil
}
