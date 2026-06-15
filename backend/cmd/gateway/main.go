package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

	"github.com/diegomcastronuovo/prism-gateway/internal/benchmarking"
	"github.com/diegomcastronuovo/prism-gateway/internal/circuitbreaker"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/hooks"
	"github.com/diegomcastronuovo/prism-gateway/internal/httpapi"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/ratelimit"
	"github.com/diegomcastronuovo/prism-gateway/internal/router"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	log.Info("config loaded",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
		"tenants", len(cfg.Tenants),
		"models", len(cfg.Models),
		"log_mode", cfg.Server.LogMode,
		"config_version", cfg.Version,
		"config_sha256", cfg.SHA256,
	)

	if cfg.Server.LogMode == "full" {
		log.Warn("SECURITY WARNING: log_mode='full' enabled - logs may contain PII and sensitive data")
	}

	ctx := context.Background()
	shutdownTracer, err := gatewayotel.Setup(ctx, log)
	if err != nil {
		log.Error("failed to initialize otel", "error", err)
		os.Exit(1)
	}
	defer shutdownTracer(ctx)

	var store storage.Storage
	dsn := os.Getenv("DATABASE_URL")
	if dsn != "" {
		pg, err := storage.NewPostgres(ctx, dsn, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, log)
		if err != nil {
			log.Error("failed to connect to database", "error", err)
			os.Exit(1)
		}
		defer pg.Close()
		store = pg
		log.Info("database connected", "max_open", cfg.Database.MaxOpenConns, "max_idle", cfg.Database.MaxIdleConns)

		if err := httpapi.BootstrapTenantConfigs(ctx, cfg, store, log); err != nil {
			log.Error("failed to bootstrap tenant configs", "error", err)
			os.Exit(1)
		}

		if cfg.DynamicConfig.Enabled {
			if err := httpapi.BootstrapGlobalConfig(ctx, cfg, store, log); err != nil {
				log.Error("failed to bootstrap global config", "error", err)
				os.Exit(1)
			}

			if err := httpapi.BootstrapTenantFullSeeding(ctx, cfg, store, log); err != nil {
				log.Error("failed to bootstrap tenant full seeding", "error", err)
				os.Exit(1)
			}
		}

		if err := httpapi.BootstrapAdminAPIKey(ctx, store, log); err != nil {
			log.Error("failed to bootstrap admin api key", "error", err)
			os.Exit(1)
		}
	} else {
		store = storage.NopStorage{}
		log.Info("no DATABASE_URL set, persistence disabled")
	}

	if cfg.ConversationLogging.Enabled && !store.EncryptionConfigured() {
		log.Error("conversation logging is enabled but LOG_ENC_KEY_V1 is not set — " +
			"set LOG_ENC_ACTIVE_VERSION and LOG_ENC_KEY_V1 or disable conversation logging")
		os.Exit(1)
	}

	if dsn != "" {
		httpapi.MergeGlobalProvidersFromStore(ctx, cfg, store, log)
	}

	// Distributed lock client for schedulers that need cluster-wide coordination.
	var distlockClient *redis.Client
	if cfg.CircuitBreaker.Backend == "redis" {
		distlockClient = redis.NewClient(&redis.Options{
			Addr:     cfg.CircuitBreaker.Redis.Addr,
			Password: cfg.CircuitBreaker.Redis.Password,
			DB:       cfg.CircuitBreaker.Redis.DB,
		})
		defer distlockClient.Close()
	}

	reg, err := providers.BuildFromConfig(cfg)
	if err != nil {
		log.Error("failed to build provider registry", "error", err)
		os.Exit(1)
	}

	hookReg := hooks.BuildFromConfig(cfg, log)

	var limiter ratelimit.Limiter
	backend := cfg.RateLimit.Backend
	if backend == "" {
		backend = "in_memory"
	}
	switch backend {
	case "redis":
		redisLimiter, err := ratelimit.NewRedisLimiter(cfg.RateLimit.Redis, log)
		if err != nil {
			log.Error("failed to initialize redis limiter", "error", err)
			os.Exit(1)
		}
		limiter = redisLimiter
		log.Info("rate limiter initialized", "backend", "redis", "addr", cfg.RateLimit.Redis.Addr)
	case "in_memory":
		limiter = ratelimit.NewInMemoryLimiter()
		log.Info("rate limiter initialized", "backend", "in_memory")
	default:
		log.Error("invalid rate limit backend", "backend", backend)
		os.Exit(1)
	}

	var rt *router.Router
	metricsBackend := cfg.SmartRouting.MetricsStore.Backend
	if metricsBackend == "redis" {
		ms, err := router.NewRedisMetricsStore(cfg.SmartRouting.MetricsStore.Redis, log)
		if err != nil {
			log.Warn("failed to initialize redis metrics store, falling back to in_memory", "error", err)
			metricsBackend = "in_memory"
		} else {
			if dsn != "" {
				rt = router.NewWithMetricsStore(store, ms)
			} else {
				rt = router.NewWithMetricsStore(nil, ms)
			}
		}
	}
	if rt == nil {
		if dsn != "" {
			rt = router.NewWithStorage(store)
		} else {
			rt = router.New()
		}
	}
	log.Info("smart routing metrics store initialized", "backend", metricsBackend)

	var breaker circuitbreaker.Breaker
	switch cfg.CircuitBreaker.Backend {
	case "redis":
		rb, err := circuitbreaker.NewRedisBreaker(&cfg.CircuitBreaker, log)
		if err != nil {
			log.Error("failed to initialize circuit breaker", "error", err)
			os.Exit(1)
		}
		breaker = rb
		log.Info("circuit breaker initialized", "backend", "redis", "addr", cfg.CircuitBreaker.Redis.Addr)
	default:
		breaker = circuitbreaker.NoopBreaker{}
		log.Info("circuit breaker initialized", "backend", "in_memory (noop)")
	}

	srv := httpapi.NewServer(cfg, log, rt, reg, hookReg, store, limiter, breaker)

	var cleanup *httpapi.RetentionCleanup
	if dsn != "" {
		cleanup = httpapi.NewRetentionCleanup(cfg, store, log)
		cleanup.Start()
	}

	var benchScheduler *benchmarking.Scheduler
	if cfg.Benchmarking.Enabled {
		benchScheduler = benchmarking.NewScheduler(cfg, store, reg, log, srv.GlobalConfigCache(), distlockClient)
		benchScheduler.Start()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down...")

	if cleanup != nil {
		cleanup.Stop()
	}
	if benchScheduler != nil {
		benchScheduler.Stop()
	}

	srv.Close()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "error", err)
	}
	log.Info("server stopped")
}
