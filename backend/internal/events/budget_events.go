// Package events provides fire-and-forget event emission to Redis Streams.
// All emitters are non-blocking: errors are silently dropped so the request
// path is never affected by Redis unavailability.
package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const budgetWarnStream = "budget:warn"
const emitTimeout = 100 * time.Millisecond

// BudgetWarnPayload holds the data emitted on a budget WARN event.
type BudgetWarnPayload struct {
	TenantID  string
	Level     string  // "tenant" | "tag"
	TagKey    string  // non-empty only when Level == "tag"
	TagValue  string  // non-empty only when Level == "tag"
	SpendUSD  float64
	BudgetUSD float64
	Pct       float64
	WarnPct   float64
}

// BudgetWarnEmitter emits budget WARN events. Implementations must be safe for
// concurrent use and must never block the caller.
type BudgetWarnEmitter interface {
	EmitBudgetWarn(payload BudgetWarnPayload)
}

// NoopBudgetWarnEmitter is the default when no Redis is configured.
type NoopBudgetWarnEmitter struct{}

func (NoopBudgetWarnEmitter) EmitBudgetWarn(_ BudgetWarnPayload) {}

// RedisBudgetWarnEmitter emits events to Redis Streams (XADD budget:warn *).
// Each emission is a goroutine with a short timeout — always fire-and-forget.
type RedisBudgetWarnEmitter struct {
	client *redis.Client
	log    *slog.Logger
}

// NewRedisBudgetWarnEmitter creates a Redis-backed emitter.
// addr, password, db mirror the existing Redis config fields in the project.
func NewRedisBudgetWarnEmitter(addr, password string, db int, log *slog.Logger) (*RedisBudgetWarnEmitter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	// Ping with short timeout to fail fast on startup misconfiguration.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("budget events: redis ping failed: %w", err)
	}
	return &RedisBudgetWarnEmitter{client: client, log: log}, nil
}

// EmitBudgetWarn fires a goroutine that XADDs one entry to budget:warn.
// The goroutine has a hard 100ms timeout. On any error it logs at debug and drops.
func (e *RedisBudgetWarnEmitter) EmitBudgetWarn(payload BudgetWarnPayload) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), emitTimeout)
		defer cancel()

		now := time.Now().UTC()
		args := &redis.XAddArgs{
			Stream: budgetWarnStream,
			ID:     "*", // auto-generate stream ID
			Values: map[string]interface{}{
				"event_id":   uuid.New().String(),
				"tenant_id":  payload.TenantID,
				"level":      payload.Level,
				"tag_key":    payload.TagKey,
				"tag_value":  payload.TagValue,
				"spend_usd":  fmt.Sprintf("%.6f", payload.SpendUSD),
				"budget_usd": fmt.Sprintf("%.6f", payload.BudgetUSD),
				"pct":        fmt.Sprintf("%.6f", payload.Pct),
				"warn_pct":   fmt.Sprintf("%.6f", payload.WarnPct),
				"timestamp":  now.Format(time.RFC3339),
			},
		}
		if err := e.client.XAdd(ctx, args).Err(); err != nil {
			// Debug only — Redis down or slow is expected and acceptable.
			e.log.Debug("budget events: xadd failed (dropping)", "error", err, "tenant", payload.TenantID)
		}
	}()
}

// Close releases the Redis connection pool.
func (e *RedisBudgetWarnEmitter) Close() error {
	return e.client.Close()
}
