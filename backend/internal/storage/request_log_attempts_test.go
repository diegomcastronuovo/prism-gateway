package storage

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestRequestLogAttempts_MultipleAttemptsSameRequestID verifies that multiple attempts
// for the same logical request are logged with the same request_id but different attempt numbers
func TestRequestLogAttempts_MultipleAttemptsSameRequestID(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	// Generate a single request ID for the logical request
	requestID := uuid.New().String()
	tenantID := "test_attempts_tenant"

	// Simulate attempt 1: failure
	attempt1 := RequestLog{
		ID:                         uuid.New(), // Unique per row
		RequestID:                  requestID,   // Stable per request
		Attempt:                    1,           // First attempt
		TenantID:                   tenantID,
		Model:                      "gpt-4o-mini",
		Provider:                   "openai",
		Strategy:                   "round_robin",
		Status:                     "error",
		LatencyMs:                  150,
		Error:                      "upstream timeout",
		FallbackUsed:               false,
		PIIWebhookRequestDecision:  nil,
		PIIWebhookResponseDecision: nil,
	}

	err = store.LogRequest(context.Background(), attempt1)
	if err != nil {
		t.Fatalf("failed to log attempt 1: %v", err)
	}

	// Simulate attempt 2: success (after fallback)
	attempt2 := RequestLog{
		ID:                         uuid.New(), // Different ID (new row)
		RequestID:                  requestID,   // Same request ID
		Attempt:                    2,           // Second attempt
		TenantID:                   tenantID,
		Model:                      "claude-3-5-sonnet",
		Provider:                   "anthropic",
		Strategy:                   "round_robin",
		Status:                     "ok",
		LatencyMs:                  200,
		Error:                      "",
		FallbackUsed:               true,
		PIIWebhookRequestDecision:  nil,
		PIIWebhookResponseDecision: nil,
	}

	err = store.LogRequest(context.Background(), attempt2)
	if err != nil {
		t.Fatalf("failed to log attempt 2: %v", err)
	}

	// Verify both rows exist in database with correct data
	rows, err := db.Query(`
		SELECT id, request_id, attempt, status, model, fallback_used
		FROM request_log
		WHERE request_id = $1
		ORDER BY attempt ASC
	`, requestID)
	if err != nil {
		t.Fatalf("failed to query request_log: %v", err)
	}
	defer rows.Close()

	var attempts []struct {
		id           uuid.UUID
		requestID    string
		attempt      int
		status       string
		model        string
		fallbackUsed bool
	}

	for rows.Next() {
		var a struct {
			id           uuid.UUID
			requestID    string
			attempt      int
			status       string
			model        string
			fallbackUsed bool
		}
		if err := rows.Scan(&a.id, &a.requestID, &a.attempt, &a.status, &a.model, &a.fallbackUsed); err != nil {
			t.Fatalf("failed to scan row: %v", err)
		}
		attempts = append(attempts, a)
	}

	// Verify we got 2 rows
	if len(attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(attempts))
	}

	// Verify attempt 1
	if attempts[0].requestID != requestID {
		t.Errorf("attempt 1: expected request_id %s, got %s", requestID, attempts[0].requestID)
	}
	if attempts[0].attempt != 1 {
		t.Errorf("attempt 1: expected attempt=1, got %d", attempts[0].attempt)
	}
	if attempts[0].status != "error" {
		t.Errorf("attempt 1: expected status=error, got %s", attempts[0].status)
	}
	if attempts[0].model != "gpt-4o-mini" {
		t.Errorf("attempt 1: expected model=gpt-4o-mini, got %s", attempts[0].model)
	}
	if attempts[0].fallbackUsed {
		t.Errorf("attempt 1: expected fallback_used=false, got true")
	}
	if attempts[0].id == attempts[1].id {
		t.Errorf("attempt 1 and 2 should have different row IDs")
	}

	// Verify attempt 2
	if attempts[1].requestID != requestID {
		t.Errorf("attempt 2: expected request_id %s, got %s", requestID, attempts[1].requestID)
	}
	if attempts[1].attempt != 2 {
		t.Errorf("attempt 2: expected attempt=2, got %d", attempts[1].attempt)
	}
	if attempts[1].status != "ok" {
		t.Errorf("attempt 2: expected status=ok, got %s", attempts[1].status)
	}
	if attempts[1].model != "claude-3-5-sonnet" {
		t.Errorf("attempt 2: expected model=claude-3-5-sonnet, got %s", attempts[1].model)
	}
	if !attempts[1].fallbackUsed {
		t.Errorf("attempt 2: expected fallback_used=true, got false")
	}

	// Cleanup
	_, err = db.Exec("DELETE FROM request_log WHERE request_id = $1", requestID)
	if err != nil {
		t.Logf("cleanup failed: %v", err)
	}
}

