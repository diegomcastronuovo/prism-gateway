package router

import (
	"context"
	"sync"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// ModelStatsCache caches DB-based model statistics with TTL.
type ModelStatsCache struct {
	store      storage.Storage
	mu         sync.RWMutex
	cache      map[string]*cacheEntry
	ttl        time.Duration
	windowDays int
}

type cacheEntry struct {
	stats      map[string]*AggregatedStats // model -> stats
	expiration time.Time
}

// AggregatedStats holds aggregated statistics for a model.
type AggregatedStats struct {
	ErrorRate    float64
	AvgLatencyMs float64
	RequestCount int
}

// NewModelStatsCache creates a cache with specified TTL and window.
func NewModelStatsCache(store storage.Storage, ttl time.Duration, windowDays int) *ModelStatsCache {
	return &ModelStatsCache{
		store:      store,
		cache:      make(map[string]*cacheEntry),
		ttl:        ttl,
		windowDays: windowDays,
	}
}

// GetStats retrieves aggregated stats for a tenant, using cache when valid.
func (c *ModelStatsCache) GetStats(ctx context.Context, tenantID string) (map[string]*AggregatedStats, error) {
	c.mu.RLock()
	entry, exists := c.cache[tenantID]
	if exists && time.Now().Before(entry.expiration) {
		c.mu.RUnlock()
		return entry.stats, nil
	}
	c.mu.RUnlock()

	// Cache miss or expired, fetch from DB
	dbStats, err := c.store.GetModelStats(ctx, tenantID, c.windowDays)
	if err != nil {
		return nil, err
	}

	// Aggregate stats by model
	aggregated := make(map[string]*AggregatedStats)
	for _, stat := range dbStats {
		agg, exists := aggregated[stat.Model]
		if !exists {
			agg = &AggregatedStats{}
			aggregated[stat.Model] = agg
		}

		agg.RequestCount += stat.RequestCount
		// Weighted average latency
		if stat.SuccessCount > 0 {
			totalLatency := agg.AvgLatencyMs * float64(agg.RequestCount-stat.RequestCount)
			totalLatency += stat.AvgLatencyMs * float64(stat.SuccessCount)
			agg.AvgLatencyMs = totalLatency / float64(agg.RequestCount)
		}
	}

	// Calculate error rates
	errorCounts := make(map[string]int)
	for _, stat := range dbStats {
		errorCounts[stat.Model] += stat.ErrorCount
	}

	for model, agg := range aggregated {
		if agg.RequestCount > 0 {
			agg.ErrorRate = float64(errorCounts[model]) / float64(agg.RequestCount)
		}
	}

	// Update cache
	c.mu.Lock()
	c.cache[tenantID] = &cacheEntry{
		stats:      aggregated,
		expiration: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	return aggregated, nil
}

// Invalidate removes a tenant's cache entry.
func (c *ModelStatsCache) Invalidate(tenantID string) {
	c.mu.Lock()
	delete(c.cache, tenantID)
	c.mu.Unlock()
}

// Clear removes all cache entries.
func (c *ModelStatsCache) Clear() {
	c.mu.Lock()
	c.cache = make(map[string]*cacheEntry)
	c.mu.Unlock()
}
