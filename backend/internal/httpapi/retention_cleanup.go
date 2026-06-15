package httpapi

import (
	"context"
	"log/slog"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// RetentionCleanup runs daily cleanup of old request/usage data per tenant policy
type RetentionCleanup struct {
	cfg    *config.Config
	store  storage.Storage
	log    *slog.Logger
	ticker *time.Ticker
	done   chan struct{}
}

func NewRetentionCleanup(cfg *config.Config, store storage.Storage, log *slog.Logger) *RetentionCleanup {
	return &RetentionCleanup{
		cfg:   cfg,
		store: store,
		log:   log,
		done:  make(chan struct{}),
	}
}

// Start begins the daily cleanup ticker (non-blocking)
func (rc *RetentionCleanup) Start() {
	// Run once immediately at startup
	go rc.runCleanup()

	// Schedule to run every 24 hours from startup
	rc.ticker = time.NewTicker(24 * time.Hour)

	go func() {
		for {
			select {
			case <-rc.ticker.C:
				rc.runCleanup()
			case <-rc.done:
				return
			}
		}
	}()

	rc.log.Info("retention cleanup job started", "interval", "24h", "note", "runs every 24h from startup time")
}

// Stop gracefully stops the cleanup job
func (rc *RetentionCleanup) Stop() {
	if rc.ticker != nil {
		rc.ticker.Stop()
	}
	close(rc.done)
	rc.log.Info("retention cleanup job stopped")
}

func (rc *RetentionCleanup) runCleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	rc.log.Info("retention cleanup started")

	// Purge expired budget reservations (orphaned from failed/crashed requests).
	if deleted, err := rc.store.PurgeExpiredReservations(ctx); err != nil {
		rc.log.ErrorContext(ctx, "failed to purge expired budget reservations", "error", err)
	} else if deleted > 0 {
		rc.log.InfoContext(ctx, "purged expired budget reservations", "count", deleted)
	}

	for _, tenant := range rc.cfg.Tenants {
		if tenant.Compliance.RetentionDays == 0 {
			continue // Skip if not configured
		}

		cutoffDate := time.Now().UTC().AddDate(0, 0, -tenant.Compliance.RetentionDays)

		deleted, err := rc.store.DeleteOldRecords(ctx, tenant.ID, cutoffDate)
		if err != nil {
			rc.log.ErrorContext(ctx, "retention cleanup failed",
				"tenant", tenant.ID,
				"retention_days", tenant.Compliance.RetentionDays,
				"error", err,
			)
			continue
		}

		if deleted > 0 {
			rc.log.InfoContext(ctx, "retention cleanup completed",
				"tenant", tenant.ID,
				"retention_days", tenant.Compliance.RetentionDays,
				"cutoff_date", cutoffDate.Format("2006-01-02"),
				"records_deleted", deleted,
			)
		}
	}

	rc.log.Info("retention cleanup finished")
}
