package httpapi

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// StatsDispatcher manages asynchronous model stats recording with bounded concurrency.
type StatsDispatcher struct {
	store   storage.Storage
	log     *slog.Logger
	queue   chan storage.ModelStatDaily
	workers int
	wg      sync.WaitGroup
	stopped atomic.Bool
	dropped atomic.Int64
}

// NewStatsDispatcher creates a stats dispatcher with bounded queue and worker pool.
// queueSize: buffered channel capacity (recommend 1000-5000)
// workers: number of concurrent DB writers (recommend 2-4)
func NewStatsDispatcher(store storage.Storage, log *slog.Logger, queueSize, workers int) *StatsDispatcher {
	d := &StatsDispatcher{
		store:   store,
		log:     log,
		queue:   make(chan storage.ModelStatDaily, queueSize),
		workers: workers,
	}
	d.start()
	return d
}

func (d *StatsDispatcher) start() {
	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go d.worker(i)
	}
}

func (d *StatsDispatcher) worker(id int) {
	defer d.wg.Done()

	for stat := range d.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := d.store.UpsertModelStatDaily(ctx, stat)
		cancel()

		if err != nil {
			d.log.ErrorContext(context.Background(), "stats worker failed to upsert",
				"worker_id", id,
				"tenant", stat.TenantID,
				"model", stat.Model,
				"error", err)
		}
	}
}

// Submit attempts to queue a stat update. Returns false if queue is full (non-blocking).
func (d *StatsDispatcher) Submit(stat storage.ModelStatDaily) bool {
	if d.stopped.Load() {
		return false
	}

	select {
	case d.queue <- stat:
		return true
	default:
		// Queue full, drop update
		dropped := d.dropped.Add(1)
		gatewayotel.StatsDispatcherDropTotal.Inc()
		if dropped%100 == 1 { // Log every 100th drop to avoid log spam
			d.log.WarnContext(context.Background(), "stats queue full, dropping update",
				"tenant", stat.TenantID,
				"model", stat.Model,
				"total_dropped", dropped)
		}
		return false
	}
}

// SubmitBlocking attempts to queue with timeout. Blocks briefly if queue is full.
func (d *StatsDispatcher) SubmitBlocking(stat storage.ModelStatDaily, timeout time.Duration) bool {
	if d.stopped.Load() {
		return false
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case d.queue <- stat:
		return true
	case <-timer.C:
		// Timeout, drop update
		dropped := d.dropped.Add(1)
		gatewayotel.StatsDispatcherDropTotal.Inc()
		if dropped%100 == 1 {
			d.log.WarnContext(context.Background(), "stats queue timeout, dropping update",
				"tenant", stat.TenantID,
				"model", stat.Model,
				"timeout_ms", timeout.Milliseconds(),
				"total_dropped", dropped)
		}
		return false
	}
}

// Stop gracefully shuts down the dispatcher, waiting for queued items to flush.
func (d *StatsDispatcher) Stop(timeout time.Duration) {
	if !d.stopped.CompareAndSwap(false, true) {
		return // Already stopped
	}

	close(d.queue)

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		d.log.Info("stats dispatcher stopped gracefully", "dropped_total", d.dropped.Load())
	case <-time.After(timeout):
		d.log.Warn("stats dispatcher shutdown timeout", "timeout_ms", timeout.Milliseconds(), "dropped_total", d.dropped.Load())
	}
}

// DroppedCount returns the total number of dropped updates.
func (d *StatsDispatcher) DroppedCount() int64 {
	return d.dropped.Load()
}
