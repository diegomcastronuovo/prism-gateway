package router

import (
	"context"
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

func metricsLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ---------- InMemoryMetricsStore tests ----------

func TestInMemoryMetricsStore_EWMAUpdate(t *testing.T) {
	s := NewInMemoryMetricsStore()
	ctx := context.Background()

	// First sample initialises EWMA to the sample value.
	if err := s.UpdateLatencyEWMA(ctx, "t1", "m1", 100.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, _ := s.GetLatencyEWMA(ctx, "t1", []string{"m1"})
	if m["m1"] != 100.0 {
		t.Errorf("expected 100.0 on first sample, got %f", m["m1"])
	}

	// Second sample: ewma = alpha*new + (1-alpha)*prev = 0.3*200 + 0.7*100 = 130.
	if err := s.UpdateLatencyEWMA(ctx, "t1", "m1", 200.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, _ = s.GetLatencyEWMA(ctx, "t1", []string{"m1"})
	expected := ewmaAlpha*200.0 + (1-ewmaAlpha)*100.0
	if math.Abs(m["m1"]-expected) > 0.001 {
		t.Errorf("expected EWMA %.4f, got %.4f", expected, m["m1"])
	}
}

func TestInMemoryMetricsStore_CountersReqErr(t *testing.T) {
	s := NewInMemoryMetricsStore()
	ctx := context.Background()

	s.IncRequest(ctx, "t1", "m1", false) // success
	s.IncRequest(ctx, "t1", "m1", false) // success
	s.IncRequest(ctx, "t1", "m1", true)  // error

	stats, _ := s.GetErrorStats(ctx, "t1", []string{"m1"})
	es, ok := stats["m1"]
	if !ok {
		t.Fatal("expected stats for m1")
	}
	if es.RequestCount != 3 {
		t.Errorf("expected 3 requests, got %d", es.RequestCount)
	}
	if es.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", es.ErrorCount)
	}
	wantRate := 1.0 / 3.0
	if math.Abs(es.ErrorRate-wantRate) > 0.001 {
		t.Errorf("expected error rate %.4f, got %.4f", wantRate, es.ErrorRate)
	}
}

func TestInMemoryMetricsStore_BatchRead(t *testing.T) {
	s := NewInMemoryMetricsStore()
	ctx := context.Background()

	s.UpdateLatencyEWMA(ctx, "t1", "m1", 100.0)
	s.UpdateLatencyEWMA(ctx, "t1", "m2", 200.0)
	// m3 has no data

	lats, _ := s.GetLatencyEWMA(ctx, "t1", []string{"m1", "m2", "m3"})
	if lats["m1"] != 100.0 {
		t.Errorf("expected 100 for m1, got %f", lats["m1"])
	}
	if lats["m2"] != 200.0 {
		t.Errorf("expected 200 for m2, got %f", lats["m2"])
	}
	if _, ok := lats["m3"]; ok {
		t.Error("m3 should be absent (no data)")
	}
}

func TestInMemoryMetricsStore_UnknownTenantReturnsEmpty(t *testing.T) {
	s := NewInMemoryMetricsStore()
	ctx := context.Background()

	lats, err := s.GetLatencyEWMA(ctx, "nope", []string{"m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lats) != 0 {
		t.Errorf("expected empty map, got %v", lats)
	}

	stats, err := s.GetErrorStats(ctx, "nope", []string{"m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected empty map, got %v", stats)
	}
}

func TestInMemoryMetricsStore_CounterDecayAfter100(t *testing.T) {
	s := NewInMemoryMetricsStore()
	ctx := context.Background()

	// Push 101 requests: 50% errors
	for i := 0; i < 101; i++ {
		s.IncRequest(ctx, "t1", "m1", i%2 == 0)
	}
	stats, _ := s.GetErrorStats(ctx, "t1", []string{"m1"})
	es := stats["m1"]
	// After decay: req=50 (from 101), counters halved — just verify it's < 101
	if es.RequestCount >= 101 {
		t.Errorf("expected decay: req < 101, got %d", es.RequestCount)
	}
}

// ---------- Router integration with InMemoryMetricsStore ----------

func TestRouter_LatencyBasedUsesMetricsStore(t *testing.T) {
	rt := New()
	rt.RecordLatency("t1", "gpt-4o-mini", 200)
	rt.RecordLatency("t1", "claude-3-5-sonnet", 50)
	rt.RecordLatency("t1", "local-llama", 100)

	req := Request{
		TenantID:   "t1",
		Strategy:   "latency",
		Candidates: testModels,
	}
	result, err := rt.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Selected != "claude-3-5-sonnet" {
		t.Errorf("expected claude-3-5-sonnet (lowest latency 50ms), got %s", result.Selected)
	}
}

func TestRouter_GetLatency_ReadsFromStore(t *testing.T) {
	rt := New()
	rt.RecordLatency("t1", "m1", 100.0)

	v, ok := rt.GetLatency("t1", "m1")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != 100.0 {
		t.Errorf("expected 100.0, got %f", v)
	}

	_, ok = rt.GetLatency("t1", "unknown")
	if ok {
		t.Error("expected ok=false for unknown model")
	}
}

func TestRouter_TwoInstancesSharedStore_ConsistentOrdering(t *testing.T) {
	// Simulates 2 router instances sharing the same MetricsStore.
	// After one instance records latency, the other should see it.
	shared := NewInMemoryMetricsStore()

	rt1 := &Router{rrIndex: make(map[string]int), metricsStore: shared}
	rt2 := &Router{rrIndex: make(map[string]int), metricsStore: shared}

	// Instance 1 records latencies
	rt1.RecordLatency("t1", "gpt-4o-mini", 300)
	rt1.RecordLatency("t1", "local-llama", 50)
	rt1.RecordLatency("t1", "claude-3-5-sonnet", 150)

	// Instance 2 selects by latency — must see instance 1's data
	req := Request{TenantID: "t1", Strategy: "latency", Candidates: testModels}
	result, err := rt2.Select(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Selected != "local-llama" {
		t.Errorf("expected local-llama (50ms), got %s", result.Selected)
	}
}

// ---------- RedisMetricsStore tests (using miniredis) ----------

func newTestRedisMetricsStore(t *testing.T, addr string) *RedisMetricsStore {
	t.Helper()
	cfg := config.RedisLimiterConfig{
		Addr:          addr,
		DialTimeoutMs: 500,
		OpTimeoutMs:   300,
		KeyPrefix:     "sr:",
	}
	store, err := NewRedisMetricsStore(cfg, metricsLogger())
	if err != nil {
		t.Fatalf("failed to create redis metrics store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestRedisMetricsStore_EWMAUpdate(t *testing.T) {
	mr := miniredis.RunT(t)
	s := newTestRedisMetricsStore(t, mr.Addr())
	ctx := context.Background()

	// First sample.
	s.UpdateLatencyEWMA(ctx, "t1", "m1", 100.0)
	lats, err := s.GetLatencyEWMA(ctx, "t1", []string{"m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(lats["m1"]-100.0) > 0.001 {
		t.Errorf("expected 100.0, got %f", lats["m1"])
	}

	// Second sample: ewma = 0.3*200 + 0.7*100 = 130.
	s.UpdateLatencyEWMA(ctx, "t1", "m1", 200.0)
	lats, _ = s.GetLatencyEWMA(ctx, "t1", []string{"m1"})
	expected := ewmaAlpha*200.0 + (1-ewmaAlpha)*100.0
	if math.Abs(lats["m1"]-expected) > 0.01 {
		t.Errorf("expected EWMA %.4f, got %.4f", expected, lats["m1"])
	}
}

func TestRedisMetricsStore_Counters(t *testing.T) {
	mr := miniredis.RunT(t)
	s := newTestRedisMetricsStore(t, mr.Addr())
	ctx := context.Background()

	s.IncRequest(ctx, "t1", "m1", false)
	s.IncRequest(ctx, "t1", "m1", false)
	s.IncRequest(ctx, "t1", "m1", true)

	stats, err := s.GetErrorStats(ctx, "t1", []string{"m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	es, ok := stats["m1"]
	if !ok {
		t.Fatal("expected stats for m1")
	}
	if es.RequestCount != 3 {
		t.Errorf("expected 3 requests, got %d", es.RequestCount)
	}
	if es.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", es.ErrorCount)
	}
}

func TestRedisMetricsStore_BatchRead(t *testing.T) {
	mr := miniredis.RunT(t)
	s := newTestRedisMetricsStore(t, mr.Addr())
	ctx := context.Background()

	s.UpdateLatencyEWMA(ctx, "t1", "m1", 100.0)
	s.UpdateLatencyEWMA(ctx, "t1", "m2", 200.0)

	lats, _ := s.GetLatencyEWMA(ctx, "t1", []string{"m1", "m2", "m3"})
	if math.Abs(lats["m1"]-100.0) > 0.01 {
		t.Errorf("expected 100 for m1, got %f", lats["m1"])
	}
	if math.Abs(lats["m2"]-200.0) > 0.01 {
		t.Errorf("expected 200 for m2, got %f", lats["m2"])
	}
	if _, ok := lats["m3"]; ok {
		t.Error("m3 should be absent from result")
	}
}

func TestRedisMetricsStore_TTLRenewed(t *testing.T) {
	mr := miniredis.RunT(t)
	s := newTestRedisMetricsStore(t, mr.Addr())
	ctx := context.Background()

	s.UpdateLatencyEWMA(ctx, "t1", "m1", 100.0)

	// Verify TTL is set on the EWMA key.
	ttl := mr.TTL("sr:ewma:t1")
	if ttl <= 0 {
		t.Errorf("expected positive TTL on ewma key, got %v", ttl)
	}
	// TTL must be close to 7 days.
	sevenDays := 7 * 24 * time.Hour
	if ttl > sevenDays+time.Minute || ttl < sevenDays-time.Minute {
		t.Errorf("TTL out of range: %v (expected ~7d)", ttl)
	}

	s.IncRequest(ctx, "t1", "m1", false)
	cntTTL := mr.TTL("sr:cnt:t1")
	if cntTTL <= 0 {
		t.Errorf("expected positive TTL on cnt key, got %v", cntTTL)
	}
}

func TestRedisMetricsStore_RedisDownReturnsEmptyNoError(t *testing.T) {
	// Start miniredis, connect, then close it to simulate Redis going down.
	mr := miniredis.RunT(t)
	s := newTestRedisMetricsStore(t, mr.Addr())

	// Write a value while Redis is up.
	ctx := context.Background()
	s.UpdateLatencyEWMA(ctx, "t1", "m1", 100.0)

	// Bring Redis down.
	mr.Close()

	// Write operations must silently degrade — no returned error.
	if err := s.UpdateLatencyEWMA(ctx, "t1", "m1", 200.0); err != nil {
		t.Errorf("expected nil error on redis-down UpdateLatencyEWMA, got: %v", err)
	}
	if err := s.IncRequest(ctx, "t1", "m1", false); err != nil {
		t.Errorf("expected nil error on redis-down IncRequest, got: %v", err)
	}

	// Read operations must return empty map, not an error (degrade gracefully).
	lats, err := s.GetLatencyEWMA(ctx, "t1", []string{"m1"})
	if err != nil {
		t.Errorf("expected nil error on redis-down GetLatencyEWMA, got: %v", err)
	}
	if len(lats) != 0 {
		t.Errorf("expected empty map on redis-down, got: %v", lats)
	}
	stats, err := s.GetErrorStats(ctx, "t1", []string{"m1"})
	if err != nil {
		t.Errorf("expected nil error on redis-down GetErrorStats, got: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected empty stats on redis-down, got: %v", stats)
	}
}
