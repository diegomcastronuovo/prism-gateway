package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/diegomcastronuovo/prism-gateway/internal/encryption"
	"github.com/google/uuid"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestPostgres_LogRequest_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	reqID := uuid.New().String()
	rl := RequestLog{
		ID:                         uuid.New(),
		RequestID:                  reqID,
		Attempt:                    1,
		Timestamp:                  time.Now(),
		TenantID:                   "tenant_a",
		Model:                      "gpt-4o-mini",
		Provider:                   "openai",
		Strategy:                   "round_robin",
		Status:                     "ok",
		LatencyMs:                  150,
		Error:                      "",
		FallbackUsed:               false,
		PIIWebhookRequestDecision:  nil,
		PIIWebhookResponseDecision: nil,
	}

	mock.ExpectExec("INSERT INTO request_log").
		WithArgs(
			rl.ID, rl.RequestID, rl.Attempt, rl.TenantID, rl.Model, rl.Provider, rl.Strategy, rl.Status,
			rl.LatencyMs, rl.Error, rl.FallbackUsed,
			rl.PIIWebhookRequestDecision, rl.PIIWebhookResponseDecision,
			rl.DecisionReason, rl.ErrorType, nil, nil, nil,
			rl.RouterPreMS, rl.LLMLatencyMS, rl.RouterPostMS,
			rl.PreDecodeMS, rl.PreAuthzMS, rl.PreTenantConfigMS, rl.PrePIIMS, rl.PreRateLimitMS, rl.PreModelFilterMS, rl.PreRoutingMS, rl.PreRequestBuildMS,
			rl.CfgToolRoutesMS, rl.CfgDynamicRoutesMS, rl.CfgBudgetPressureMS, rl.CfgSemanticMS, rl.CfgModelResolutionMS,
			rl.ToolRoutesEmbeddingModelMS, rl.ToolRoutesEmbeddingGenerateMS, rl.ToolRoutesSemanticDBMS, rl.ToolRoutesMatchEvalMS,
			rl.APIKeyID, rl.APIKeyName, rl.JWTSub,
			rl.CustomerID, rl.Channel, rl.InteractionType, rl.AgentID, rl.Department, rl.TicketID, rl.CustomerSegment, rl.Language,
			rl.Intent, rl.ExperimentID, rl.AutonomyLevel, rl.PolicyID, rl.RiskLevel, rl.RevenueImpact, rl.Currency,
			rl.CachedTokens, rl.ToolCostUSD,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.LogRequest(context.Background(), rl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_LogRequest_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	reqID := uuid.New().String()
	rl := RequestLog{
		ID:                         uuid.New(),
		RequestID:                  reqID,
		Attempt:                    2,
		TenantID:                   "tenant_a",
		Model:                      "gpt-4o-mini",
		Provider:                   "openai",
		Strategy:                   "round_robin",
		Status:                     "error",
		LatencyMs:                  50,
		Error:                      "upstream returned status 500",
		FallbackUsed:               true,
		PIIWebhookRequestDecision:  nil,
		PIIWebhookResponseDecision: nil,
	}

	mock.ExpectExec("INSERT INTO request_log").
		WithArgs(
			rl.ID, rl.RequestID, rl.Attempt, rl.TenantID, rl.Model, rl.Provider, rl.Strategy, rl.Status,
			rl.LatencyMs, rl.Error, rl.FallbackUsed,
			rl.PIIWebhookRequestDecision, rl.PIIWebhookResponseDecision,
			rl.DecisionReason, rl.ErrorType, nil, nil, nil,
			rl.RouterPreMS, rl.LLMLatencyMS, rl.RouterPostMS,
			rl.PreDecodeMS, rl.PreAuthzMS, rl.PreTenantConfigMS, rl.PrePIIMS, rl.PreRateLimitMS, rl.PreModelFilterMS, rl.PreRoutingMS, rl.PreRequestBuildMS,
			rl.CfgToolRoutesMS, rl.CfgDynamicRoutesMS, rl.CfgBudgetPressureMS, rl.CfgSemanticMS, rl.CfgModelResolutionMS,
			rl.ToolRoutesEmbeddingModelMS, rl.ToolRoutesEmbeddingGenerateMS, rl.ToolRoutesSemanticDBMS, rl.ToolRoutesMatchEvalMS,
			rl.APIKeyID, rl.APIKeyName, rl.JWTSub,
			rl.CustomerID, rl.Channel, rl.InteractionType, rl.AgentID, rl.Department, rl.TicketID, rl.CustomerSegment, rl.Language,
			rl.Intent, rl.ExperimentID, rl.AutonomyLevel, rl.PolicyID, rl.RiskLevel, rl.RevenueImpact, rl.Currency,
			rl.CachedTokens, rl.ToolCostUSD,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.LogRequest(context.Background(), rl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_LogRequest_DBFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	mock.ExpectExec("INSERT INTO request_log").
		WillReturnError(fmt.Errorf("connection refused"))

	err = store.LogRequest(context.Background(), RequestLog{
		ID:        uuid.New(),
		RequestID: uuid.New().String(),
		Attempt:   1,
	})
	if err == nil {
		t.Fatal("expected error on DB failure")
	}
}

func TestPostgres_LogConversation_EncryptsSensitiveFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	enc, err := encryption.NewFieldEncryptionService("v1", map[string][]byte{"v1": key})
	if err != nil {
		t.Fatalf("encryption init: %v", err)
	}
	store.encService = enc

	row := ConversationLog{
		ID:               uuid.New(),
		RequestID:        uuid.New().String(),
		TenantID:         "tenant_a",
		PromptPreview:    "hello",
		ResponsePreview:  "world",
		PromptRedacted:   stringPtr("redacted"),
		ResponseRedacted: stringPtr("response"),
		PromptFull:       stringPtr("prompt full"),
		ResponseFull:     stringPtr("response full"),
		PIIDetected:      true,
		LoggingMode:      "full",
	}

	mock.ExpectExec("INSERT INTO conversation_log").
		WithArgs(
			row.ID, row.RequestID, row.TenantID, row.JWTSub,
			row.WorkflowID, row.ConversationID, row.CustomerID,
			row.PromptPreview, row.ResponsePreview,
			nil, nil, nil, nil,
			row.PIIDetected, row.LoggingMode,
			"v1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.LogConversation(context.Background(), row); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgres_ListConversations_DecryptsFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(40 + i)
	}
	enc, err := encryption.NewFieldEncryptionService("v1", map[string][]byte{"v1": key})
	if err != nil {
		t.Fatalf("encryption init: %v", err)
	}
	store.encService = enc

	ctPrompt, _, err := enc.EncryptString("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	rows := sqlmock.NewRows([]string{
		"id", "request_id", "tenant_id", "jwt_sub", "workflow_id", "conversation_id", "customer_id",
		"prompt_preview", "response_preview",
		"prompt_redacted", "response_redacted", "prompt_full", "response_full",
		"pii_detected", "logging_mode", "created_at",
		"enc_key_version", "prompt_redacted_enc", "response_redacted_enc", "prompt_full_enc", "response_full_enc",
	}).AddRow(
		uuid.New(), "req-1", "tenant_a", nil, nil, nil, nil,
		"preview", "preview2",
		"legacy", nil, nil, nil,
		false, "redacted", time.Now().UTC(),
		"v1", ctPrompt, nil, nil, nil,
	)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM conversation_log").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, request_id, tenant_id, jwt_sub").
		WillReturnRows(rows)

	result, total, err := store.ListConversations(context.Background(), ConversationLogFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 || len(result) != 1 {
		t.Fatalf("expected 1 row, got total=%d len=%d", total, len(result))
	}
	if result[0].PromptRedacted == nil || *result[0].PromptRedacted != "secret" {
		t.Fatalf("prompt_redacted=%v, want secret", result[0].PromptRedacted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestPostgres_SaveUsage_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	requestID := uuid.New().String()
	u := UsageRecord{
		ID:               uuid.New(),
		Timestamp:        time.Now(),
		TenantID:         "tenant_a",
		Model:            "gpt-4o-mini",
		Provider:         "openai",
		PromptTokens:     42,
		CompletionTokens: 18,
		TotalTokens:      60,
		CostUSD:          0.000017,
		RequestID:        requestID,
	}

	mock.ExpectExec("INSERT INTO usage").
		WithArgs(u.ID, u.TenantID, u.Model, u.Provider, u.PromptTokens, u.CompletionTokens, u.TotalTokens, u.CostUSD, u.RequestID, u.APIKeyID, u.APIKeyName, u.JWTSub).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.SaveUsage(context.Background(), u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_SaveUsage_DBFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	mock.ExpectExec("INSERT INTO usage").
		WillReturnError(fmt.Errorf("disk full"))

	err = store.SaveUsage(context.Background(), UsageRecord{ID: uuid.New(), RequestID: uuid.New().String()})
	if err == nil {
		t.Fatal("expected error on DB failure")
	}
}

func TestPostgres_GetAPIKeyMetaByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()
	store := NewPostgresFromDB(db, testLogger())
	keyID := uuid.New()

	// QueryRowContext returns ErrNoRows when no rows
	mock.ExpectQuery("SELECT id, tenant_id, name, prefix, scopes, created_at, expires_at, revoked_at, last_used_at").
		WithArgs(keyID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "prefix", "scopes", "created_at", "expires_at", "revoked_at", "last_used_at"}))

	_, found, err := store.GetAPIKeyMetaByID(context.Background(), keyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_GetAPIKeyMetaByID_Found(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()
	store := NewPostgresFromDB(db, testLogger())
	keyID := uuid.New()
	createdAt := time.Now().UTC()
	scopesJSON := []byte(`["inference"]`)

	mock.ExpectQuery("SELECT id, tenant_id, name, prefix, scopes, created_at, expires_at, revoked_at, last_used_at").
		WithArgs(keyID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "prefix", "scopes", "created_at", "expires_at", "revoked_at", "last_used_at"}).
			AddRow(keyID, "tenant_a", "my-key", "rk_live_abc", scopesJSON, createdAt, nil, nil, nil))

	meta, found, err := store.GetAPIKeyMetaByID(context.Background(), keyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected found=true")
	}
	if meta.TenantID != "tenant_a" || meta.Name != "my-key" {
		t.Errorf("meta tenant_id=%q name=%q", meta.TenantID, meta.Name)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_ListAPIKeyRawUsage_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()
	store := NewPostgresFromDB(db, testLogger())
	filter := APIKeyRawUsageFilter{Limit: 50, Offset: 0}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM request_log rl INNER JOIN usage u ON").
		WithArgs(nil, nil, nil, nil, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT rl.ts").
		WithArgs(nil, nil, nil, nil, nil, nil, nil, 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"ts", "tenant_id", "api_key_id", "api_key_name", "request_id", "model", "provider", "status", "latency_ms", "cost_usd", "prompt_tokens", "completion_tokens", "total_tokens"}))

	rows, total, err := store.ListAPIKeyRawUsage(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Errorf("total=%d len(rows)=%d", total, len(rows))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_GetJWTSubUsage_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()
	store := NewPostgresFromDB(db, testLogger())
	filter := JWTSubUsageFilter{Limit: 50, Offset: 0, SortBy: "cost_usd", SortOrder: "desc"}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM \\(SELECT jwt_sub, tenant_id FROM usage WHERE").
		WithArgs(nil, nil, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT jwt_sub").
		WithArgs(nil, nil, nil, nil, nil, 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"jwt_sub", "requests", "prompt_tokens", "completion_tokens", "total_tokens", "total_cost_usd", "first_seen", "last_seen"}))

	rows, total, err := store.GetJWTSubUsage(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Errorf("total=%d len(rows)=%d", total, len(rows))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_GetJWTSubUsageDetail_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()
	store := NewPostgresFromDB(db, testLogger())
	filter := JWTSubUsageDetailFilter{GroupBy: "model"}
	jwtSub := "sub-123"

	mock.ExpectQuery("SELECT\\s+COUNT\\(\\*\\)::int").
		WithArgs(jwtSub, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"requests", "prompt_tokens", "completion_tokens", "total_tokens", "total_cost_usd"}).AddRow(0, 0, 0, 0, 0.0))
	mock.ExpectQuery("SELECT model AS group_label").
		WithArgs(jwtSub, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"group_label", "requests", "total_tokens", "total_cost_usd"}))

	summary, breakdown, err := store.GetJWTSubUsageDetail(context.Background(), jwtSub, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Requests != 0 || len(breakdown) != 0 {
		t.Errorf("summary requests=%d breakdown=%d", summary.Requests, len(breakdown))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_ListJWTSubRawUsage_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()
	store := NewPostgresFromDB(db, testLogger())
	filter := JWTSubRawUsageFilter{Limit: 50, Offset: 0}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM request_log r INNER JOIN usage u ON").
		WithArgs(nil, nil, nil, nil, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT r.ts").
		WithArgs(nil, nil, nil, nil, nil, nil, nil, 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"ts", "tenant_id", "jwt_sub", "request_id", "model", "provider", "status", "latency_ms", "cost_usd", "prompt_tokens", "completion_tokens", "total_tokens"}))

	rows, total, err := store.ListJWTSubRawUsage(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Errorf("total=%d len(rows)=%d", total, len(rows))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_CheckAndReserveBudget_UnderLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	monthStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(cost_usd\\), 0\\) FROM usage").
		WithArgs("tenant_a", monthStart, monthEnd).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(5.0))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(estimated_cost_usd\\), 0\\) FROM budget_reservations").
		WithArgs("tenant_a", monthStart, monthEnd).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(1.0))
	mock.ExpectExec("INSERT INTO budget_reservations").
		WithArgs(sqlmock.AnyArg(), "tenant_a", 0.01).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	check, err := store.CheckAndReserveBudget(context.Background(), "tenant_a", monthStart, monthEnd, 100.0, 0.01)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if check.MonthSpendUSD != 6.0 {
		t.Errorf("expected month spend 6.0, got %f", check.MonthSpendUSD)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_CheckAndReserveBudget_Exceeded(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	monthStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(cost_usd\\), 0\\) FROM usage").
		WithArgs("tenant_a", monthStart, monthEnd).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(95.0))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(estimated_cost_usd\\), 0\\) FROM budget_reservations").
		WithArgs("tenant_a", monthStart, monthEnd).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(5.5))
	mock.ExpectRollback()

	check, err := store.CheckAndReserveBudget(context.Background(), "tenant_a", monthStart, monthEnd, 100.0, 0.01)
	if err != ErrBudgetExceeded {
		t.Fatalf("expected ErrBudgetExceeded, got: %v", err)
	}
	if check.MonthSpendUSD != 100.5 {
		t.Errorf("expected month spend 100.5, got %f", check.MonthSpendUSD)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_CheckAndReserveBudget_DBFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	monthStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin().WillReturnError(fmt.Errorf("connection refused"))

	_, err = store.CheckAndReserveBudget(context.Background(), "tenant_a", monthStart, monthEnd, 100.0, 0.01)
	if err == nil {
		t.Fatal("expected error on DB failure")
	}
	if err == ErrBudgetExceeded {
		t.Fatal("should not be ErrBudgetExceeded on DB failure")
	}
}

