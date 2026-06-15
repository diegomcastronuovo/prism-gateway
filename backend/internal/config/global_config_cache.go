package config

import (
	"sync"
	"time"
)

// GlobalConfigCache is a single-entry, TTL-based cache for the resolved global config.
type GlobalConfigCache struct {
	mu        sync.RWMutex
	cached    *GlobalConfig
	version   int
	fetchedAt time.Time
	ttl       time.Duration
}

// NewGlobalConfigCache creates a new cache with the given TTL.
func NewGlobalConfigCache(ttl time.Duration) *GlobalConfigCache {
	return &GlobalConfigCache{ttl: ttl}
}

// Get returns the cached GlobalConfig if it exists and hasn't expired.
// Returns (config, version, found).
func (c *GlobalConfigCache) Get() (*GlobalConfig, int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cached == nil {
		return nil, 0, false
	}
	if time.Since(c.fetchedAt) > c.ttl {
		return nil, 0, false
	}
	return c.cached, c.version, true
}

// Set stores a GlobalConfig in the cache.
func (c *GlobalConfigCache) Set(gc *GlobalConfig, version int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cached = gc
	c.version = version
	c.fetchedAt = time.Now()
}

// Invalidate removes the cached entry.
func (c *GlobalConfigCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cached = nil
}
