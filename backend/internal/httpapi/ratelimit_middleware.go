package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/ratelimit"
)

// rateLimitBucketKey computes the token-bucket key and its type for the given
// request and tenant config. This function is the single source of truth for
// scope logic — no limiter backend should recompute it.
//
// Supported scopes:
//   - "tenant" (default/empty/unknown) → "tenant:{tenantID}"
//   - "api_key"                        → "tenant:{tenantID}:api_key:{sha256hex(X-API-Key)}"
//     fallback to "tenant:{tenantID}" when the header is absent.
//   - "jwt_sub"                        → "tenant:{tenantID}:user:{hashSub(sub)}"
//     fallback to "tenant:{tenantID}" when auth_type != "jwt" or sub is empty.
//
// Raw secrets (API keys, JWT subs) are never stored or returned.
func rateLimitBucketKey(r *http.Request, tenant *config.TenantConfig) (bucketKey, keyType string) {
	if tenant.RateLimit.Scope == "api_key" {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "" {
			sum := sha256.Sum256([]byte(apiKey))
			return "tenant:" + tenant.ID + ":api_key:" + hex.EncodeToString(sum[:]), "api_key"
		}
		// Header absent — degrade to tenant-level bucket.
		return "tenant:" + tenant.ID, "tenant"
	}

	if tenant.RateLimit.Scope == "jwt_sub" {
		sub := auth.SubFromContext(r.Context())
		authType := auth.AuthTypeFromContext(r.Context())
		if authType == "jwt" && sub != "" {
			return "tenant:" + tenant.ID + ":user:" + ratelimit.HashSub(sub), "jwt_sub"
		}
		// No JWT or empty sub — degrade to tenant-level bucket.
		return "tenant:" + tenant.ID, "tenant"
	}

	// "tenant", empty, or unknown → tenant bucket.
	return "tenant:" + tenant.ID, "tenant"
}

// RateLimitMiddleware enforces per-tenant rate limits using token bucket algorithm.
func RateLimitMiddleware(cfg *config.Config, log *slog.Logger, limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			tenant := auth.TenantFromContext(ctx)

			// Skip rate limiting if tenant not resolved yet (auth middleware runs after)
			if tenant == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Skip rate limiting if rpm is not configured or <=0
			if tenant.RateLimit.RPM <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			rpm := tenant.RateLimit.RPM
			burst := tenant.RateLimit.Burst
			if burst <= 0 {
				burst = rpm // default burst = rpm if not specified
			}

			// Determine backend for metrics (check if Redis or in-memory)
			backend := "in_memory"
			if _, ok := limiter.(*ratelimit.RedisLimiter); ok {
				backend = "redis"
			}

			bucketKey, keyType := rateLimitBucketKey(r, tenant)

			start := time.Now()
			allowed, remaining, resetAt := limiter.Allow(ctx, bucketKey, rpm, burst)
			latency := time.Since(start).Milliseconds()

			// Record check latency metric
			gatewayotel.RateLimitCheckLatency.WithLabelValues(backend).Observe(float64(latency))

			// Attach rate limit info to context for adding headers later
			ctx = ratelimit.WithInfo(ctx, ratelimit.Info{
				Limit:     rpm,
				Remaining: remaining,
				ResetAt:   resetAt,
			})
			r = r.WithContext(ctx)

			if !allowed {
				// Record denial metric using the configured (or effective) scope.
				scope := tenant.RateLimit.Scope
				if scope == "" {
					scope = "tenant"
				}
				gatewayotel.RateLimitDeniedCounter.WithLabelValues(tenant.ID, backend, scope).Inc()

				// Extract span from context if available
				span := trace.SpanFromContext(ctx)
				span.AddEvent("rate_limit_block", trace.WithAttributes(
					attribute.String("tenant.id", tenant.ID),
					attribute.Int("rate_limit.rpm", rpm),
					attribute.Int("rate_limit.burst", burst),
					attribute.String("rate_limit.scope", scope),
					attribute.String("rate_limit.bucket_key_type", keyType),
				))

				logWithMode(ctx, log, LogMode(cfg.Server.LogMode), slog.LevelWarn, "rate limit exceeded",
					slog.String("tenant", tenant.ID),
					slog.String("scope", scope),
					slog.String("bucket_key_type", keyType),
					slog.Int("rpm", rpm),
					slog.Int("burst", burst),
					slog.String("path", r.URL.Path),
				)

				// Add rate limit headers on 429 response
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rpm))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

				writeError(w, http.StatusTooManyRequests, "rate limit exceeded", "rate_limited")
				return
			}

			// Add rate limit headers on successful requests
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rpm))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			next.ServeHTTP(w, r)
		})
	}
}