func TestNopStorage(t *testing.T) {
	store := NopStorage{}

	if err := store.LogRequest(context.Background(), RequestLog{
		ID:        uuid.New(),
		RequestID: uuid.New().String(),
		Attempt:   1,
	}); err != nil {
		t.Errorf("NopStorage.LogRequest should not error: %v", err)
	}
	if err := store.SaveUsage(context.Background(), UsageRecord{}); err != nil {
		t.Errorf("NopStorage.SaveUsage should not error: %v", err)
	}
	if _, err := store.CheckAndReserveBudget(context.Background(), "t", time.Now(), time.Now(), 100, 0.01); err != nil {
		t.Errorf("NopStorage.CheckAndReserveBudget should not error: %v", err)
	}
}

func TestPostgres_GetSmartImpactData(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	requestID1 := uuid.New()
	requestID2 := uuid.New()

	// Mock query result
	rows := sqlmock.NewRows([]string{
		"request_id", "ts", "tenant_id", "model",
		"prompt_tokens", "completion_tokens", "cost_usd",
		"status", "latency_ms",
	}).
		AddRow(
			requestID1, from, "t1", "gpt-4o-mini",
			100, 50, 0.01, "ok", 150,
		).
		AddRow(
			requestID2, from.Add(time.Hour), "t1", "claude-3-5-sonnet",
			200, 100, 0.05, "ok", 300,
		)

	mock.ExpectQuery("SELECT.*FROM usage u.*JOIN request_log rl").
		WithArgs("t1", from, to).
		WillReturnRows(rows)

	data, err := store.GetSmartImpactData(context.Background(), "t1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.TenantID != "t1" {
		t.Errorf("expected tenant_id 't1', got '%s'", data.TenantID)
	}

	if data.TotalRequests != 2 {
		t.Errorf("expected 2 requests, got %d", data.TotalRequests)
	}

	expectedCost := 0.06
	if data.TotalCostUSD < expectedCost-0.0001 || data.TotalCostUSD > expectedCost+0.0001 {
		t.Errorf("expected ~0.06 cost, got %f", data.TotalCostUSD)
	}

	if data.SuccessRequests != 2 {
		t.Errorf("expected 2 successes, got %d", data.SuccessRequests)
	}

	if data.ErrorRequests != 0 {
		t.Errorf("expected 0 errors, got %d", data.ErrorRequests)
	}

	expectedLatency := (150.0 + 300.0) / 2.0
	if data.AvgLatencyMs != expectedLatency {
		t.Errorf("expected avg latency %f, got %f", expectedLatency, data.AvgLatencyMs)
	}

	if len(data.UsageDetails) != 2 {
		t.Errorf("expected 2 usage details, got %d", len(data.UsageDetails))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_GetSmartImpactData_WithErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	requestID1 := uuid.New()
	requestID2 := uuid.New()

	// Mock query result with one error
	rows := sqlmock.NewRows([]string{
		"request_id", "ts", "tenant_id", "model",
		"prompt_tokens", "completion_tokens", "cost_usd",
		"status", "latency_ms",
	}).
		AddRow(
			requestID1, from, "t1", "gpt-4o-mini",
			100, 50, 0.01, "ok", 150,
		).
		AddRow(
			requestID2, from.Add(time.Hour), "t1", "claude-3-5-sonnet",
			200, 100, 0.05, "error", 0,
		)

	mock.ExpectQuery("SELECT.*FROM usage u.*JOIN request_log rl").
		WithArgs("t1", from, to).
		WillReturnRows(rows)

	data, err := store.GetSmartImpactData(context.Background(), "t1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.TotalRequests != 2 {
		t.Errorf("expected 2 requests, got %d", data.TotalRequests)
	}

	if data.SuccessRequests != 1 {
		t.Errorf("expected 1 success, got %d", data.SuccessRequests)
	}

	if data.ErrorRequests != 1 {
		t.Errorf("expected 1 error, got %d", data.ErrorRequests)
	}

	// Average latency should only count successful requests
	if data.AvgLatencyMs != 150.0 {
		t.Errorf("expected avg latency 150.0, got %f", data.AvgLatencyMs)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_GetSmartImpactData_NoData(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// Mock empty query result
	rows := sqlmock.NewRows([]string{
		"request_id", "ts", "tenant_id", "model",
		"prompt_tokens", "completion_tokens", "cost_usd",
		"status", "latency_ms",
	})

	mock.ExpectQuery("SELECT.*FROM usage u.*JOIN request_log rl").
		WithArgs("t1", from, to).
		WillReturnRows(rows)

	data, err := store.GetSmartImpactData(context.Background(), "t1", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.TotalRequests != 0 {
		t.Errorf("expected 0 requests, got %d", data.TotalRequests)
	}

	if data.AvgLatencyMs != 0 {
		t.Errorf("expected 0 avg latency, got %f", data.AvgLatencyMs)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_GetAuditRecords(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	requestID := uuid.New()
	allowDecision := "allow"
	modifyDecision := "modify"

	rows := sqlmock.NewRows([]string{
		"id", "ts", "tenant_id", "model", "provider", "strategy",
		"status", "latency_ms", "fallback_used",
		"pii_webhook_request_decision", "pii_webhook_response_decision",
		"prompt_tokens", "completion_tokens", "total_tokens", "cost_usd",
	}).AddRow(
		requestID, from, "tenant_a", "gpt-4o-mini", "openai", "smart",
		"ok", 150, false, &allowDecision, &modifyDecision,
		100, 50, 150, 0.01,
	)

	mock.ExpectQuery("SELECT.*FROM request_log rl.*LEFT JOIN usage u").
		WithArgs("tenant_a", from, to).
		WillReturnRows(rows)

	records, err := store.GetAuditRecords(context.Background(), "tenant_a", from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	r := records[0]
	if r.RequestID != requestID {
		t.Errorf("expected request_id %v, got %v", requestID, r.RequestID)
	}
	if r.TenantID != "tenant_a" {
		t.Errorf("expected tenant_id 'tenant_a', got '%s'", r.TenantID)
	}
	if r.PIIWebhookRequestDecision == nil || *r.PIIWebhookRequestDecision != "allow" {
		t.Errorf("expected pii_webhook_request_decision 'allow', got %v", r.PIIWebhookRequestDecision)
	}
	if r.PIIWebhookResponseDecision == nil || *r.PIIWebhookResponseDecision != "modify" {
		t.Errorf("expected pii_webhook_response_decision 'modify', got %v", r.PIIWebhookResponseDecision)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPostgres_GetAuditRecords_90DayLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC) // > 90 days

	// Should not execute query, should return error immediately
	_, err = store.GetAuditRecords(context.Background(), "tenant_a", from, to)
	if err == nil {
		t.Fatal("expected error for >90 day window")
	}
	if err.Error() != "audit window cannot exceed 90 days" {
		t.Errorf("unexpected error message: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestPostgres_PutTenantConfig_VersionedPath verifies that PutTenantConfig writes
// correctly to the versioned tables (tenant_config_versions + tenant_active_config)
// when the tenant's config is NOT present in the flat tenants_config table.
// This is the scenario for tenants seeded via SeedTenantVersionedConfig.
func TestPostgres_PutTenantConfig_VersionedPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	newConfigJSON := json.RawMessage(`{"id":"tenant_a","routing":{"semantic":{"threshold_default":0.7}}}`)
	const ifMatchVersion = 1

	mock.ExpectBegin()

	// Step 1: versioned path SELECT — found at version 1 (flat tenants_config is NOT queried)
	mock.ExpectQuery("SELECT tcv.version").
		WithArgs("tenant_a").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))

	// Step 2: INSERT new tenant_config_versions row → returns new UUID
	newUUID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	mock.ExpectQuery("INSERT INTO tenant_config_versions").
		WithArgs("tenant_a", 2, sqlmock.AnyArg(), sqlmock.AnyArg(), "admin").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(newUUID))

	// Step 4: UPDATE tenant_active_config pointer
	mock.ExpectExec("UPDATE tenant_active_config").
		WithArgs(newUUID, "admin", sqlmock.AnyArg(), "tenant_a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Step 5: INSERT config_change_log
	mock.ExpectExec("INSERT INTO config_change_log").
		WithArgs("tenant_a", "admin", sqlmock.AnyArg(), 1, 2, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	newVersion, err := store.PutTenantConfig(
		context.Background(), "tenant_a", ifMatchVersion,
		newConfigJSON, "admin", []string{"admin"}, "update threshold", nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newVersion != 2 {
		t.Errorf("expected new version 2, got %d", newVersion)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestPostgres_PutTenantConfig_VersionedPath_Conflict verifies that a version mismatch
// on the versioned path surfaces as ErrVersionConflict (not a crash or wrong error).
func TestPostgres_PutTenantConfig_VersionedPath_Conflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	mock.ExpectBegin()

	// Step 1: versioned path SELECT — current version is 3, but caller expects 1
	// (flat tenants_config is NOT queried; versioned path wins)
	mock.ExpectQuery("SELECT tcv.version").
		WithArgs("tenant_a").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))

	mock.ExpectRollback()

	_, err = store.PutTenantConfig(
		context.Background(), "tenant_a", 1, // stale ifMatchVersion
		json.RawMessage(`{}`), "admin", []string{"admin"}, "patch", nil,
	)
	if err == nil {
		t.Fatal("expected ErrVersionConflict, got nil")
	}
	conflict, ok := err.(ErrVersionConflict)
	if !ok {
		t.Fatalf("expected ErrVersionConflict, got %T: %v", err, err)
	}
	if conflict.Expected != 1 || conflict.Current != 3 {
		t.Errorf("conflict fields: expected {1, 3}, got {%d, %d}", conflict.Expected, conflict.Current)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestPostgres_PutTenantConfig_BothPaths_VersionedWins verifies that when a tenant
// exists in BOTH tenants_config (stale flat row, version=0) AND tenant_active_config
// (authoritative versioned row, version=1), the versioned path wins and the stale
// flat row does NOT cause ErrVersionConflict.
func TestPostgres_PutTenantConfig_BothPaths_VersionedWins(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	newConfigJSON := json.RawMessage(`{"id":"tenant_a"}`)
	const ifMatchVersion = 1 // matches the versioned path version

	mock.ExpectBegin()

	// Step 1: versioned path SELECT — found at version 1.
	// The stale tenants_config row (version=0) is intentionally ignored.
	mock.ExpectQuery("SELECT tcv.version").
		WithArgs("tenant_a").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))

	// Flat tenants_config must NOT be queried.

	// Step 2: INSERT new tenant_config_versions row
	newUUID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	mock.ExpectQuery("INSERT INTO tenant_config_versions").
		WithArgs("tenant_a", 2, sqlmock.AnyArg(), sqlmock.AnyArg(), "admin").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(newUUID))

	// Step 3: UPDATE tenant_active_config pointer
	mock.ExpectExec("UPDATE tenant_active_config").
		WithArgs(newUUID, "admin", sqlmock.AnyArg(), "tenant_a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Step 4: INSERT config_change_log
	mock.ExpectExec("INSERT INTO config_change_log").
		WithArgs("tenant_a", "admin", sqlmock.AnyArg(), 1, 2, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	newVersion, err := store.PutTenantConfig(
		context.Background(), "tenant_a", ifMatchVersion,
		newConfigJSON, "admin", []string{"admin"}, "update config", nil,
	)
	if err != nil {
		t.Fatalf("expected no error (versioned path must win over stale flat row), got: %v", err)
	}
	if newVersion != 2 {
		t.Errorf("expected new version 2, got %d", newVersion)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (flat tenants_config must NOT have been queried): %v", err)
	}
}

func TestPostgres_DeleteOldRecords(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create mock: %v", err)
	}
	defer db.Close()

	store := NewPostgresFromDB(db, testLogger())

	cutoffDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	// Expect delete from usage
	mock.ExpectExec("DELETE FROM usage").
		WithArgs("tenant_a", cutoffDate).
		WillReturnResult(sqlmock.NewResult(0, 10))

	// Expect delete from model_stats_daily
	mock.ExpectExec("DELETE FROM model_stats_daily").
		WithArgs("tenant_a", cutoffDate).
		WillReturnResult(sqlmock.NewResult(0, 5))

	// Expect delete from request_log
	mock.ExpectExec("DELETE FROM request_log").
		WithArgs("tenant_a", cutoffDate).
		WillReturnResult(sqlmock.NewResult(0, 10))

	deleted, err := store.DeleteOldRecords(context.Background(), "tenant_a", cutoffDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deleted != 10 {
		t.Errorf("expected 10 deleted records, got %d", deleted)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
