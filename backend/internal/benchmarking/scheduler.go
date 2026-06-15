package benchmarking

import (
	"context"
	"log/slog"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/distlock"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/redis/go-redis/v9"
)

// Scheduler periodically runs the benchmark runner and optionally cleans up old rows.
type Scheduler struct {
	runner         *Runner
	cfg            *config.BenchmarkingConfig
	mainCfg        *config.Config
	globalCfgCache *config.GlobalConfigCache
	store          storage.Storage
	log            *slog.Logger
	ticker         *time.Ticker
	done           chan struct{}
	redisClient    *redis.Client
}

// NewScheduler creates a Scheduler.
// redisClient may be nil (disables distributed lock).
func NewScheduler(cfg *config.Config, store storage.Storage, registry *providers.Registry, log *slog.Logger, globalCfgCache *config.GlobalConfigCache, redisClient *redis.Client) *Scheduler {
	return &Scheduler{
		runner:         NewRunner(cfg, store, registry, log, globalCfgCache),
		cfg:            &cfg.Benchmarking,
		mainCfg:        cfg,
		globalCfgCache: globalCfgCache,
		store:          store,
		log:            log,
		done:           make(chan struct{}),
		redisClient:    redisClient,
	}
}

// Start begins the benchmarking ticker (non-blocking).
// It runs one round immediately on startup, then repeats every interval_minutes.
func (s *Scheduler) Start() {
	interval := time.Duration(s.cfg.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	go s.runOnce()

	s.ticker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.runOnce()
			case <-s.done:
				return
			}
		}
	}()

	s.log.Info("model benchmark scheduler started",
		"interval", interval.String(),
		"max_concurrency", s.cfg.MaxConcurrency,
	)
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.done)
	s.log.Info("model benchmark scheduler stopped")
}

// workflowConversationTTL reads the TTL from GlobalConfig (cache → DB → default).
// Default: 3600 seconds (1 hour).
func (s *Scheduler) workflowConversationTTL() time.Duration {
	const defaultTTL = 3600 * time.Second
	if s.globalCfgCache == nil || s.mainCfg == nil {
		return defaultTTL
	}
	gc, _, ok := s.globalCfgCache.Get()
	if !ok || gc == nil {
		return defaultTTL
	}
	if gc.WorkflowConversationTTLSeconds > 0 {
		return time.Duration(gc.WorkflowConversationTTLSeconds) * time.Second
	}
	return defaultTTL
}

// workflowSnapshotRetentionDays reads the snapshot retention setting from GlobalConfig.
// Default: 90 days. Returns 0 if retention cleanup should be skipped.
func (s *Scheduler) workflowSnapshotRetentionDays() int {
	const defaultDays = 90
	if s.globalCfgCache == nil || s.mainCfg == nil {
		return defaultDays
	}
	gc, _, ok := s.globalCfgCache.Get()
	if !ok || gc == nil {
		return defaultDays
	}
	if gc.WorkflowSnapshotRetentionDays > 0 {
		return gc.WorkflowSnapshotRetentionDays
	}
	return defaultDays
}

func (s *Scheduler) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Try distributed lock — skip if another pod is running this job
	if s.redisClient != nil {
		ttl := time.Duration(s.cfg.IntervalMinutes) * time.Minute
		if !distlock.TryAcquire(ctx, s.redisClient, "scheduler:benchmark:lock", ttl) {
			s.log.Debug("model benchmark scheduler: lock held by another pod, skipping")
			return
		}
	}

	// Run benchmarks (fail-open: panics are not propagated).
	func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("model benchmark panic recovered", "panic", r)
			}
		}()
		s.runner.Run(ctx)
	}()

	// Best-effort retention cleanup.
	if s.cfg.Storage.RetainDays > 0 {
		deleted, err := s.store.DeleteOldModelBenchmarks(ctx, s.cfg.Storage.RetainDays)
		if err != nil {
			s.log.Warn("model benchmark retention cleanup failed", "error", err)
		} else if deleted > 0 {
			s.log.Info("model benchmark retention cleanup", "deleted_rows", deleted)
		}
	}

	// Workflow conversation TTL cleanup + snapshot (SPEC_173).
	// Snapshots each expired row before deleting it from workflow_conversations.
	wfTTL := s.workflowConversationTTL()
	cutoff := time.Now().Add(-wfTTL)
	if wfProcessed, wfErr := s.store.SnapshotAndDeleteExpiredConversations(ctx, cutoff); wfErr != nil {
		s.log.Warn("workflow conversation snapshot+cleanup failed", "error", wfErr)
	} else if wfProcessed > 0 {
		s.log.Info("workflow conversation snapshot+cleanup",
			"processed", wfProcessed, "ttl", wfTTL.String())
	}

	// Workflow snapshot retention cleanup (SPEC_173).
	snapshotRetentionDays := s.workflowSnapshotRetentionDays()
	if snapshotRetentionDays > 0 {
		cutoffSnap := time.Now().AddDate(0, 0, -snapshotRetentionDays)
		if deleted, err := s.store.DeleteOldWorkflowConversationSnapshots(ctx, cutoffSnap); err != nil {
			s.log.Warn("workflow snapshot retention cleanup failed", "error", err)
		} else if deleted > 0 {
			s.log.Info("workflow snapshot retention cleanup",
				"deleted_rows", deleted, "retention_days", snapshotRetentionDays)
		}
	}
}
