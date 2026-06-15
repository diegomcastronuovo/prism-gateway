package config

import (
	"sync"
	"time"
)

type cachedConfig struct {
	config    *TenantConfig
	version   int
	fetchedAt time.Time
}

// TenantConfigCache provides thread-safe caching of tenant configurations with TTL
type TenantConfigCache struct {
	mu    sync.RWMutex
	cache map[string]*cachedConfig
	ttl   time.Duration
}

// NewTenantConfigCache creates a new cache with the specified TTL
func NewTenantConfigCache(ttl time.Duration) *TenantConfigCache {
	return &TenantConfigCache{
		cache: make(map[string]*cachedConfig),
		ttl:   ttl,
	}
}

// Get retrieves a tenant config from cache if it exists and hasn't expired
// Returns (config, version, found)
func (c *TenantConfigCache) Get(tenantID string) (*TenantConfig, int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, ok := c.cache[tenantID]
	if !ok {
		return nil, 0, false
	}

	// Check TTL
	if time.Since(cached.fetchedAt) > c.ttl {
		return nil, 0, false
	}

	return cached.config, cached.version, true
}

// Set stores a tenant config in the cache
func (c *TenantConfigCache) Set(tenantID string, config *TenantConfig, version int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[tenantID] = &cachedConfig{
		config:    config,
		version:   version,
		fetchedAt: time.Now(),
	}
}

// Invalidate removes a tenant config from the cache
func (c *TenantConfigCache) Invalidate(tenantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, tenantID)
}

// Clear removes all entries from the cache
func (c *TenantConfigCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*cachedConfig)
}