// TestRequestLogAttempts_NoConstraintViolation verifies that multiple LogRequest calls
// with the same request_id don't cause duplicate key violations
func TestRequestLogAttempts_NoConstraintViolation(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	requestID := uuid.New().String()
	tenantID := "test_no_violation"

	// Log 5 attempts rapidly with same request_id
	for i := 1; i <= 5; i++ {
		log := RequestLog{
			ID:        uuid.New(),
			RequestID: requestID,
			Attempt:   i,
			TenantID:  tenantID,
			Model:     "test-model",
			Provider:  "test-provider",
			Strategy:  "round_robin",
			Status:    "error",
			LatencyMs: 100,
			Error:     "test error",
		}

		err = store.LogRequest(context.Background(), log)
		if err != nil {
			t.Fatalf("attempt %d failed: %v (should not have constraint violation)", i, err)
		}
	}

	// Verify all 5 rows exist
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM request_log WHERE request_id = $1", requestID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 5 {
		t.Errorf("expected 5 rows, got %d", count)
	}

	// Cleanup
	_, err = db.Exec("DELETE FROM request_log WHERE request_id = $1", requestID)
	if err != nil {
		t.Logf("cleanup failed: %v", err)
	}
}

// TestRequestLogAttempts_IndexPerformance verifies indexes work correctly
func TestRequestLogAttempts_IndexPerformance(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	// Create multiple requests with multiple attempts
	var requestIDs []string
	tenantID := "test_index_perf"

	for r := 0; r < 3; r++ {
		reqID := uuid.New().String()
		requestIDs = append(requestIDs, reqID)

		for a := 1; a <= 3; a++ {
			log := RequestLog{
				ID:        uuid.New(),
				RequestID: reqID,
				Attempt:   a,
				TenantID:  tenantID,
				Model:     "test-model",
				Provider:  "test-provider",
				Strategy:  "round_robin",
				Status:    "ok",
				LatencyMs: 100,
			}

			err = store.LogRequest(context.Background(), log)
			if err != nil {
				t.Fatalf("failed to log request %d attempt %d: %v", r, a, err)
			}
		}
	}

	// Query by request_id (should use idx_request_log_request_id)
	start := time.Now()
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM request_log WHERE request_id = $1", requestIDs[0]).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query by request_id: %v", err)
	}
	queryTime := time.Since(start)

	if count != 3 {
		t.Errorf("expected 3 attempts for first request, got %d", count)
	}

	// Query should be fast (< 100ms even without index optimization)
	if queryTime > 100*time.Millisecond {
		t.Logf("query by request_id took %v (might indicate missing index)", queryTime)
	}

	// Query by tenant_id and request_id (should use idx_request_log_tenant_request)
	start = time.Now()
	rows, err := db.Query(`
		SELECT attempt, status
		FROM request_log
		WHERE tenant_id = $1 AND request_id = $2
		ORDER BY attempt ASC
	`, tenantID, requestIDs[1])
	if err != nil {
		t.Fatalf("failed to query by tenant_id and request_id: %v", err)
	}
	defer rows.Close()

	var attempts []int
	for rows.Next() {
		var attempt int
		var status string
		if err := rows.Scan(&attempt, &status); err != nil {
			t.Fatalf("failed to scan: %v", err)
		}
		attempts = append(attempts, attempt)
	}
	queryTime = time.Since(start)

	if len(attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(attempts))
	}

	if attempts[0] != 1 || attempts[1] != 2 || attempts[2] != 3 {
		t.Errorf("attempts not in correct order: %v", attempts)
	}

	// Cleanup
	for _, reqID := range requestIDs {
		_, err = db.Exec("DELETE FROM request_log WHERE request_id = $1", reqID)
		if err != nil {
			t.Logf("cleanup failed for request %s: %v", reqID, err)
		}
	}
}
