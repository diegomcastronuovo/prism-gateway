package httpapi

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

type mockStorageForRetention struct {
	storage.NopStorage
	deleteCallCount int
	lastTenantID    string
	lastCutoff      time.Time
	deletedCount    int
}

func (m *mockStorageForRetention) DeleteOldRecords(ctx context.Context, tenantID string, cutoffDate time.Time) (int, error) {
	m.deleteCallCount++
	m.lastTenantID = tenantID
	m.lastCutoff = cutoffDate
	return m.deletedCount, nil
}

func TestRetentionCleanup_RunCleanup(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{
				ID: "tenant_a",
				Compliance: config.ComplianceConfig{
					RetentionDays: 90,
				},
			},
			{
				ID: "tenant_b",
				Compliance: config.ComplianceConfig{
					RetentionDays: 30,
				},
			},
			{
				ID: "tenant_c",
				Compliance: config.ComplianceConfig{
					RetentionDays: 0, // Not configured, should skip
				},
			},
		},
	}

	mockStore := &mockStorageForRetention{deletedCount: 5}
	log := slog.Default()

	rc := NewRetentionCleanup(cfg, mockStore, log)

	// Run cleanup once
	rc.runCleanup()

	// Should have been called 2 times (tenant_a and tenant_b, skipping tenant_c)
	if mockStore.deleteCallCount != 2 {
		t.Errorf("DeleteOldRecords called %d times, want 2", mockStore.deleteCallCount)
	}
}

func TestRetentionCleanup_CutoffDateCalculation(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{
				ID: "tenant_a",
				Compliance: config.ComplianceConfig{
					RetentionDays: 90,
				},
			},
		},
	}

	mockStore := &mockStorageForRetention{}
	log := slog.Default()

	rc := NewRetentionCleanup(cfg, mockStore, log)
	rc.runCleanup()

	// Verify cutoff date is approximately 90 days ago
	expectedCutoff := time.Now().UTC().AddDate(0, 0, -90)
	diff := mockStore.lastCutoff.Sub(expectedCutoff)
	if diff > time.Minute || diff < -time.Minute {
		t.Errorf("Cutoff date mismatch: got %v, expected around %v (diff: %v)",
			mockStore.lastCutoff, expectedCutoff, diff)
	}
}

func TestRetentionCleanup_StartStop(t *testing.T) {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{
				ID: "tenant_a",
				Compliance: config.ComplianceConfig{
					RetentionDays: 90,
				},
			},
		},
	}

	mockStore := &mockStorageForRetention{}
	log := slog.Default()

	rc := NewRetentionCleanup(cfg, mockStore, log)

	// Start cleanup (non-blocking)
	rc.Start()

	// Give it a moment to run the initial cleanup
	time.Sleep(100 * time.Millisecond)

	// Stop cleanup
	rc.Stop()

	// Should have been called at least once (initial run)
	if mockStore.deleteCallCount < 1 {
		t.Errorf("DeleteOldRecords called %d times, want at least 1", mockStore.deleteCallCount)
	}
}
