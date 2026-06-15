package otel

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RateLimitDeniedCounter tracks total rate limit denials
	RateLimitDeniedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_denied_total",
			Help: "Total number of rate limit denials",
		},
		[]string{"tenant_id", "backend", "scope"},
	)

	// RateLimitCheckLatency tracks rate limit check latency
	RateLimitCheckLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rate_limit_check_latency_ms",
			Help:    "Rate limit check latency in milliseconds",
			Buckets: []float64{1, 2, 5, 10, 20, 50, 100},
		},
		[]string{"backend"},
	)

	// RateLimitRedisErrorCounter tracks Redis errors
	RateLimitRedisErrorCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_redis_errors_total",
			Help: "Total number of Redis errors during rate limiting",
		},
		[]string{"error_type"},
	)

	// APIKeyValidationsCounter tracks API key validation attempts
	APIKeyValidationsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_key_validations_total",
			Help: "Total API key validation attempts",
		},
		[]string{"result"}, // ok, revoked, expired, not_found, error
	)

	// APIKeyAdminActionsCounter tracks admin API key operations
	APIKeyAdminActionsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_key_admin_actions_total",
			Help: "Total admin API key operations",
		},
		[]string{"action"}, // create, revoke, rotate
	)

	// CBDeniedCounter tracks circuit breaker denied requests
	CBDeniedCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "cb_denied_total", Help: "Circuit breaker denied requests"},
		[]string{"provider", "state"},
	)

	// CBTransitionsCounter tracks circuit breaker state transitions
	CBTransitionsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "cb_transitions_total", Help: "Circuit breaker state transitions"},
		[]string{"provider", "from", "to", "reason"},
	)

	// CBRedisErrorCounter tracks circuit breaker Redis errors
	CBRedisErrorCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "cb_redis_errors_total", Help: "Circuit breaker Redis errors"},
		[]string{"type"},
	)

	// SemanticSimilarityTestCounter tracks semantic anchor similarity test requests
	SemanticSimilarityTestCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "semantic_similarity_test_total",
			Help: "Total semantic anchor similarity test requests",
		},
		[]string{"matched"}, // "true" | "false" | "no_anchors"
	)

	// SemanticCacheLookupCounter tracks total semantic cache lookups
	SemanticCacheLookupCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "semantic_cache_lookup_total", Help: "Total semantic cache lookups"},
		[]string{"tenant_id"},
	)

	// SemanticCacheHitCounter tracks total semantic cache hits
	SemanticCacheHitCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "semantic_cache_hit_total", Help: "Total semantic cache hits"},
		[]string{"tenant_id"},
	)

	// SemanticCacheMissCounter tracks total semantic cache misses
	SemanticCacheMissCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "semantic_cache_miss_total", Help: "Total semantic cache misses"},
		[]string{"tenant_id"},
	)

	// SemanticCacheWriteCounter tracks total semantic cache writes
	SemanticCacheWriteCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "semantic_cache_write_total", Help: "Total semantic cache writes"},
		[]string{"tenant_id"},
	)

	// SemanticCacheSimilarityHist tracks similarity scores on cache hits
	SemanticCacheSimilarityHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "semantic_cache_similarity",
			Help:    "Similarity score on cache hits",
			Buckets: []float64{0.80, 0.85, 0.90, 0.92, 0.95, 0.97, 0.99, 1.0},
		},
		[]string{"tenant_id"},
	)

	// TrafficReplayTotalCounter tracks total traffic replay executions
	TrafficReplayTotalCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "traffic_replay_total", Help: "Total traffic replay executions"},
		[]string{"tenant_id"},
	)

	// TrafficReplayChangedRoutesCounter tracks changed route_groups detected during replay
	TrafficReplayChangedRoutesCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "traffic_replay_changed_routes", Help: "Changed route_groups detected during replay"},
		[]string{"tenant_id"},
	)

	// TrafficReplayChangedModelsCounter tracks changed model selections detected during replay
	TrafficReplayChangedModelsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "traffic_replay_changed_models", Help: "Changed model selections detected during replay"},
		[]string{"tenant_id"},
	)

	// TrafficReplayCostDeltaHist tracks cost delta (USD) per replay execution
	TrafficReplayCostDeltaHist = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "traffic_replay_cost_delta_usd",
			Help:    "Cost delta (USD) per replay execution",
			Buckets: []float64{-100, -10, -1, -0.1, 0, 0.1, 1, 10, 100},
		},
		[]string{"tenant_id"},
	)

	// BudgetCheckCounter tracks budget enforcement check results
	BudgetCheckCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "budget_check_total", Help: "Budget enforcement check results"},
		[]string{"tenant_id", "result", "scope"}, // result: ok|warn|hard|error; scope: tenant|tag
	)

	// BudgetBlockCounter tracks requests blocked by budget enforcement
	BudgetBlockCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "budget_block_total", Help: "Requests blocked by budget enforcement"},
		[]string{"tenant_id", "scope"},
	)

	// BudgetDegradeCounter tracks requests degraded by budget enforcement
	BudgetDegradeCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{Name: "budget_degrade_total", Help: "Requests degraded by budget enforcement"},
		[]string{"tenant_id", "scope"},
	)

	// --- Gateway request & monetization (SPEC_110 Prometheus) ---

	// RequestsTotal counts one increment per user-visible gateway outcome (LLM, embedding, ML).
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "requests_total",
			Help: "Total processed gateway requests (LLM, embedding, ML) by outcome",
		},
		[]string{"tenant_id", "model", "provider", "model_type", "auth_type", "status"},
	)

	// RequestLatencyMs is end-to-end latency per gateway request.
	RequestLatencyMs = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_latency_ms",
			Help:    "End-to-end gateway request latency in milliseconds",
			Buckets: []float64{25, 50, 100, 200, 400, 800, 1500, 3000, 6000, 12000, 30000, 120000},
		},
		[]string{"tenant_id", "model", "provider", "model_type"},
	)

	// RequestCostEffectiveUSD is effective cost per request (token + infra allocation).
	RequestCostEffectiveUSD = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_cost_effective_usd",
			Help:    "Effective cost per request in USD (token spend plus infra allocation)",
			Buckets: []float64{0, 1e-6, 5e-6, 1e-5, 5e-5, 1e-4, 5e-4, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 25, 100},
		},
		[]string{"tenant_id", "model", "provider", "model_type"},
	)

	// RequestPriceUSD is monetized price per request after markup.
	RequestPriceUSD = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_price_usd",
			Help:    "Monetized price per request in USD after markup",
			Buckets: []float64{0, 1e-6, 5e-6, 1e-5, 5e-5, 1e-4, 5e-4, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 25, 100},
		},
		[]string{"tenant_id", "model", "provider", "model_type"},
	)

	// RequestMarginUSD is margin (price minus effective cost) per request.
	RequestMarginUSD = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_margin_usd",
			Help:    "Margin per request in USD (price minus effective cost)",
			Buckets: []float64{0, 1e-7, 1e-6, 5e-6, 1e-5, 5e-5, 1e-4, 5e-4, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 25},
		},
		[]string{"tenant_id", "model", "provider", "model_type"},
	)

	// UpstreamLatencyMs measures time-to-first-byte per upstream provider call.
	// Use to detect per-provider degradation and inform circuit breaker tuning.
	UpstreamLatencyMs = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "upstream_latency_ms",
			Help:    "Time from sending request to first byte received from upstream provider, in milliseconds",
			Buckets: []float64{50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000},
		},
		[]string{"provider", "model", "status"},
	)

	// EffectiveSpendTotalUSD accumulates effective cost in USD.
	EffectiveSpendTotalUSD = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "effective_spend_total_usd",
			Help: "Accumulated effective cost in USD",
		},
		[]string{"tenant_id", "model"},
	)

	// TotalPriceUSD accumulates monetized price in USD.
	TotalPriceUSD = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "total_price_usd",
			Help: "Accumulated monetized price in USD",
		},
		[]string{"tenant_id", "model"},
	)

	// TotalMarginUSD accumulates margin in USD.
	TotalMarginUSD = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "total_margin_usd",
			Help: "Accumulated margin in USD (price minus effective cost)",
		},
		[]string{"tenant_id", "model"},
	)

	// MLRequestsTotal counts ML pipeline requests by status.
	MLRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ml_requests_total",
			Help: "Total ML execution requests",
		},
		[]string{"tenant_id", "model", "provider", "status"},
	)

	// MLUpstreamErrorsTotal counts upstream ML endpoint failures.
	MLUpstreamErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ml_upstream_errors_total",
			Help: "Upstream ML execution failures",
		},
		[]string{"model", "provider", "error_type"},
	)

	// MLObservableFieldsLoggedTotal counts observable field extractions logged.
	MLObservableFieldsLoggedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ml_observable_fields_logged_total",
			Help: "Observable fields captured/logged from ML requests and responses",
		},
		[]string{"model"},
	)

	// BillingReportExportsTotal counts billing CSV export requests.
	BillingReportExportsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "billing_report_exports_total",
			Help: "Billing report CSV export requests",
		},
		[]string{"result", "auth_type"},
	)

	// BillingReportRowsTotal counts data rows emitted in billing CSV exports.
	BillingReportRowsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "billing_report_rows_total",
			Help: "Rows emitted in billing CSV exports by identity type",
		},
		[]string{"identity_type"},
	)

	// MarkupAppliedRequestsTotal counts requests where model markup was applied.
	MarkupAppliedRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "markup_applied_requests_total",
			Help: "Requests where model markup_percentage was greater than zero",
		},
		[]string{"model"},
	)

	// RequestTokensTotal counts prompt and completion tokens (LLM / embeddings as applicable).
	RequestTokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "request_tokens_total",
			Help: "Token usage by type",
		},
		[]string{"tenant_id", "model", "token_type"},
	)

	// RequestErrorsTotal counts gateway-level errors after routing.
	RequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "request_errors_total",
			Help: "Gateway request errors by classifier",
		},
		[]string{"tenant_id", "model", "provider", "error_type"},
	)

	// AsyncDropTotal counts fire-and-forget goroutines dropped due to semaphore capacity.
	AsyncDropTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "async_drop_total",
			Help: "Fire-and-forget goroutines dropped due to semaphore capacity",
		},
		[]string{"operation"},
	)

	// StatsDispatcherDropTotal counts stat updates dropped due to full queue.
	StatsDispatcherDropTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "stats_dispatcher_drop_total",
			Help: "Total stat updates dropped because the queue was full",
		},
	)

	// BudgetCheckFailTotal counts requests that bypassed budget enforcement due to a DB error.
	BudgetCheckFailTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "budget_check_fail_total",
			Help: "Requests that bypassed budget enforcement due to a database error (fail-open)",
		},
		[]string{"tenant_id"},
	)

	// ConversationLogDroppedTotal counts conversation log rows silently dropped because encryption is not configured.
	ConversationLogDroppedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "conversation_log_dropped_total",
			Help: "Conversation log rows dropped because field encryption (LOG_ENC_KEY_V1) is not configured",
		},
	)
)
