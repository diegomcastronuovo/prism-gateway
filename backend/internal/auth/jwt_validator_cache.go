package auth

import (
	"log/slog"
	"sync"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// JWTValidatorCache caches JWT validators by (issuer, audience, jwks_url) so the data plane
// and admin middleware use the same validators for a given active global auth.jwt block.
type JWTValidatorCache struct {
	mu  sync.RWMutex
	m   map[string]*JWTValidator
	log *slog.Logger
}

// NewJWTValidatorCache creates an empty cache. Pass the same instance to inference Middleware
// and AdminMiddleware so JWKS updates share validator instances.
func NewJWTValidatorCache(log *slog.Logger) *JWTValidatorCache {
	return &JWTValidatorCache{log: log}
}

// GetOrCreate returns a validator for the given JWT config (typically gc.Auth.JWT).
func (c *JWTValidatorCache) GetOrCreate(jwtCfg config.JWTConfig) *JWTValidator {
	key := jwtCfg.Issuer + "|" + jwtCfg.Audience + "|" + jwtCfg.JWKSURL
	c.mu.RLock()
	if v, ok := c.m[key]; ok {
		c.mu.RUnlock()
		return v
	}
	c.mu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.m[key]; ok {
		return v
	}
	v := NewJWTValidator(jwtCfg, c.log)
	if c.m == nil {
		c.m = make(map[string]*JWTValidator)
	}
	c.m[key] = v
	return v
}
