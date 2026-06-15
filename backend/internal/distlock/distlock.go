package distlock

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// TryAcquire attempts to acquire a distributed lock using SET NX EX.
// Returns true if the lock was acquired, false if another pod holds it.
// On Redis error, returns true (fail-open) — better to run twice than never.
func TryAcquire(ctx context.Context, client *redis.Client, key string, ttl time.Duration) bool {
	ok, err := client.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		// fail-open: Redis unavailable → run anyway
		return true
	}
	return ok
}
