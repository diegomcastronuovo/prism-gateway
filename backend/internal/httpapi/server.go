package httpapi

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/circuitbreaker"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/events"
	"github.com/diegomcastronuovo/prism-gateway/internal/hooks"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/ratelimit"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// Server wraps http.Server and adds cleanup hooks.
type Server struct {
	*http.Server
	handlers *Handlers
}

// GlobalConfigCache returns the global config cache so callers (main.go) can share it
// with other components (e.g. the benchmarking scheduler).
func (s *Server) GlobalConfigCache() *config.GlobalConfigCache {
	return s.handlers.globalCfgCache
}

// Close gracefully shuts down handlers before the HTTP server.
func (s *Server) Close() error {
	if s.handlers != nil {
		s.handlers.Close()
	}
	return nil
}

// NewServer creates and configures the HTTP server with all routes.
func NewServer(cfg *config.Config, log *slog.Logger, rt *router.Router, reg *providers.Registry, hookReg *hooks.Registry, store storage.Storage, limiter ratelimit.Limiter, breaker circuitbreaker.Breaker) *Server {
	h := NewHandlers(cfg, log, rt, reg, hookReg, store, breaker)

	// Initialize tenant config cache using configured TTL
	cacheTTL := time.Duration(cfg.DynamicConfig.CacheTTLms) * time.Millisecond
	tenantCache := config.NewTenantConfigCache(cacheTTL)
	h.tenantCache = tenantCache

	// Initialize global config cache (same TTL)
	h.globalCfgCache = config.NewGlobalConfigCache(cacheTTL)

	// Wire the Orchestrator after caches are initialised.
	h.orchestrator = &Orchestrator{h: h}

	// Initialize budget WARN event emitter (Redis Streams).
	// Falls back to noop when not configured: zero risk to the request path.
	initBudgetEmitter(h, cfg, log)

	// Shared JWKS-backed validators for inference (/v1/*) and admin (/admin/*).
	jwtCache := auth.NewJWTValidatorCache(log)
	if cfg.Auth.Mode == "jwt" || cfg.Auth.Mode == "both" {
		log.Info("jwt authentication enabled (jwks from active global config)",
			"mode", cfg.Auth.Mode,
		)
	}

	// Middleware factories
	authMW := auth.Middleware(cfg, log, tenantCache, store, h.globalCfgCache, jwtCache)
	rateLimitMW := RateLimitMiddleware(cfg, log, limiter)
	userRBACMW := auth.RBACMiddleware(cfg, log, cfg.Auth.RBAC.UserRoles)
	tenantIsolationMW := auth.TenantIsolationMiddleware(cfg, log)

	inferenceScope := auth.ScopeMiddleware([]string{"inference"}, log)

	maxBodyMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 16*1024*1024) // 16 MB
			next.ServeHTTP(w, r)
		})
	}

	mux := http.NewServeMux()

	// Public endpoints
	metricsHandler := http.Handler(promhttp.Handler())
	if metricsToken := os.Getenv("METRICS_TOKEN"); metricsToken != "" {
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(metricsToken)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			promhttp.Handler().ServeHTTP(w, r)
		})
	}
	mux.Handle("GET /metrics", metricsHandler)

	// Bootstrap endpoint: create first admin API key (only works if api_keys table is empty)
	mux.Handle("POST /admin/bootstrap/api-keys",
		http.HandlerFunc(BootstrapAPIKeyHandler(store, log)))

	// /v1/* endpoints: auth → inference scope → user RBAC → rate limit → handler
	mux.Handle("GET /v1/models",
		authMW(inferenceScope(userRBACMW(rateLimitMW(http.HandlerFunc(h.ListModels))))))
	mux.Handle("POST /v1/chat/completions",
		authMW(inferenceScope(userRBACMW(rateLimitMW(maxBodyMW(http.HandlerFunc(h.ChatCompletions)))))))
	mux.Handle("POST /v1/responses",
		authMW(inferenceScope(userRBACMW(rateLimitMW(maxBodyMW(http.HandlerFunc(h.ResponsesAPI)))))))
	mux.Handle("POST /v1/messages",
		authMW(inferenceScope(userRBACMW(rateLimitMW(maxBodyMW(http.HandlerFunc(h.AnthropicMessagesRouted)))))))
	mux.Handle("POST /v1/models/",
		authMW(inferenceScope(userRBACMW(rateLimitMW(maxBodyMW(http.HandlerFunc(h.GeminiGenerateContent)))))))
	mux.Handle("POST /v1/embeddings",
		authMW(inferenceScope(userRBACMW(rateLimitMW(maxBodyMW(http.HandlerFunc(h.Embeddings)))))))
	mux.Handle("POST /v1/claudecode",
		authMW(inferenceScope(userRBACMW(rateLimitMW(maxBodyMW(http.HandlerFunc(h.AnthropicMessages)))))))

	// /admin/* middleware
	adminAuthMW := AdminMiddleware(cfg, tenantCache, h.store, h.globalCfgCache, jwtCache, log)
	adminScopeMW := AdminScopeMiddleware(log)
	adminTenantMW := AdminTenantIsolationMiddleware(log)
	logsReadMW := RequireLogsReadAccessMiddleware(log)
	adminOnlyMW := AdminOnlyMiddleware(log)

	// Semantic anchor management
	mux.Handle("POST /v1/semantic/anchors",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.CreateSemanticAnchor)))))
	mux.Handle("POST /v1/semantic/anchors/similarity-test",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.SemanticSimilarityTest)))))
	mux.Handle("POST /admin/semantic/anchors/calibrate",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.CalibrateSemanticThreshold)))))
	mux.Handle("POST /admin/semantic/anchors/suggest",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.SuggestSemanticAnchors)))))
	mux.Handle("POST /admin/semantic/test",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminSemanticTest)))))
	mux.Handle("GET /v1/semantic/anchors",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.ListSemanticAnchors)))))
	mux.Handle("PATCH /v1/semantic/anchors/{name}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.PatchSemanticAnchor)))))
	mux.Handle("DELETE /v1/semantic/anchors/{name}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.DeleteSemanticAnchor)))))

	// Semantic route management
	mux.Handle("POST /admin/semantic/routes",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.CreateSemanticRoute)))))
	mux.Handle("GET /admin/semantic/routes",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.ListSemanticRoutes)))))
	mux.Handle("PATCH /admin/semantic/routes/{name}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.PatchSemanticRoute)))))
	mux.Handle("DELETE /admin/semantic/routes/{name}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.DeleteSemanticRoute)))))

	// Budget and usage observability
	mux.Handle("GET /admin/tenants/{tenant_id}/usage/summary",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminUsageSummary))))))
	mux.Handle("GET /admin/tenants/{tenant_id}/budget/forecast",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminBudgetForecast))))))
	mux.Handle("GET /admin/tenants/{tenant_id}/models/stats",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminModelStats))))))
	mux.Handle("GET /admin/tenants/{tenant_id}/smart/impact",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminSmartImpact))))))
	mux.Handle("GET /admin/tenants/{tenant_id}/audit/export",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(logsReadMW(http.HandlerFunc(h.AdminAuditExport)))))))
	mux.Handle("GET /admin/tenants/{tenant_id}/billing/export",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminBillingExport))))))
	mux.Handle("GET /admin/billing/report.csv",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminBillingReportCSV)))))
	mux.Handle("GET /admin/tenants/{tenant_id}/alerts",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminBudgetAlerts))))))
	mux.Handle("GET /admin/tenants/{tenant_id}/usage/by-tag",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminUsageByTag))))))
	mux.Handle("GET /admin/tenants/{tenant_id}/budgets/status",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminBudgetStatus))))))

	// Semantic threshold config
	mux.Handle("PATCH /admin/tenants/{tenant_id}/semantic-threshold",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminPatchSemanticThreshold))))))

	// Global config management
	mux.Handle("GET /admin/config/global",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminGetGlobalConfig)))))
	mux.Handle("PUT /admin/config/global",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminPutGlobalConfig)))))
	mux.Handle("PATCH /admin/config/global",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminPatchGlobalConfig)))))
	mux.Handle("POST /admin/config/global/apply",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminApplyGlobalConfigVersion)))))

	// Model management
	mux.Handle("PATCH /admin/models/{model_name}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminPatchModel)))))
	mux.Handle("GET /admin/models/{model_id}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminGetModel)))))
	mux.Handle("POST /admin/models",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminCreateModel)))))
	mux.Handle("DELETE /admin/models/{model_id}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminDeleteModel)))))

	// Model catalog
	mux.Handle("GET /admin/model-catalog",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListModelCatalog)))))
	mux.Handle("POST /admin/model-catalog",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminCreateModelCatalogEntry)))))
	mux.Handle("PUT /admin/model-catalog/{provider}/{id}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminUpdateModelCatalogEntry)))))
	mux.Handle("DELETE /admin/model-catalog/{provider}/{id}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminDeleteModelCatalogEntry)))))

	// Tool catalog
	mux.Handle("GET /admin/tool-catalog",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListToolCatalog)))))
	mux.Handle("POST /admin/tool-catalog",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminCreateToolCatalogEntry)))))
	mux.Handle("PUT /admin/tool-catalog/{provider}/{id}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminUpdateToolCatalogEntry)))))
	mux.Handle("DELETE /admin/tool-catalog/{provider}/{id}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminDeleteToolCatalogEntry)))))

	// Tenant config management
	mux.Handle("GET /admin/tenants/{tenant_id}/config",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminGetTenantConfig)))))
	mux.Handle("PUT /admin/tenants/{tenant_id}/config",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminPutTenantConfig)))))
	mux.Handle("PATCH /admin/tenants/{tenant_id}/config",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminPatchTenantConfig)))))
	mux.Handle("GET /admin/tenants/{tenant_id}/config/changes",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListConfigChanges)))))
	mux.Handle("GET /admin/config/history",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminConfigHistory)))))
	mux.Handle("GET /admin/config/versions",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminConfigVersions)))))
	mux.Handle("GET /admin/config/diff",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminConfigDiff)))))

	// Connection tests
	mux.Handle("POST /admin/test-connection",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminTestConnection)))))
	mux.Handle("POST /admin/tenants/{tenant_id}/pii/test-connection",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminPIITestConnection)))))

	// API key management
	mux.Handle("POST /admin/tenants/{tenant_id}/api-keys",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminCreateAPIKey))))))
	mux.Handle("GET /admin/tenants/{tenant_id}/api-keys",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminListAPIKeys))))))
	mux.Handle("POST /admin/tenants/{tenant_id}/api-keys/{id}/revoke",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminRevokeAPIKey))))))
	mux.Handle("POST /admin/tenants/{tenant_id}/api-keys/{id}/rotate",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminRotateAPIKey))))))

	// Observability
	mux.Handle("GET /admin/tenants/{tenant_id}/usage",
		adminAuthMW(adminScopeMW(adminTenantMW(tenantIsolationMW(http.HandlerFunc(h.AdminTenantUsage))))))
	mux.Handle("GET /admin/observability/semantic-cache",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminSemanticCacheStats)))))
	mux.Handle("GET /admin/observability/semantic-routing",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminSemanticRoutingStats)))))
	mux.Handle("GET /admin/observability/semantic-correlation",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminSemanticCorrelation)))))
	mux.Handle("GET /admin/routing/stats",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminRoutingStats)))))
	mux.Handle("GET /admin/requests/recent",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminRequestsRecent)))))
	mux.Handle("GET /admin/requests/stats",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminRequestsStats)))))
	mux.Handle("GET /admin/requests",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListRequests)))))
	mux.Handle("GET /admin/audit/requests",
		adminAuthMW(adminScopeMW(adminTenantMW(logsReadMW(http.HandlerFunc(h.AdminAuditRequests))))))
	mux.Handle("GET /admin/router/performance",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminRouterPerformance)))))

	// Audit export CSV (admin only)
	mux.Handle("GET /admin/audit/requests/export.csv",
		adminAuthMW(adminScopeMW(adminTenantMW(adminOnlyMW(http.HandlerFunc(h.AdminAuditRequests))))))

	// Tenant / model / provider catalog
	mux.Handle("GET /admin/tenants",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListTenants)))))
	mux.Handle("POST /admin/tenants",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminCreateTenant)))))
	mux.Handle("DELETE /admin/tenants/{tenant_id}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminDeleteTenant)))))
	mux.Handle("GET /admin/models",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListModels)))))
	mux.Handle("GET /admin/providers",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListProviders)))))
	mux.Handle("GET /admin/route-groups",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListRouteGroups)))))
	mux.Handle("DELETE /admin/tenants/{tenant_id}/route-groups/{name}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminDeleteRouteGroup)))))
	mux.Handle("GET /admin/features",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListFeatures)))))

	// API key + JWT sub attribution (usage observability)
	mux.Handle("GET /admin/api-keys/usage",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminAPIKeysUsage)))))
	mux.Handle("GET /admin/api-keys/{api_key_id}/usage",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminAPIKeyUsageDetail)))))
	mux.Handle("GET /admin/api-keys/requests",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminAPIKeysRequests)))))
	mux.Handle("GET /admin/jwt-subs/usage",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminJWTSubsUsage)))))
	mux.Handle("GET /admin/jwt-subs/{jwt_sub}/usage",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminJWTSubUsageDetail)))))
	mux.Handle("GET /admin/jwt-subs/requests",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminJWTSubsRequests)))))

	// Version, benchmarks, health
	mux.Handle("GET /admin/version",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminGetVersion)))))
	mux.Handle("GET /admin/benchmarks/models",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminListModelBenchmarks)))))
	mux.Handle("DELETE /admin/benchmarks/models",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminDeleteModelBenchmarks)))))
	mux.Handle("GET /admin/health/system",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminSystemHealth)))))

	// Routing snapshots and replay
	mux.Handle("GET /admin/requests/{request_id}/routing",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.GetRoutingSnapshot)))))
	mux.Handle("POST /admin/replay/{request_id}",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.ReplayRequest)))))
	mux.Handle("POST /admin/traffic/replay",
		adminAuthMW(adminScopeMW(adminTenantMW(http.HandlerFunc(h.AdminTrafficReplay)))))

	topMux := http.NewServeMux()
	topMux.HandleFunc("GET /healthz", h.Healthz)
	topMux.Handle("/", mux)

	return &Server{
		Server: &http.Server{
			Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
			Handler:           gatewayotel.PropagationMiddleware(topMux),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       time.Duration(cfg.Server.RequestTimeoutMs) * time.Millisecond,
			WriteTimeout:      0,
			IdleTimeout:       120 * time.Second,
		},
		handlers: h,
	}
}

// initBudgetEmitter initializes the budget WARN Redis Streams emitter from config.
// Falls back to noop when not configured: zero risk to the request path.
func initBudgetEmitter(h *Handlers, cfg *config.Config, log *slog.Logger) {
	for _, t := range cfg.Tenants {
		addr := t.BudgetEnforcement.Events.Redis.Addr
		if addr == "" {
			continue
		}
		emitter, err := events.NewRedisBudgetWarnEmitter(
			addr,
			t.BudgetEnforcement.Events.Redis.Password,
			t.BudgetEnforcement.Events.Redis.DB,
			log,
		)
		if err != nil {
			log.Warn("budget events: redis connect failed, using noop", "error", err, "addr", addr)
			return
		}
		h.budgetEmitter = emitter
		log.Info("budget events: emitter initialized", "addr", addr, "stream", "budget:warn")
		return
	}
}
