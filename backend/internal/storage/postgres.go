package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/encryption"
	gatewayotel "github.com/diegomcastronuovo/prism-gateway/internal/otel"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// jsonbText converts a json.RawMessage to a *string suitable for pgx v5 via
// database/sql. pgx sends []byte as bytea wire type; PostgreSQL cannot implicitly
// cast bytea → jsonb. Sending as a string lets the server use its text → jsonb
// implicit cast path, which always works for well-formed JSON.
// A nil input stays nil (→ SQL NULL).
func jsonbText(v json.RawMessage) interface{} {
	if v == nil {
		return nil
	}
	s := string(v)
	return &s
}

// PostgresStorage implements Storage using a Postgres database.
type PostgresStorage struct {
	db         *sql.DB
	log        *slog.Logger
	encService encryption.FieldEncryptionService
}

// NewPostgres opens a connection to the given DSN and returns a PostgresStorage.
func NewPostgres(ctx context.Context, dsn string, maxOpen, maxIdle int, log *slog.Logger) (*PostgresStorage, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	encService, encErr := encryption.NewFieldEncryptionServiceFromEnv()
	if encErr != nil {
		if !errors.Is(encErr, encryption.ErrEncryptionNotConfigured) {
			log.Error("failed to configure field encryption", "error", encErr)
		} else {
			log.Warn("field encryption not configured")
		}
	}
	s := &PostgresStorage{db: db, log: log, encService: encService}
	if err := s.runMigrations(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return s, nil
}

// NewPostgresFromDB creates a PostgresStorage from an existing *sql.DB (for testing).
func NewPostgresFromDB(db *sql.DB, log *slog.Logger) *PostgresStorage {
	encService, encErr := encryption.NewFieldEncryptionServiceFromEnv()
	if encErr != nil {
		if !errors.Is(encErr, encryption.ErrEncryptionNotConfigured) {
			log.Error("failed to configure field encryption", "error", encErr)
		} else {
			log.Warn("field encryption not configured")
		}
	}
	return &PostgresStorage{db: db, log: log, encService: encService}
}

// DB returns the underlying *sql.DB.
func (s *PostgresStorage) DB() *sql.DB {
	return s.db
}

// Close closes the database connection.
func (s *PostgresStorage) Close() error {
	return s.db.Close()
}

// EncryptionConfigured reports whether field-level encryption is available.
// Returns false when LOG_ENC_KEY_V* env vars are not set.
func (s *PostgresStorage) EncryptionConfigured() bool {
	return s.encService != nil
}

func (s *PostgresStorage) runMigrations(ctx context.Context) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("execute migration %s: %w", entry.Name(), err)
		}
		s.log.Info("migration applied", "file", entry.Name())
	}
	return nil
}

func (s *PostgresStorage) LogRequest(ctx context.Context, rl RequestLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO request_log
		 (id, request_id, attempt, tenant_id, model, provider, strategy, status, latency_ms, error, fallback_used,
		  pii_webhook_request_decision, pii_webhook_response_decision,
		  decision_reason, error_type, decision_snapshot, metadata, routing_snapshot,
		  router_pre_ms, llm_latency_ms, router_post_ms,
		  pre_decode_ms, pre_authz_ms, pre_tenant_config_ms, pre_pii_ms, pre_rate_limit_ms, pre_model_filter_ms, pre_routing_ms, pre_request_build_ms,
		  cfg_tool_routes_ms, cfg_dynamic_routes_ms, cfg_budget_pressure_ms, cfg_semantic_ms, cfg_model_resolution_ms,
		  tool_routes_embedding_model_ms, tool_routes_embedding_generate_ms, tool_routes_semantic_db_ms, tool_routes_match_eval_ms,
		  api_key_id, api_key_name, jwt_sub,
		  customer_id, channel, interaction_type, agent_id, department, ticket_id, customer_segment, language,
		  intent, experiment_id, autonomy_level, policy_id, risk_level, revenue_impact, currency,
		  cached_tokens, tool_cost_usd)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46, $47, $48, $49, $50, $51, $52, $53, $54, $55, $56, $57, $58, $59)`,
		rl.ID, rl.RequestID, rl.Attempt, rl.TenantID, rl.Model, rl.Provider, rl.Strategy, rl.Status,
		rl.LatencyMs, rl.Error, rl.FallbackUsed,
		rl.PIIWebhookRequestDecision, rl.PIIWebhookResponseDecision,
		rl.DecisionReason, rl.ErrorType, jsonbText(rl.DecisionSnapshot), jsonbText(rl.Metadata), jsonbText(rl.RoutingSnapshot),
		rl.RouterPreMS, rl.LLMLatencyMS, rl.RouterPostMS,
		rl.PreDecodeMS, rl.PreAuthzMS, rl.PreTenantConfigMS, rl.PrePIIMS, rl.PreRateLimitMS, rl.PreModelFilterMS, rl.PreRoutingMS, rl.PreRequestBuildMS,
		rl.CfgToolRoutesMS, rl.CfgDynamicRoutesMS, rl.CfgBudgetPressureMS, rl.CfgSemanticMS, rl.CfgModelResolutionMS,
		rl.ToolRoutesEmbeddingModelMS, rl.ToolRoutesEmbeddingGenerateMS, rl.ToolRoutesSemanticDBMS, rl.ToolRoutesMatchEvalMS,
		rl.APIKeyID, rl.APIKeyName, rl.JWTSub,
		rl.CustomerID, rl.Channel, rl.InteractionType, rl.AgentID, rl.Department, rl.TicketID, rl.CustomerSegment, rl.Language,
		rl.Intent, rl.ExperimentID, rl.AutonomyLevel, rl.PolicyID, rl.RiskLevel, rl.RevenueImpact, rl.Currency,
		rl.CachedTokens, rl.ToolCostUSD,
	)
	return err
}

func (s *PostgresStorage) LogConversation(ctx context.Context, row ConversationLog) error {
	if s.encService == nil {
		gatewayotel.ConversationLogDroppedTotal.Inc()
		return fmt.Errorf("conversation log encryption not configured")
	}

	keyVersion := ""
	encryptField := func(value *string) ([]byte, error) {
		if value == nil || *value == "" {
			return nil, nil
		}
		ciphertext, version, err := s.encService.EncryptString(*value)
		if err != nil {
			return nil, err
		}
		if keyVersion == "" {
			keyVersion = version
		}
		return ciphertext, nil
	}

	var err error
	row.PromptRedactedEnc, err = encryptField(row.PromptRedacted)
	if err != nil {
		return fmt.Errorf("encrypt prompt_redacted: %w", err)
	}
	row.ResponseRedactedEnc, err = encryptField(row.ResponseRedacted)
	if err != nil {
		return fmt.Errorf("encrypt response_redacted: %w", err)
	}
	row.PromptFullEnc, err = encryptField(row.PromptFull)
	if err != nil {
		return fmt.Errorf("encrypt prompt_full: %w", err)
	}
	row.ResponseFullEnc, err = encryptField(row.ResponseFull)
	if err != nil {
		return fmt.Errorf("encrypt response_full: %w", err)
	}
	if keyVersion == "" {
		keyVersion = s.encService.ActiveKeyVersion()
	}
	row.EncKeyVersion = keyVersion

	row.PromptRedacted = nil
	row.ResponseRedacted = nil
	row.PromptFull = nil
	row.ResponseFull = nil

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO conversation_log
		(id, request_id, tenant_id, jwt_sub, workflow_id, conversation_id, customer_id, prompt_preview, response_preview, prompt_redacted, response_redacted, prompt_full, response_full, pii_detected, logging_mode,
		 enc_key_version, prompt_redacted_enc, response_redacted_enc, prompt_full_enc, response_full_enc)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		row.ID, row.RequestID, row.TenantID, row.JWTSub, row.WorkflowID, row.ConversationID, row.CustomerID, row.PromptPreview, row.ResponsePreview, row.PromptRedacted, row.ResponseRedacted, row.PromptFull, row.ResponseFull, row.PIIDetected, row.LoggingMode,
		row.EncKeyVersion, row.PromptRedactedEnc, row.ResponseRedactedEnc, row.PromptFullEnc, row.ResponseFullEnc,
	)
	return err
}

func (s *PostgresStorage) LogComplianceEvent(ctx context.Context, ev ComplianceEventLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO compliance_event_log
		 (id, tenant_id, request_id, event_type, action_taken, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ev.ID, ev.TenantID, ev.RequestID, ev.EventType, ev.ActionTaken, ev.Metadata,
	)
	return err
}

func (s *PostgresStorage) GetRoutingSnapshot(ctx context.Context, requestID string) (string, json.RawMessage, bool, error) {
	var tenantID string
	var snapshot json.RawMessage
	err := s.db.QueryRowContext(ctx,
		`SELECT tenant_id, routing_snapshot
		 FROM request_log
		 WHERE request_id = $1
		   AND status = 'ok'
		   AND routing_snapshot IS NOT NULL
		 ORDER BY attempt ASC
		 LIMIT 1`,
		requestID,
	).Scan(&tenantID, &snapshot)
	if err == sql.ErrNoRows {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("get routing snapshot: %w", err)
	}
	return tenantID, snapshot, true, nil
}

func (s *PostgresStorage) GetReplayDiagnostics(ctx context.Context, requestID string) (ReplayDiagnostics, bool, error) {
	var d ReplayDiagnostics
	var decisionReason, decisionSnapshot sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT tenant_id, routing_snapshot, decision_reason, decision_snapshot, strategy
		 FROM request_log
		 WHERE request_id = $1 AND status = 'ok' AND routing_snapshot IS NOT NULL
		 ORDER BY attempt ASC
		 LIMIT 1`,
		requestID,
	).Scan(&d.TenantID, &d.RoutingSnapshot, &decisionReason, &decisionSnapshot, &d.Strategy)
	if err == sql.ErrNoRows {
		return ReplayDiagnostics{}, false, nil
	}
	if err != nil {
		return ReplayDiagnostics{}, false, fmt.Errorf("get replay diagnostics: %w", err)
	}
	if decisionReason.Valid {
		d.DecisionReason = &decisionReason.String
	}
	// decision_snapshot is JSONB; driver may return as string or []byte
	if decisionSnapshot.Valid && decisionSnapshot.String != "" {
		d.DecisionSnapshot = json.RawMessage(decisionSnapshot.String)
	}
	return d, true, nil
}

func (s *PostgresStorage) SaveUsage(ctx context.Context, u UsageRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO usage (id, tenant_id, model, provider, prompt_tokens, completion_tokens, total_tokens, cost_usd, request_id, api_key_id, api_key_name, jwt_sub)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		u.ID, u.TenantID, u.Model, u.Provider, u.PromptTokens, u.CompletionTokens, u.TotalTokens, u.CostUSD, u.RequestID,
		u.APIKeyID, u.APIKeyName, u.JWTSub,
	)
	return err
}

// CheckAndReserveBudget atomically checks the tenant's monthly spend and reserves
// budget for the current request using a Postgres advisory lock keyed by tenant_id.
//
// Inside a transaction:
//  1. Acquire pg_advisory_xact_lock(hash(tenant_id)) — blocks concurrent budget checks for same tenant
//  2. SUM(cost_usd) from usage WHERE tenant_id AND ts in [monthStart, monthEnd)
//  3. SUM(estimated_cost_usd) from budget_reservations WHERE tenant_id AND ts in [monthStart, monthEnd)
//  4. If total + estimatedCost > limitUSD → rollback, return ErrBudgetExceeded
//  5. Otherwise insert reservation row and commit
func (s *PostgresStorage) CheckAndReserveBudget(ctx context.Context, tenantID string, monthStart, monthEnd time.Time, limitUSD, estimatedCost float64) (BudgetCheck, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BudgetCheck{}, fmt.Errorf("begin budget tx: %w", err)
	}
	defer tx.Rollback()

	// Advisory lock keyed by tenant_id hash — serializes budget checks per tenant
	lockKey := tenantLockKey(tenantID)
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		return BudgetCheck{}, fmt.Errorf("advisory lock: %w", err)
	}

	// Sum confirmed usage
	var usageSpend sql.NullFloat64
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) FROM usage WHERE tenant_id = $1 AND ts >= $2 AND ts < $3`,
		tenantID, monthStart, monthEnd,
	).Scan(&usageSpend)
	if err != nil {
		return BudgetCheck{}, fmt.Errorf("query usage spend: %w", err)
	}

	// Sum pending reservations
	var reservedSpend sql.NullFloat64
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM budget_reservations WHERE tenant_id = $1 AND ts >= $2 AND ts < $3 AND expires_at > NOW()`,
		tenantID, monthStart, monthEnd,
	).Scan(&reservedSpend)
	if err != nil {
		return BudgetCheck{}, fmt.Errorf("query reserved spend: %w", err)
	}

	totalSpend := usageSpend.Float64 + reservedSpend.Float64
	check := BudgetCheck{MonthSpendUSD: totalSpend}

	if totalSpend+estimatedCost > limitUSD {
		return check, ErrBudgetExceeded
	}

	// Insert reservation — capture the UUID so the caller can release it later.
	reservationID := uuid.New()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO budget_reservations (id, tenant_id, estimated_cost_usd) VALUES ($1, $2, $3)`,
		reservationID, tenantID, estimatedCost,
	)
	if err != nil {
		return check, fmt.Errorf("insert reservation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return check, fmt.Errorf("commit budget tx: %w", err)
	}

	check.ReservationID = reservationID
	return check, nil
}

// ReleaseReservation deletes a single budget_reservations row by primary key.
// Fail-safe: returns nil if the row is not found (idempotent).
func (s *PostgresStorage) ReleaseReservation(ctx context.Context, reservationID uuid.UUID) error {
	if reservationID == uuid.Nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM budget_reservations WHERE id = $1`,
		reservationID,
	)
	return err
}

// PurgeExpiredReservations deletes all budget_reservations rows past their expires_at.
func (s *PostgresStorage) PurgeExpiredReservations(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM budget_reservations WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("purge expired reservations: %w", err)
	}
	return res.RowsAffected()
}

// GetMonthlyReservedSpend returns the sum of estimated_cost_usd across all in-flight
// budget_reservations rows for a tenant within the given month window.
func (s *PostgresStorage) GetMonthlyReservedSpend(ctx context.Context, tenantID string, monthStart, monthEnd time.Time) (float64, error) {
	var reserved float64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(estimated_cost_usd), 0) FROM budget_reservations WHERE tenant_id = $1 AND ts >= $2 AND ts < $3 AND expires_at > NOW()`,
		tenantID, monthStart, monthEnd,
	).Scan(&reserved)
	if err != nil {
		return 0, err
	}
	return reserved, nil
}

// tenantLockKey produces a stable int64 hash from the tenant ID for use as advisory lock key.
func tenantLockKey(tenantID string) int64 {
	h := fnv.New64a()
	h.Write([]byte(tenantID))
	return int64(h.Sum64())
}

// UpsertModelStatDaily atomically updates or inserts daily model statistics.
func (s *PostgresStorage) UpsertModelStatDaily(ctx context.Context, stat ModelStatDaily) error {
	query := `
		INSERT INTO model_stats_daily
			(date, tenant_id, model, request_count, success_count, error_count, avg_latency_ms, total_cost_usd, updated_at)
		VALUES
			($1, $2, $3, 1, $4, $5, $6, $7, NOW())
		ON CONFLICT (date, tenant_id, model)
		DO UPDATE SET
			request_count = model_stats_daily.request_count + 1,
			success_count = model_stats_daily.success_count + EXCLUDED.success_count,
			error_count = model_stats_daily.error_count + EXCLUDED.error_count,
			avg_latency_ms = (
				(model_stats_daily.avg_latency_ms * model_stats_daily.request_count + EXCLUDED.avg_latency_ms)
				/ (model_stats_daily.request_count + 1)
			),
			total_cost_usd = model_stats_daily.total_cost_usd + EXCLUDED.total_cost_usd,
			updated_at = NOW()
	`

	successCount := 0
	errorCount := 0
	if stat.SuccessCount > 0 {
		successCount = 1
	} else {
		errorCount = 1
	}

	_, err := s.db.ExecContext(ctx, query,
		stat.Date, stat.TenantID, stat.Model,
		successCount, errorCount, stat.AvgLatencyMs, stat.TotalCostUSD,
	)
	return err
}

// GetModelStats retrieves daily model statistics for a tenant within the specified window.
func (s *PostgresStorage) GetModelStats(ctx context.Context, tenantID string, windowDays int) ([]ModelStatDaily, error) {
	cutoffDate := time.Now().UTC().AddDate(0, 0, -windowDays)

	query := `
		SELECT date, tenant_id, model, request_count, success_count, error_count,
		       avg_latency_ms, total_cost_usd
		FROM model_stats_daily
		WHERE tenant_id = $1 AND date >= $2
		ORDER BY date DESC, model ASC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, cutoffDate)
	if err != nil {
		return nil, fmt.Errorf("query model stats: %w", err)
	}
	defer rows.Close()

	var stats []ModelStatDaily
	for rows.Next() {
		var stat ModelStatDaily
		err := rows.Scan(&stat.Date, &stat.TenantID, &stat.Model,
			&stat.RequestCount, &stat.SuccessCount, &stat.ErrorCount,
			&stat.AvgLatencyMs, &stat.TotalCostUSD)
		if err != nil {
			return nil, fmt.Errorf("scan model stat: %w", err)
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// GetUsageSummary returns monthly usage aggregates for a tenant.
func (s *PostgresStorage) GetUsageSummary(ctx context.Context, tenantID string, month time.Time) (UsageSummary, error) {
	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)

	query := `
		SELECT model, COUNT(*) as requests, COALESCE(SUM(cost_usd), 0) as cost
		FROM usage
		WHERE tenant_id = $1 AND ts >= $2 AND ts < $3
		GROUP BY model
		ORDER BY cost DESC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, monthStart, monthEnd)
	if err != nil {
		return UsageSummary{}, fmt.Errorf("query usage summary: %w", err)
	}
	defer rows.Close()

	summary := UsageSummary{
		TenantID:       tenantID,
		Month:          month.Format("2006-01"),
		ModelBreakdown: make(map[string]ModelUsage),
	}

	for rows.Next() {
		var model string
		var requests int
		var cost float64
		if err := rows.Scan(&model, &requests, &cost); err != nil {
			return summary, fmt.Errorf("scan usage: %w", err)
		}

		summary.TotalRequests += requests
		summary.TotalCost += cost
		summary.ModelBreakdown[model] = ModelUsage{Requests: requests, Cost: cost}
	}

	return summary, rows.Err()
}

// GetBudgetForecast calculates projected spending based on current month usage.
func (s *PostgresStorage) GetBudgetForecast(ctx context.Context, tenantID string, month time.Time, budgetLimit float64) (BudgetForecast, error) {
	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)

	var currentSpend float64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) FROM usage WHERE tenant_id = $1 AND ts >= $2 AND ts < $3`,
		tenantID, monthStart, monthEnd,
	).Scan(&currentSpend)
	if err != nil {
		return BudgetForecast{}, fmt.Errorf("query current spend: %w", err)
	}

	now := time.Now().UTC()
	daysElapsed := now.Day()
	daysInMonth := monthEnd.Sub(monthStart).Hours() / 24

	var projectedSpend float64
	if daysElapsed > 0 {
		dailyRate := currentSpend / float64(daysElapsed)
		projectedSpend = dailyRate * daysInMonth
	}

	return BudgetForecast{
		TenantID:       tenantID,
		Month:          month.Format("2006-01"),
		CurrentSpend:   currentSpend,
		ProjectedSpend: projectedSpend,
		DaysElapsed:    daysElapsed,
		DaysInMonth:    int(daysInMonth),
		IsOverBudget:   projectedSpend > budgetLimit,
		BudgetLimit:    budgetLimit,
	}, nil
}

// GetSmartImpactData retrieves detailed usage data for ROI calculation.
// Joins usage table with request_log to get per-request token counts and status.
func (s *PostgresStorage) GetSmartImpactData(ctx context.Context, tenantID string, from, to time.Time) (SmartImpactData, error) {
	query := `
		SELECT
			u.request_id,
			u.ts,
			u.tenant_id,
			u.model,
			u.prompt_tokens,
			u.completion_tokens,
			u.cost_usd,
			rl.status,
			rl.latency_ms
		FROM usage u
		INNER JOIN request_log rl ON u.request_id = rl.id::TEXT
		WHERE u.tenant_id = $1 AND u.ts >= $2 AND u.ts < $3
		ORDER BY u.ts ASC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, from, to)
	if err != nil {
		return SmartImpactData{}, fmt.Errorf("query smart impact data: %w", err)
	}
	defer rows.Close()

	data := SmartImpactData{
		TenantID:     tenantID,
		PeriodStart:  from,
		PeriodEnd:    to,
		UsageDetails: make([]UsageDetailRow, 0),
	}

	var totalLatency int64

	for rows.Next() {
		var row UsageDetailRow
		err := rows.Scan(
			&row.RequestID,
			&row.Timestamp,
			&row.TenantID,
			&row.Model,
			&row.PromptTokens,
			&row.CompletionTokens,
			&row.CostUSD,
			&row.Status,
			&row.LatencyMs,
		)
		if err != nil {
			return data, fmt.Errorf("scan usage detail: %w", err)
		}

		data.UsageDetails = append(data.UsageDetails, row)
		data.TotalRequests++
		data.TotalCostUSD += row.CostUSD

		if row.Status == "ok" {
			data.SuccessRequests++
			totalLatency += int64(row.LatencyMs)
		} else {
			data.ErrorRequests++
		}
	}

	if err := rows.Err(); err != nil {
		return data, fmt.Errorf("rows iteration: %w", err)
	}

	// Calculate average latency (only for successful requests)
	if data.SuccessRequests > 0 {
		data.AvgLatencyMs = float64(totalLatency) / float64(data.SuccessRequests)
	}

	return data, nil
}

// GetAuditRecords retrieves audit trail for export (90-day max window).
func (s *PostgresStorage) GetAuditRecords(ctx context.Context, tenantID string, from, to time.Time) ([]AuditRecord, error) {
	// Enforce 90-day max window
	maxWindow := 90 * 24 * time.Hour
	if to.Sub(from) > maxWindow {
		return nil, fmt.Errorf("audit window cannot exceed 90 days")
	}

	query := `
		SELECT
			rl.id, rl.ts, rl.tenant_id, rl.model, rl.provider, rl.strategy,
			rl.status, rl.latency_ms, rl.fallback_used,
			rl.pii_webhook_request_decision, rl.pii_webhook_response_decision,
			COALESCE(u.prompt_tokens, 0) as prompt_tokens,
			COALESCE(u.completion_tokens, 0) as completion_tokens,
			COALESCE(u.total_tokens, 0) as total_tokens,
			COALESCE(u.cost_usd, 0) as cost_usd
		FROM request_log rl
		LEFT JOIN usage u ON rl.id::TEXT = u.request_id
		WHERE rl.tenant_id = $1 AND rl.ts >= $2 AND rl.ts < $3
		ORDER BY rl.ts DESC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query audit records: %w", err)
	}
	defer rows.Close()

	var records []AuditRecord
	for rows.Next() {
		var r AuditRecord
		err := rows.Scan(
			&r.RequestID, &r.Timestamp, &r.TenantID, &r.Model, &r.Provider,
			&r.Strategy, &r.Status, &r.LatencyMs, &r.FallbackUsed,
			&r.PIIWebhookRequestDecision, &r.PIIWebhookResponseDecision,
			&r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &r.CostUSD,
		)
		if err != nil {
			return nil, fmt.Errorf("scan audit record: %w", err)
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// DeleteOldRecords removes request_log and usage entries older than cutoff.
func (s *PostgresStorage) DeleteOldRecords(ctx context.Context, tenantID string, cutoffDate time.Time) (int, error) {
	// Delete from usage first (has FK to request_log)
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM usage WHERE tenant_id = $1 AND ts < $2`,
		tenantID, cutoffDate,
	)
	if err != nil {
		return 0, fmt.Errorf("delete old usage: %w", err)
	}

	// Delete from model_stats_daily (daily aggregates)
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM model_stats_daily WHERE tenant_id = $1 AND date < $2::date`,
		tenantID, cutoffDate,
	)
	if err != nil {
		return 0, fmt.Errorf("delete old model_stats_daily: %w", err)
	}

	// Delete from request_log
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM request_log WHERE tenant_id = $1 AND ts < $2`,
		tenantID, cutoffDate,
	)
	if err != nil {
		return 0, fmt.Errorf("delete old request_log: %w", err)
	}

	deleted, _ := result.RowsAffected()
	return int(deleted), nil
}

func (s *PostgresStorage) InsertBudgetAlert(ctx context.Context, alert BudgetAlert) (bool, error) {
	var insertedID uuid.UUID
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO budget_alerts
		 (id, tenant_id, threshold, month, triggered_at, current_spend_usd, budget_limit_usd)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (tenant_id, threshold, month) DO NOTHING
		 RETURNING id`,
		alert.ID, alert.TenantID, alert.Threshold, alert.Month,
		alert.TriggeredAt, alert.CurrentSpendUSD, alert.BudgetLimitUSD,
	).Scan(&insertedID)

	if err == sql.ErrNoRows {
		return false, nil // Duplicate (ON CONFLICT triggered)
	}
	if err != nil {
		return false, fmt.Errorf("insert budget alert: %w", err)
	}
	return true, nil // Successfully inserted
}

func (s *PostgresStorage) GetBudgetAlerts(ctx context.Context, tenantID, month string) ([]BudgetAlert, error) {
	query := `
		SELECT id, tenant_id, threshold, month, triggered_at, current_spend_usd, budget_limit_usd
		FROM budget_alerts
		WHERE tenant_id = $1 AND month = $2
		ORDER BY triggered_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, month)
	if err != nil {
		return nil, fmt.Errorf("query budget alerts: %w", err)
	}
	defer rows.Close()

	var alerts []BudgetAlert
	for rows.Next() {
		var a BudgetAlert
		err := rows.Scan(&a.ID, &a.TenantID, &a.Threshold, &a.Month,
			&a.TriggeredAt, &a.CurrentSpendUSD, &a.BudgetLimitUSD)
		if err != nil {
			return nil, fmt.Errorf("scan budget alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *PostgresStorage) InsertCostAnomaly(ctx context.Context, anomaly CostAnomaly) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cost_anomalies
		 (id, tenant_id, detected_at, date, daily_spend_usd, baseline_avg_usd, multiplier)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		anomaly.ID, anomaly.TenantID, anomaly.DetectedAt, anomaly.Date,
		anomaly.DailySpendUSD, anomaly.BaselineAvgUSD, anomaly.Multiplier,
	)
	return err
}

func (s *PostgresStorage) GetCostAnomalies(ctx context.Context, tenantID string, windowDays int) ([]CostAnomaly, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -windowDays)

	query := `
		SELECT id, tenant_id, detected_at, date, daily_spend_usd, baseline_avg_usd, multiplier
		FROM cost_anomalies
		WHERE tenant_id = $1 AND detected_at >= $2
		ORDER BY detected_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, tenantID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query cost anomalies: %w", err)
	}
	defer rows.Close()

	var anomalies []CostAnomaly
	for rows.Next() {
		var a CostAnomaly
		err := rows.Scan(&a.ID, &a.TenantID, &a.DetectedAt, &a.Date,
			&a.DailySpendUSD, &a.BaselineAvgUSD, &a.Multiplier)
		if err != nil {
			return nil, fmt.Errorf("scan cost anomaly: %w", err)
		}
		anomalies = append(anomalies, a)
	}
	return anomalies, rows.Err()
}

// ListAnomalies returns paginated rows from cost_anomalies for the admin API.
// Filters by window (hours) and optional tenant_id; model/provider/status not in schema, ignored.
func (s *PostgresStorage) ListAnomalies(ctx context.Context, filter AnomalyListFilter) ([]AnomalyListRow, int, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(filter.WindowHours) * time.Hour)
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}

	// Count total matching rows
	countQuery := `
		SELECT COUNT(*) FROM cost_anomalies
		WHERE detected_at >= $1 AND ($2::text IS NULL OR tenant_id = $2)
	`
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, cutoff, tenantArg).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_anomalies: %w", err)
	}

	// Fetch page: id, detected_at, tenant_id, daily_spend_usd, baseline_avg_usd, multiplier
	dataQuery := `
		SELECT id, detected_at, tenant_id, daily_spend_usd, baseline_avg_usd, multiplier
		FROM cost_anomalies
		WHERE detected_at >= $1 AND ($2::text IS NULL OR tenant_id = $2)
		ORDER BY detected_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := s.db.QueryContext(ctx, dataQuery, cutoff, tenantArg, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query cost_anomalies: %w", err)
	}
	defer rows.Close()

	var result []AnomalyListRow
	for rows.Next() {
		var id uuid.UUID
		var ts time.Time
		var tenantID string
		var observed, expected float64
		var mult float64
		if err := rows.Scan(&id, &ts, &tenantID, &observed, &expected, &mult); err != nil {
			return nil, 0, fmt.Errorf("scan anomaly row: %w", err)
		}
		deviationPct := 0.0
		if expected > 0 {
			deviationPct = (mult - 1.0) * 100
		}
		result = append(result, AnomalyListRow{
			AnomalyID:       id.String(),
			Timestamp:       ts,
			TenantID:        tenantID,
			Model:           "",
			Provider:        "",
			ExpectedCostUSD: expected,
			ObservedCostUSD: observed,
			DeviationPct:    deviationPct,
			Status:          "open",
			AnomalyType:     "cost_spike",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// GetAnomalyExplanations returns top drivers (model, provider, api_key) per anomaly for the last windowDays.
// Uses cost_anomalies for the list and usage for current/baseline breakdown by dimension.
func (s *PostgresStorage) GetAnomalyExplanations(ctx context.Context, windowDays int) ([]AnomalyExplanation, error) {
	const baselineDays = 7
	const maxAnomalies = 100

	// List anomalies: date within last window_days (anomaly date, not detected_at)
	rows, err := s.db.QueryContext(ctx, `
		SELECT tenant_id, date, daily_spend_usd, baseline_avg_usd, multiplier
		FROM cost_anomalies
		WHERE (date::date) >= (CURRENT_DATE - $1::int)
		ORDER BY date DESC
		LIMIT $2
	`, windowDays, maxAnomalies)
	if err != nil {
		return nil, fmt.Errorf("list anomalies for explain: %w", err)
	}
	defer rows.Close()

	type anomRow struct {
		tenantID      string
		date          string
		observedSpend float64
		expectedSpend float64
		multiplier    float64
	}
	var anomalies []anomRow
	for rows.Next() {
		var a anomRow
		if err := rows.Scan(&a.tenantID, &a.date, &a.observedSpend, &a.expectedSpend, &a.multiplier); err != nil {
			return nil, fmt.Errorf("scan anomaly: %w", err)
		}
		anomalies = append(anomalies, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]AnomalyExplanation, 0, len(anomalies))
	for _, a := range anomalies {
		expl := AnomalyExplanation{
			TenantID:      a.tenantID,
			ObservedSpend: a.observedSpend,
			ExpectedSpend: a.expectedSpend,
			Multiplier:    a.multiplier,
			TopDrivers:    AnomalyTopDrivers{},
		}

		// Top 3 models by delta_spend (current day - baseline avg over previous 7 days)
		modelNames, modelDeltas, err := s.topDriversByDimension(ctx, a.tenantID, a.date, "model", baselineDays, 3)
		if err != nil {
			return nil, fmt.Errorf("top models for %s@%s: %w", a.tenantID, a.date, err)
		}
		for i := range modelNames {
			expl.TopDrivers.Models = append(expl.TopDrivers.Models, ModelDriver{Model: modelNames[i], DeltaSpend: modelDeltas[i]})
		}

		provNames, provDeltas, err := s.topDriversByDimension(ctx, a.tenantID, a.date, "provider", baselineDays, 3)
		if err != nil {
			return nil, fmt.Errorf("top providers for %s@%s: %w", a.tenantID, a.date, err)
		}
		for i := range provNames {
			expl.TopDrivers.Providers = append(expl.TopDrivers.Providers, ProviderDriver{Provider: provNames[i], DeltaSpend: provDeltas[i]})
		}

		keyNames, keyDeltas, err := s.topDriversByDimension(ctx, a.tenantID, a.date, "api_key_name", baselineDays, 3)
		if err != nil {
			return nil, fmt.Errorf("top api_keys for %s@%s: %w", a.tenantID, a.date, err)
		}
		for i := range keyNames {
			expl.TopDrivers.APIKeys = append(expl.TopDrivers.APIKeys, APIKeyDriver{APIKeyName: keyNames[i], DeltaSpend: keyDeltas[i]})
		}

		result = append(result, expl)
	}
	return result, nil
}

// topDriversByDimension returns top N dimensions (model, provider, or api_key_name) by delta_spend.
// dimensionCol must be "model", "provider", or "api_key_name". Baseline = avg daily spend over previous baselineDays.
// Returns (dimension names, delta spends) ordered by delta descending.
func (s *PostgresStorage) topDriversByDimension(ctx context.Context, tenantID, anomalyDate, dimensionCol string, baselineDays, topN int) (names []string, deltas []float64, err error) {
	// usage has model, provider, api_key_name (ts = timestamp); use u. alias
	if dimensionCol != "model" && dimensionCol != "provider" && dimensionCol != "api_key_name" {
		return nil, nil, fmt.Errorf("invalid dimension: %s", dimensionCol)
	}
	dimExpr := "u." + dimensionCol
	if dimensionCol == "api_key_name" {
		dimExpr = "COALESCE(u.api_key_name, '')"
	}

	// Current day spend by dimension (usage has ts timestamp; filter by DATE(u.ts) = anomaly date)
	currentQuery := fmt.Sprintf(`
		SELECT %s AS dim, COALESCE(SUM(u.cost_usd), 0) AS spend
		FROM usage u
		WHERE u.tenant_id = $1 AND DATE(u.ts) = $2::date
		GROUP BY %s
	`, dimExpr, dimExpr)
	currentRows, err := s.db.QueryContext(ctx, currentQuery, tenantID, anomalyDate)
	if err != nil {
		return nil, nil, err
	}
	defer currentRows.Close()
	currentMap := make(map[string]float64)
	for currentRows.Next() {
		var dim string
		var spend float64
		if err := currentRows.Scan(&dim, &spend); err != nil {
			return nil, nil, err
		}
		currentMap[dim] = spend
	}
	if err := currentRows.Err(); err != nil {
		return nil, nil, err
	}

	// Baseline: avg daily spend by dimension over previous 7 days (usage has ts; use DATE(u.ts) BETWEEN a.date-7d and a.date-1d)
	baselineQuery := fmt.Sprintf(`
		WITH daily AS (
			SELECT %s AS dim, DATE(u.ts) AS d, SUM(u.cost_usd) AS daily_spend
			FROM usage u
			WHERE u.tenant_id = $1
			  AND DATE(u.ts) BETWEEN ($2::date - INTERVAL '7 days')::date AND ($2::date - INTERVAL '1 day')::date
			GROUP BY %s, DATE(u.ts)
		)
		SELECT dim, COALESCE(AVG(daily_spend), 0) AS baseline_avg
		FROM daily
		GROUP BY dim
	`, dimExpr, dimExpr)
	baselineRows, err := s.db.QueryContext(ctx, baselineQuery, tenantID, anomalyDate)
	if err != nil {
		return nil, nil, err
	}
	defer baselineRows.Close()
	baselineMap := make(map[string]float64)
	for baselineRows.Next() {
		var dim string
		var avg float64
		if err := baselineRows.Scan(&dim, &avg); err != nil {
			return nil, nil, err
		}
		baselineMap[dim] = avg
	}
	if err := baselineRows.Err(); err != nil {
		return nil, nil, err
	}

	// All dimension values that appear in either current or baseline
	seen := make(map[string]bool)
	for k := range currentMap {
		seen[k] = true
	}
	for k := range baselineMap {
		seen[k] = true
	}
	type dimDelta struct {
		dim   string
		delta float64
	}
	var pairs []dimDelta
	for dim := range seen {
		currentSpend := currentMap[dim]
		baselineSpend := baselineMap[dim]
		// delta_spend = current - baseline so spike contributors are positive
		deltaSpend := currentSpend - baselineSpend
		pairs = append(pairs, dimDelta{dim, deltaSpend})
	}
	// Sort by delta descending (highest positive contributors first)
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].delta > pairs[j].delta })
	if len(pairs) > topN {
		pairs = pairs[:topN]
	}
	names = make([]string, 0, len(pairs))
	deltas = make([]float64, 0, len(pairs))
	for _, p := range pairs {
		names = append(names, p.dim)
		deltas = append(deltas, p.delta)
	}
	return names, deltas, nil
}
func (s *PostgresStorage) GetAPIKeyUsage(ctx context.Context, filter APIKeyUsageFilter) (APIKeyUsageSummary, []APIKeyUsageRow, error) {
	since := time.Now().UTC().Add(-time.Duration(filter.WindowHours) * time.Hour)
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}
	providerArg := interface{}(nil)
	if filter.Provider != "" {
		providerArg = filter.Provider
	}
	modelArg := interface{}(nil)
	if filter.Model != "" {
		modelArg = filter.Model
	}
	statusArg := interface{}(nil)
	if filter.Status != "" {
		statusArg = filter.Status
	}
	apiKeyNameArg := interface{}(nil)
	if filter.APIKeyName != "" {
		apiKeyNameArg = filter.APIKeyName
	}
	args := []interface{}{since, tenantArg, providerArg, modelArg, statusArg, apiKeyNameArg}

	// Summary: from usage (api_key_id not null) in window with same filters
	// total_active_api_keys, total_requests, total_spend, avg_success_rate, highest_spend_key, most_active_key
	var totalKeys int
	var totalReqs int
	var totalSpend float64
	var avgRate sql.NullFloat64
	var highestSpendKey, mostActiveKey sql.NullString
	summaryQuery := `
		WITH u AS (
			SELECT api_key_id, api_key_name, tenant_id,
				COUNT(*) AS requests,
				SUM(cost_usd) AS spend
			FROM usage
			WHERE api_key_id IS NOT NULL AND ts >= $1
			  AND ($2::text IS NULL OR tenant_id = $2)
			  AND ($3::text IS NULL OR provider = $3)
			  AND ($4::text IS NULL OR model = $4)
			  AND ($6::text IS NULL OR api_key_name = $6)
			GROUP BY api_key_id, api_key_name, tenant_id
		),
		r AS (
			SELECT api_key_id,
				COUNT(DISTINCT request_id) AS total,
				COUNT(DISTINCT CASE WHEN status = 'ok' THEN request_id END) AS ok
			FROM request_log
			WHERE api_key_id IS NOT NULL AND ts >= $1
			  AND ($2::text IS NULL OR tenant_id = $2)
			  AND ($3::text IS NULL OR provider = $3)
			  AND ($4::text IS NULL OR model = $4)
			  AND ($5::text IS NULL OR status = $5)
			  AND ($6::text IS NULL OR api_key_name = $6)
			GROUP BY api_key_id
		)
		SELECT
			COUNT(*)::int,
			COALESCE(SUM(u.requests), 0)::int,
			COALESCE(SUM(u.spend), 0),
			(SELECT (SUM(r.ok)::float / NULLIF(SUM(r.total), 0)) FROM r),
			(SELECT api_key_name FROM u ORDER BY spend DESC LIMIT 1),
			(SELECT api_key_name FROM u ORDER BY requests DESC LIMIT 1)
		FROM u
		LEFT JOIN r ON r.api_key_id = u.api_key_id
	`
	if err := s.db.QueryRowContext(ctx, summaryQuery, args...).Scan(&totalKeys, &totalReqs, &totalSpend, &avgRate, &highestSpendKey, &mostActiveKey); err != nil {
		return APIKeyUsageSummary{}, nil, fmt.Errorf("api key usage summary: %w", err)
	}
	summary := APIKeyUsageSummary{
		TotalActiveAPIKeys: totalKeys,
		TotalRequests:      totalReqs,
		TotalSpend:         totalSpend,
		AvgSuccessRate:     0,
		HighestSpendKey:    highestSpendKey.String,
		MostActiveKey:      mostActiveKey.String,
	}
	if avgRate.Valid {
		summary.AvgSuccessRate = avgRate.Float64
	}

	// Data: per-key rows with success_rate, avg_latency_ms, top_model, top_provider, last_seen; paginated
	dataQuery := `
		WITH u AS (
			SELECT api_key_id, api_key_name, tenant_id,
				COUNT(*) AS requests,
				SUM(cost_usd) AS spend,
				MAX(ts) AS last_seen
			FROM usage
			WHERE api_key_id IS NOT NULL AND ts >= $1
			  AND ($2::text IS NULL OR tenant_id = $2)
			  AND ($3::text IS NULL OR provider = $3)
			  AND ($4::text IS NULL OR model = $4)
			  AND ($6::text IS NULL OR api_key_name = $6)
			GROUP BY api_key_id, api_key_name, tenant_id
		),
		r AS (
			SELECT api_key_id,
				COUNT(DISTINCT request_id) AS total,
				COUNT(DISTINCT CASE WHEN status = 'ok' THEN request_id END) AS ok,
				AVG(latency_ms) FILTER (WHERE status = 'ok' AND latency_ms IS NOT NULL) AS avg_latency_ms
			FROM request_log
			WHERE api_key_id IS NOT NULL AND ts >= $1
			  AND ($2::text IS NULL OR tenant_id = $2)
			  AND ($3::text IS NULL OR provider = $3)
			  AND ($4::text IS NULL OR model = $4)
			  AND ($5::text IS NULL OR status = $5)
			  AND ($6::text IS NULL OR api_key_name = $6)
			GROUP BY api_key_id
		),
		top_model AS (
			SELECT api_key_id, model AS top_model
			FROM (
				SELECT api_key_id, model, ROW_NUMBER() OVER (PARTITION BY api_key_id ORDER BY COUNT(*) DESC) AS rn
				FROM usage
				WHERE api_key_id IS NOT NULL AND ts >= $1
				  AND ($2::text IS NULL OR tenant_id = $2)
				  AND ($3::text IS NULL OR provider = $3)
				  AND ($4::text IS NULL OR model = $4)
				  AND ($6::text IS NULL OR api_key_name = $6)
				GROUP BY api_key_id, model
			) x
			WHERE rn = 1
		),
		top_provider AS (
			SELECT api_key_id, provider AS top_provider
			FROM (
				SELECT api_key_id, provider, ROW_NUMBER() OVER (PARTITION BY api_key_id ORDER BY COUNT(*) DESC) AS rn
				FROM usage
				WHERE api_key_id IS NOT NULL AND ts >= $1
				  AND ($2::text IS NULL OR tenant_id = $2)
				  AND ($3::text IS NULL OR provider = $3)
				  AND ($4::text IS NULL OR model = $4)
				  AND ($6::text IS NULL OR api_key_name = $6)
				GROUP BY api_key_id, provider
			) x
			WHERE rn = 1
		)
		SELECT u.api_key_id, COALESCE(u.api_key_name, ''), u.tenant_id, u.requests, u.spend,
			COALESCE(r.ok::float / NULLIF(r.total, 0), 0),
			COALESCE(r.avg_latency_ms, 0),
			COALESCE(tm.top_model, ''),
			COALESCE(tp.top_provider, ''),
			u.last_seen
		FROM u
		LEFT JOIN r ON r.api_key_id = u.api_key_id
		LEFT JOIN top_model tm ON tm.api_key_id = u.api_key_id
		LEFT JOIN top_provider tp ON tp.api_key_id = u.api_key_id
		ORDER BY u.requests DESC
		LIMIT $7 OFFSET $8
	`
	dataArgs := append(args, filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return APIKeyUsageSummary{}, nil, fmt.Errorf("api key usage data: %w", err)
	}
	defer rows.Close()

	var result []APIKeyUsageRow
	for rows.Next() {
		var row APIKeyUsageRow
		var avgLat, successRate float64
		if err := rows.Scan(&row.APIKeyID, &row.APIKeyName, &row.TenantID, &row.Requests, &row.Spend, &successRate, &avgLat, &row.TopModel, &row.TopProvider, &row.LastSeen); err != nil {
			return APIKeyUsageSummary{}, nil, fmt.Errorf("scan api key usage row: %w", err)
		}
		row.SuccessRate = successRate
		row.AvgLatencyMs = avgLat
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return APIKeyUsageSummary{}, nil, err
	}
	return summary, result, nil
}

// GetAPIKeyModelBreakdown returns per-(api_key_id, model) usage rows matching the same
// filter window as GetAPIKeyUsage. Used for effective cost computation.
func (s *PostgresStorage) GetAPIKeyModelBreakdown(ctx context.Context, filter APIKeyUsageFilter) ([]APIKeyModelUsageRow, error) {
	since := time.Now().UTC().Add(-time.Duration(filter.WindowHours) * time.Hour)
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}
	apiKeyNameArg := interface{}(nil)
	if filter.APIKeyName != "" {
		apiKeyNameArg = filter.APIKeyName
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT api_key_id, tenant_id, model,
		       COUNT(*)::int AS requests,
		       COALESCE(SUM(cost_usd), 0) AS spend
		FROM usage
		WHERE api_key_id IS NOT NULL
		  AND ts >= $1
		  AND ($2::text IS NULL OR tenant_id = $2)
		  AND ($3::text IS NULL OR api_key_name = $3)
		GROUP BY api_key_id, tenant_id, model
	`, since, tenantArg, apiKeyNameArg)
	if err != nil {
		return nil, fmt.Errorf("api key model breakdown: %w", err)
	}
	defer rows.Close()

	var result []APIKeyModelUsageRow
	for rows.Next() {
		var r APIKeyModelUsageRow
		if err := rows.Scan(&r.APIKeyID, &r.TenantID, &r.Model, &r.Requests, &r.Spend); err != nil {
			return nil, fmt.Errorf("scan api key model breakdown: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetJWTSubModelBreakdown returns per-(jwt_sub, tenant_id, model) usage rows matching the
// same filter window as GetJWTSubUsage. Used for monetization computation.
func (s *PostgresStorage) GetJWTSubModelBreakdown(ctx context.Context, filter JWTSubUsageFilter) ([]JWTSubModelUsageRow, error) {
	var fromT, toT interface{}
	if filter.From != nil {
		fromT = *filter.From
	}
	if filter.To != nil {
		toT = *filter.To
	}
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}
	modelArg := interface{}(nil)
	if filter.Model != "" {
		modelArg = filter.Model
	}
	providerArg := interface{}(nil)
	if filter.Provider != "" {
		providerArg = filter.Provider
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT jwt_sub, tenant_id, model,
		       COUNT(*)::int AS requests,
		       COALESCE(SUM(cost_usd), 0) AS spend
		FROM usage
		WHERE jwt_sub IS NOT NULL
		  AND ($1::timestamptz IS NULL OR ts >= $1)
		  AND ($2::timestamptz IS NULL OR ts <= $2)
		  AND ($3::text IS NULL OR tenant_id = $3)
		  AND ($4::text IS NULL OR model = $4)
		  AND ($5::text IS NULL OR provider = $5)
		GROUP BY jwt_sub, tenant_id, model
	`, fromT, toT, tenantArg, modelArg, providerArg)
	if err != nil {
		return nil, fmt.Errorf("jwt sub model breakdown: %w", err)
	}
	defer rows.Close()

	var result []JWTSubModelUsageRow
	for rows.Next() {
		var r JWTSubModelUsageRow
		if err := rows.Scan(&r.JWTSub, &r.TenantID, &r.Model, &r.Requests, &r.Spend); err != nil {
			return nil, fmt.Errorf("scan jwt sub model breakdown: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetTenantModelRequestCounts returns total request counts per model for a tenant since the given time.
// If tenantID is "", returns counts across all tenants (keyed by model name).
func (s *PostgresStorage) GetTenantModelRequestCounts(ctx context.Context, tenantID string, since time.Time) (map[string]int64, error) {
	tenantArg := interface{}(nil)
	if tenantID != "" {
		tenantArg = tenantID
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT model, COUNT(*)::bigint AS requests
		FROM usage
		WHERE ts >= $1
		  AND ($2::text IS NULL OR tenant_id = $2)
		GROUP BY model
	`, since, tenantArg)
	if err != nil {
		return nil, fmt.Errorf("tenant model request counts: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var model string
		var cnt int64
		if err := rows.Scan(&model, &cnt); err != nil {
			return nil, fmt.Errorf("scan tenant model counts: %w", err)
		}
		result[model] = cnt
	}
	return result, rows.Err()
}

// GetAPIKeyMetaByID returns key metadata by id. Returns found=false if key does not exist.
func (s *PostgresStorage) GetAPIKeyMetaByID(ctx context.Context, id uuid.UUID) (APIKeyMeta, bool, error) {
	var meta APIKeyMeta
	var scopes []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, prefix, scopes, created_at, expires_at, revoked_at, last_used_at
		 FROM api_keys WHERE id = $1`,
		id,
	).Scan(&meta.ID, &meta.TenantID, &meta.Name, &meta.Prefix, &scopes, &meta.CreatedAt, &meta.ExpiresAt, &meta.RevokedAt, &meta.LastUsedAt)
	if err == sql.ErrNoRows {
		return APIKeyMeta{}, false, nil
	}
	if err != nil {
		return APIKeyMeta{}, false, fmt.Errorf("get api key meta by id: %w", err)
	}
	if len(scopes) > 0 {
		_ = json.Unmarshal(scopes, &meta.Scopes)
	}
	return meta, true, nil
}

// GetAPIKeyUsageDetail returns full drilldown for one API key.
func (s *PostgresStorage) GetAPIKeyUsageDetail(ctx context.Context, apiKeyID uuid.UUID, windowHours, limit, offset int) (APIKeyUsageDetailSummary, []APIKeyUsageByModelRow, []APIKeyUsageByProviderRow, []APIKeyUsageRecentRow, int, LatencyStats, []ErrorCountRow, error) {
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	emptySummary := APIKeyUsageDetailSummary{}
	emptyLatency := LatencyStats{}

	// Summary: same logic as aggregate but filtered by api_key_id
	var requests int
	var spend float64
	var totalReqs, okReqs sql.NullInt64
	var avgLat sql.NullFloat64
	var topModel, topProvider sql.NullString
	var lastSeen sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		WITH u AS (
			SELECT COUNT(*)::int AS requests, COALESCE(SUM(cost_usd), 0) AS spend, MAX(ts) AS last_seen
			FROM usage
			WHERE api_key_id = $1 AND ts >= $2
		),
		r AS (
			SELECT COUNT(DISTINCT request_id) AS total, COUNT(DISTINCT CASE WHEN status = 'ok' THEN request_id END) AS ok,
				AVG(latency_ms) FILTER (WHERE status = 'ok' AND latency_ms IS NOT NULL) AS avg_latency_ms
			FROM request_log
			WHERE api_key_id = $1 AND ts >= $2
		),
		top_m AS (
			SELECT model FROM (
				SELECT model, ROW_NUMBER() OVER (ORDER BY COUNT(*) DESC) AS rn FROM usage WHERE api_key_id = $1 AND ts >= $2 GROUP BY model
			) x WHERE rn = 1 LIMIT 1
		),
		top_p AS (
			SELECT provider FROM (
				SELECT provider, ROW_NUMBER() OVER (ORDER BY COUNT(*) DESC) AS rn FROM usage WHERE api_key_id = $1 AND ts >= $2 GROUP BY provider
			) x WHERE rn = 1 LIMIT 1
		)
		SELECT u.requests, u.spend, u.last_seen, r.total, r.ok, r.avg_latency_ms, (SELECT model FROM top_m), (SELECT provider FROM top_p)
		FROM u
		CROSS JOIN r
	`, apiKeyID, since).Scan(&requests, &spend, &lastSeen, &totalReqs, &okReqs, &avgLat, &topModel, &topProvider)
	if err != nil && err != sql.ErrNoRows {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("api key usage detail summary: %w", err)
	}
	summary := APIKeyUsageDetailSummary{
		Requests:     requests,
		Spend:        spend,
		AvgLatencyMs: 0,
		TopModel:     topModel.String,
		TopProvider:  topProvider.String,
	}
	if totalReqs.Valid && totalReqs.Int64 > 0 && okReqs.Valid {
		summary.SuccessRate = float64(okReqs.Int64) / float64(totalReqs.Int64)
	}
	if avgLat.Valid {
		summary.AvgLatencyMs = avgLat.Float64
	}
	if lastSeen.Valid {
		summary.LastSeen = lastSeen.Time
	}

	// Requests by model: from usage, group by model, order by requests DESC
	modelRows, err := s.db.QueryContext(ctx, `
		SELECT model, COUNT(*)::int AS requests, COALESCE(SUM(cost_usd), 0) AS spend
		FROM usage
		WHERE api_key_id = $1 AND ts >= $2
		GROUP BY model
		ORDER BY requests DESC
	`, apiKeyID, since)
	if err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("api key usage by model: %w", err)
	}
	defer modelRows.Close()
	var byModel []APIKeyUsageByModelRow
	for modelRows.Next() {
		var r APIKeyUsageByModelRow
		if err := modelRows.Scan(&r.Model, &r.Requests, &r.Spend); err != nil {
			return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("scan by model: %w", err)
		}
		byModel = append(byModel, r)
	}
	if err := modelRows.Err(); err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, err
	}

	// Requests by provider: from usage, group by provider, order by requests DESC
	provRows, err := s.db.QueryContext(ctx, `
		SELECT provider, COUNT(*)::int AS requests
		FROM usage
		WHERE api_key_id = $1 AND ts >= $2
		GROUP BY provider
		ORDER BY requests DESC
	`, apiKeyID, since)
	if err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("api key usage by provider: %w", err)
	}
	defer provRows.Close()
	var byProvider []APIKeyUsageByProviderRow
	for provRows.Next() {
		var r APIKeyUsageByProviderRow
		if err := provRows.Scan(&r.Provider, &r.Requests); err != nil {
			return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("scan by provider: %w", err)
		}
		byProvider = append(byProvider, r)
	}
	if err := provRows.Err(); err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, err
	}

	// Recent requests: request_log left join usage on request_id, order by ts DESC, limit/offset
	var totalRecent int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::int FROM request_log
		WHERE api_key_id = $1 AND ts >= $2
	`, apiKeyID, since).Scan(&totalRecent); err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("count recent requests: %w", err)
	}

	recentRows, err := s.db.QueryContext(ctx, `
		SELECT rl.ts, rl.request_id, rl.model, rl.provider, rl.status, COALESCE(rl.latency_ms, 0), COALESCE(u.cost_usd, 0)
		FROM request_log rl
		INNER JOIN usage u ON u.request_id = rl.request_id AND u.api_key_id = rl.api_key_id AND u.model = rl.model
		WHERE rl.api_key_id = $1 AND rl.ts >= $2
		ORDER BY rl.ts DESC
		LIMIT $3 OFFSET $4
	`, apiKeyID, since, limit, offset)
	if err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("recent requests: %w", err)
	}
	defer recentRows.Close()
	var recent []APIKeyUsageRecentRow
	for recentRows.Next() {
		var r APIKeyUsageRecentRow
		if err := recentRows.Scan(&r.Timestamp, &r.RequestID, &r.Model, &r.Provider, &r.Status, &r.LatencyMs, &r.CostUSD); err != nil {
			return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("scan recent: %w", err)
		}
		recent = append(recent, r)
	}
	if err := recentRows.Err(); err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, err
	}

	// Latency percentiles (empty dataset → zeros)
	var p50, p95, maxLat sql.NullFloat64
	latencyStats := LatencyStats{}
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) AS p50,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) AS p95,
			MAX(latency_ms) AS max_latency
		FROM request_log
		WHERE api_key_id = $1 AND ts >= $2 AND latency_ms IS NOT NULL
	`, apiKeyID, since).Scan(&p50, &p95, &maxLat); err == nil {
		if p50.Valid {
			latencyStats.P50 = int(p50.Float64)
		}
		if p95.Valid {
			latencyStats.P95 = int(p95.Float64)
		}
		if maxLat.Valid {
			latencyStats.Max = int(maxLat.Float64)
		}
	}
	// err == sql.ErrNoRows or other: leave latencyStats as zeros

	// Errors by type (status = 'error', same window); empty dataset → []
	errorsRows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(error_type, ''), COUNT(*)::int
		FROM request_log
		WHERE api_key_id = $1 AND ts >= $2 AND status = 'error'
		GROUP BY error_type
		ORDER BY COUNT(*) DESC
	`, apiKeyID, since)
	if err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("errors by type: %w", err)
	}
	defer errorsRows.Close()
	var errorsByType []ErrorCountRow
	for errorsRows.Next() {
		var r ErrorCountRow
		if err := errorsRows.Scan(&r.ErrorType, &r.Count); err != nil {
			return emptySummary, nil, nil, nil, 0, emptyLatency, nil, fmt.Errorf("scan errors by type: %w", err)
		}
		errorsByType = append(errorsByType, r)
	}
	if err := errorsRows.Err(); err != nil {
		return emptySummary, nil, nil, nil, 0, emptyLatency, nil, err
	}

	return summary, byModel, byProvider, recent, totalRecent, latencyStats, errorsByType, nil
}

// GetAnomalyStats returns aggregate anomaly stats for the dashboard.
func (s *PostgresStorage) GetAnomalyStats(ctx context.Context, windowHours int, tenantID, model, provider string) (AnomalyStats, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	tenantArg := interface{}(nil)
	if tenantID != "" {
		tenantArg = tenantID
	}
	args := []interface{}{cutoff, tenantArg}

	// Summary: active_anomalies (all in window), cost_spike_24h_usd, affected_tenants, affected_models
	var activeAnomalies int
	var costSpikeUSD float64
	var affectedTenants int
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(GREATEST(daily_spend_usd - baseline_avg_usd, 0)), 0),
			COUNT(DISTINCT tenant_id)
		FROM cost_anomalies
		WHERE detected_at >= $1 AND ($2::text IS NULL OR tenant_id = $2)
	`, args...).Scan(&activeAnomalies, &costSpikeUSD, &affectedTenants)
	if err != nil {
		return AnomalyStats{}, fmt.Errorf("anomaly stats summary: %w", err)
	}
	summary := AnomalyStatsSummary{
		ActiveAnomalies: activeAnomalies,
		CostSpike24hUSD: costSpikeUSD,
		AffectedTenants: affectedTenants,
		AffectedModels:  0, // not in schema
	}

	// Timeline: bucket by hour if window <= 24, else by day
	trunc := "hour"
	if windowHours > 24 {
		trunc = "day"
	}
	timelineQuery := fmt.Sprintf(`
		SELECT date_trunc('%s', detected_at) AS bucket, COUNT(*), COALESCE(SUM(daily_spend_usd), 0)
		FROM cost_anomalies
		WHERE detected_at >= $1 AND ($2::text IS NULL OR tenant_id = $2)
		GROUP BY 1 ORDER BY 1
	`, trunc)
	timelineRows, err := s.db.QueryContext(ctx, timelineQuery, args...)
	if err != nil {
		return AnomalyStats{}, fmt.Errorf("anomaly stats timeline: %w", err)
	}
	defer timelineRows.Close()
	var timeline []AnomalyTimelineBucket
	for timelineRows.Next() {
		var b AnomalyTimelineBucket
		if err := timelineRows.Scan(&b.Bucket, &b.Anomalies, &b.ObservedCostUSD); err != nil {
			return AnomalyStats{}, fmt.Errorf("scan timeline row: %w", err)
		}
		timeline = append(timeline, b)
	}
	if err := timelineRows.Err(); err != nil {
		return AnomalyStats{}, err
	}

	// Top tenants by anomaly count
	topRows, err := s.db.QueryContext(ctx, `
		SELECT tenant_id, COUNT(*) AS anomalies
		FROM cost_anomalies
		WHERE detected_at >= $1 AND ($2::text IS NULL OR tenant_id = $2)
		GROUP BY tenant_id
		ORDER BY anomalies DESC
	`, args...)
	if err != nil {
		return AnomalyStats{}, fmt.Errorf("anomaly stats top_tenants: %w", err)
	}
	defer topRows.Close()
	var topTenants []AnomalyTopTenant
	for topRows.Next() {
		var t AnomalyTopTenant
		if err := topRows.Scan(&t.TenantID, &t.Anomalies); err != nil {
			return AnomalyStats{}, fmt.Errorf("scan top_tenant row: %w", err)
		}
		topTenants = append(topTenants, t)
	}
	if err := topRows.Err(); err != nil {
		return AnomalyStats{}, err
	}

	// Deviation histogram: (multiplier - 1) * 100 -> 0-25, 25-50, 50-100, 100+
	// COALESCE so that zero rows returns 0 instead of NULL (Scan would fail on NULL into int).
	histRows, err := s.db.QueryContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN (multiplier - 1.0) * 100 < 25 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (multiplier - 1.0) * 100 >= 25 AND (multiplier - 1.0) * 100 < 50 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (multiplier - 1.0) * 100 >= 50 AND (multiplier - 1.0) * 100 < 100 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (multiplier - 1.0) * 100 >= 100 THEN 1 ELSE 0 END), 0)
		FROM cost_anomalies
		WHERE detected_at >= $1 AND ($2::text IS NULL OR tenant_id = $2)
	`, args...)
	if err != nil {
		return AnomalyStats{}, fmt.Errorf("anomaly stats histogram: %w", err)
	}
	defer histRows.Close()
	histogram := []AnomalyDeviationBucket{
		{Range: "0-25%", Count: 0},
		{Range: "25-50%", Count: 0},
		{Range: "50-100%", Count: 0},
		{Range: "100%+", Count: 0},
	}
	if histRows.Next() {
		if err := histRows.Scan(&histogram[0].Count, &histogram[1].Count, &histogram[2].Count, &histogram[3].Count); err != nil {
			return AnomalyStats{}, fmt.Errorf("scan histogram: %w", err)
		}
	}
	if err := histRows.Err(); err != nil {
		return AnomalyStats{}, err
	}

	// Ensure non-nil slices for empty result sets (spec: return empty slice, not nil).
	if timeline == nil {
		timeline = []AnomalyTimelineBucket{}
	}
	if topTenants == nil {
		topTenants = []AnomalyTopTenant{}
	}

	return AnomalyStats{
		WindowHours:        windowHours,
		Summary:            summary,
		Timeline:           timeline,
		TopTenants:         topTenants,
		DeviationHistogram: histogram,
	}, nil
}

// Priority: tenant_active_config → tenant_config_versions (versioned path) takes precedence
// over tenants_config (flat path). This ensures that configs seeded via SeedTenantVersionedConfig
// (bootstrap) are always used at runtime, even if tenants_config is stale or missing.
func (s *PostgresStorage) GetTenantConfig(ctx context.Context, tenantID string) (json.RawMessage, int, bool, error) {
	// 1. Try versioned tables first — these are authoritative when present.
	var configText string
	var version int

	err := s.db.QueryRowContext(ctx, `
		SELECT tcv.config_yaml, tcv.version
		FROM tenant_active_config tac
		JOIN tenant_config_versions tcv ON tcv.id = tac.active_config_id
		WHERE tac.tenant_id = $1
	`, tenantID).Scan(&configText, &version)

	if err != nil && err != sql.ErrNoRows {
		return nil, 0, false, fmt.Errorf("get tenant versioned config: %w", err)
	}

	if err == nil {
		configJSON := json.RawMessage(configText)
		normalized, normErr := NormalizeJSONConfig(configJSON)
		if normErr != nil {
			s.log.WarnContext(ctx, "failed to normalize versioned config, returning as-is", "tenant_id", tenantID, "error", normErr)
			return configJSON, version, true, nil
		}
		return normalized, version, true, nil
	}

	// 2. Fall back to flat tenants_config (backwards compatibility for tenants not yet versioned).
	var configJSON json.RawMessage

	err = s.db.QueryRowContext(ctx, `
		SELECT config_json, version
		FROM tenants_config
		WHERE tenant_id = $1
	`, tenantID).Scan(&configJSON, &version)

	if err == sql.ErrNoRows {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("get tenant config: %w", err)
	}

	// Normalize legacy PascalCase configs to snake_case
	// This ensures consistent API responses regardless of how config was stored
	normalized, err := NormalizeJSONConfig(configJSON)
	if err != nil {
		s.log.WarnContext(ctx, "failed to normalize config, returning as-is", "tenant_id", tenantID, "error", err)
		return configJSON, version, true, nil
	}

	return normalized, version, true, nil
}

// PutTenantConfig updates tenant configuration with optimistic concurrency control
func (s *PostgresStorage) PutTenantConfig(ctx context.Context, tenantID string, ifMatchVersion int, newConfigJSON json.RawMessage, actorSub string, actorRoles []string, summary string, diffJSON json.RawMessage) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Check versioned path first — mirrors GetTenantConfig's priority so that
	//    tenant_active_config is always authoritative.  A stale tenants_config row
	//    (e.g. version=0 left over from seeding) is intentionally ignored when the
	//    versioned path is present.
	var versionedVersion int
	vErr := tx.QueryRowContext(ctx, `
		SELECT tcv.version
		FROM tenant_active_config tac
		JOIN tenant_config_versions tcv ON tcv.id = tac.active_config_id
		WHERE tac.tenant_id = $1
		FOR UPDATE
	`, tenantID).Scan(&versionedVersion)

	if vErr != nil && vErr != sql.ErrNoRows {
		return 0, fmt.Errorf("check versioned config: %w", vErr)
	}

	if vErr == nil {
		// Tenant lives in versioned tables.  Write a new version there.
		if versionedVersion != ifMatchVersion {
			return 0, ErrVersionConflict{Expected: ifMatchVersion, Current: versionedVersion}
		}
		newVersion := versionedVersion + 1
		sum := sha256.Sum256(newConfigJSON)
		hashStr := hex.EncodeToString(sum[:])

		var newVersionID string
		err = tx.QueryRowContext(ctx, `
			INSERT INTO tenant_config_versions
				(tenant_id, version, config_yaml, config_sha256, created_by)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id::text
		`, tenantID, newVersion, string(newConfigJSON), hashStr, actorSub).Scan(&newVersionID)
		if err != nil {
			return 0, fmt.Errorf("insert tenant_config_versions: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE tenant_active_config
			SET active_config_id = $1::uuid,
			    updated_by       = $2,
			    change_reason    = $3,
			    updated_at       = now()
			WHERE tenant_id     = $4
		`, newVersionID, actorSub, summary, tenantID)
		if err != nil {
			return 0, fmt.Errorf("update tenant_active_config: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO config_change_log (tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, tenantID, actorSub, pq.Array(actorRoles), versionedVersion, newVersion, summary, diffJSON)
		if err != nil {
			return 0, fmt.Errorf("insert change log: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("commit transaction: %w", err)
		}
		return newVersion, nil
	}

	// 2. No versioned path — fall back to flat tenants_config.
	var currentVersion int
	err = tx.QueryRowContext(ctx, `
		SELECT version FROM tenants_config WHERE tenant_id = $1 FOR UPDATE
	`, tenantID).Scan(&currentVersion)

	if err == sql.ErrNoRows {
		// Truly new tenant — not in either table.
		if ifMatchVersion != 0 {
			return 0, ErrVersionConflict{Expected: ifMatchVersion, Current: 0}
		}

		err = tx.QueryRowContext(ctx, `
			INSERT INTO tenants_config (tenant_id, version, config_json, updated_by)
			VALUES ($1, 1, $2, $3)
			RETURNING version
		`, tenantID, newConfigJSON, actorSub).Scan(&currentVersion)
		if err != nil {
			return 0, fmt.Errorf("insert config: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO config_change_log (tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, tenantID, actorSub, pq.Array(actorRoles), 0, 1, summary, diffJSON)
		if err != nil {
			return 0, fmt.Errorf("insert change log: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("commit transaction: %w", err)
		}
		return 1, nil
	} else if err != nil {
		return 0, fmt.Errorf("check version: %w", err)
	}

	// Flat path: version conflict check.
	if currentVersion != ifMatchVersion {
		return 0, ErrVersionConflict{Expected: ifMatchVersion, Current: currentVersion}
	}

	newVersion := currentVersion + 1
	_, err = tx.ExecContext(ctx, `
		UPDATE tenants_config
		SET version    = $1,
		    config_json = $2,
		    updated_at  = NOW(),
		    updated_by  = $3
		WHERE tenant_id = $4 AND version = $5
	`, newVersion, newConfigJSON, actorSub, tenantID, currentVersion)
	if err != nil {
		return 0, fmt.Errorf("update config: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO config_change_log (tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, tenantID, actorSub, pq.Array(actorRoles), currentVersion, newVersion, summary, diffJSON)
	if err != nil {
		return 0, fmt.Errorf("insert change log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	return newVersion, nil
}

// PatchTenantConfig applies JSON Merge Patch to tenant configuration
func (s *PostgresStorage) PatchTenantConfig(ctx context.Context, tenantID string, ifMatchVersion int, mergePatchJSON json.RawMessage, actorSub string, actorRoles []string) (int, error) {
	// 1. Get current config (already normalized by GetTenantConfig)
	currentJSON, currentVersion, exists, err := s.GetTenantConfig(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, fmt.Errorf("tenant not found: %s", tenantID)
	}
	if currentVersion != ifMatchVersion {
		return 0, ErrVersionConflict{Expected: ifMatchVersion, Current: currentVersion}
	}

	// 2. Normalize patch to snake_case (in case client sends PascalCase)
	normalizedPatchJSON, err := NormalizeJSONConfig(mergePatchJSON)
	if err != nil {
		return 0, fmt.Errorf("normalize patch: %w", err)
	}

	// 3. Apply JSON Merge Patch
	var current map[string]interface{}
	if err := json.Unmarshal(currentJSON, &current); err != nil {
		return 0, fmt.Errorf("unmarshal current config: %w", err)
	}

	var patch map[string]interface{}
	if err := json.Unmarshal(normalizedPatchJSON, &patch); err != nil {
		return 0, fmt.Errorf("unmarshal patch: %w", err)
	}

	merged := mergeMaps(current, patch)

	newConfigJSON, err := json.Marshal(merged)
	if err != nil {
		return 0, fmt.Errorf("marshal merged config: %w", err)
	}

	// 4. Generate diff
	diff := computeDiff(current, merged)
	diffJSON, _ := json.Marshal(diff)

	summary := fmt.Sprintf("PATCH: updated %d fields", len(diff))

	// 5. Use PutTenantConfig to save (now guaranteed to be snake_case)
	return s.PutTenantConfig(ctx, tenantID, ifMatchVersion, newConfigJSON, actorSub, actorRoles, summary, diffJSON)
}

// ListTenantConfigChanges retrieves configuration change history
func (s *PostgresStorage) ListTenantConfigChanges(ctx context.Context, tenantID string, limit int) ([]ConfigChange, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ts, tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json
		FROM config_change_log
		WHERE tenant_id = $1
		ORDER BY ts DESC
		LIMIT $2
	`, tenantID, limit)

	if err != nil {
		return nil, fmt.Errorf("query change log: %w", err)
	}
	defer rows.Close()

	var changes []ConfigChange
	for rows.Next() {
		var c ConfigChange
		var actorRolesArray pq.StringArray

		err := rows.Scan(&c.ID, &c.Timestamp, &c.TenantID, &c.ActorSub, &actorRolesArray, &c.FromVersion, &c.ToVersion, &c.Summary, &c.Diff)
		if err != nil {
			return nil, fmt.Errorf("scan change: %w", err)
		}

		c.ActorRoles = []string(actorRolesArray)
		changes = append(changes, c)
	}

	return changes, rows.Err()
}

// ListConfigHistory returns normalized config change history (tenant + global) for GET /admin/config/history.
func (s *PostgresStorage) ListConfigHistory(ctx context.Context, filter ConfigHistoryFilter) ([]ConfigHistoryRow, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}

	if filter.ExcludeGlobal {
		if filter.Scope == "global" {
			return nil, nil
		}
		if len(filter.AllowedTenantIDs) == 0 {
			return nil, nil
		}
		return s.listConfigHistoryTenantOnly(ctx, tenantArg, filter.AllowedTenantIDs, limit, offset)
	}

	var rows *sql.Rows
	var err error
	switch filter.Scope {
	case "global":
		rows, err = s.db.QueryContext(ctx, `
			SELECT
				'global' AS scope,
				''::text AS tenant_id,
				ts AS changed_at,
				COALESCE(actor_sub, '') AS changed_by,
				COALESCE(from_version, 0)::int AS from_version,
				to_version::int AS to_version,
				(CASE WHEN from_version IS NOT NULL AND to_version < from_version THEN true ELSE false END) AS is_rollback
			FROM global_config_change_log
			ORDER BY ts DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	case "tenant":
		rows, err = s.db.QueryContext(ctx, `
			SELECT
				'tenant' AS scope,
				tenant_id,
				ts AS changed_at,
				COALESCE(actor_sub, '') AS changed_by,
				COALESCE(from_version, 0) AS from_version,
				to_version,
				(to_version < from_version) AS is_rollback
			FROM config_change_log
			WHERE $1::text IS NULL OR tenant_id = $1
			ORDER BY ts DESC
			LIMIT $2 OFFSET $3
		`, tenantArg, limit, offset)
	default:
		// scope empty or any other: merge both, order by changed_at DESC, paginate
		rows, err = s.db.QueryContext(ctx, `
			WITH combined AS (
				SELECT 'tenant' AS scope, tenant_id, ts AS changed_at, COALESCE(actor_sub, '') AS changed_by,
					COALESCE(from_version, 0) AS from_version, to_version,
					(to_version < from_version) AS is_rollback
				FROM config_change_log
				WHERE $1::text IS NULL OR tenant_id = $1
				UNION ALL
				SELECT 'global', ''::text, ts, COALESCE(actor_sub, ''),
					COALESCE(from_version, 0)::int, to_version::int,
					(CASE WHEN from_version IS NOT NULL AND to_version < from_version THEN true ELSE false END)
				FROM global_config_change_log
			)
			SELECT scope, tenant_id, changed_at, changed_by, from_version, to_version, is_rollback
			FROM combined
			ORDER BY changed_at DESC
			LIMIT $2 OFFSET $3
		`, tenantArg, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list config history: %w", err)
	}
	defer rows.Close()

	var result []ConfigHistoryRow
	for rows.Next() {
		var r ConfigHistoryRow
		var fromVer, toVer int
		if err := rows.Scan(&r.Scope, &r.TenantID, &r.ChangedAt, &r.ChangedBy, &fromVer, &toVer, &r.IsRollback); err != nil {
			return nil, fmt.Errorf("scan config history row: %w", err)
		}
		r.FromVersion = fromVer
		r.ToVersion = toVer
		if r.IsRollback {
			r.ChangeType = "rollback"
		} else {
			r.ChangeType = "update"
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// listConfigHistoryTenantOnly returns tenant config_change_log rows only, optionally filtered by tenant_id
// and restricted to allowedTenantIDs.
func (s *PostgresStorage) listConfigHistoryTenantOnly(ctx context.Context, tenantArg interface{}, allowedTenantIDs []string, limit, offset int) ([]ConfigHistoryRow, error) {
	rows, err := s.db.QueryContext(ctx, `
			SELECT
				'tenant' AS scope,
				tenant_id,
				ts AS changed_at,
				COALESCE(actor_sub, '') AS changed_by,
				COALESCE(from_version, 0) AS from_version,
				to_version,
				(to_version < from_version) AS is_rollback
			FROM config_change_log
			WHERE ($1::text IS NULL OR tenant_id = $1)
			  AND tenant_id = ANY($2::text[])
			ORDER BY ts DESC
			LIMIT $3 OFFSET $4
		`, tenantArg, pq.Array(allowedTenantIDs), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list config history (tenant-only): %w", err)
	}
	defer rows.Close()

	var result []ConfigHistoryRow
	for rows.Next() {
		var r ConfigHistoryRow
		var fromVer, toVer int
		if err := rows.Scan(&r.Scope, &r.TenantID, &r.ChangedAt, &r.ChangedBy, &fromVer, &toVer, &r.IsRollback); err != nil {
			return nil, fmt.Errorf("scan config history row: %w", err)
		}
		r.FromVersion = fromVer
		r.ToVersion = toVer
		if r.IsRollback {
			r.ChangeType = "rollback"
		} else {
			r.ChangeType = "update"
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListConfigVersions returns version timeline (no payloads) for GET /admin/config/versions.
func (s *PostgresStorage) ListConfigVersions(ctx context.Context, filter ConfigVersionFilter) ([]ConfigVersionRow, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var rows *sql.Rows
	var err error
	switch filter.Scope {
	case "global":
		// Order by id DESC so newest version first; never ORDER BY created_at, never LIMIT without ORDER.
		rows, err = s.db.QueryContext(ctx, `
			SELECT id AS version, created_at
			FROM global_config_versions
			ORDER BY id DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	case "tenant":
		if filter.TenantID == "" {
			return nil, fmt.Errorf("tenant_id required when scope=tenant")
		}
		rows, err = s.db.QueryContext(ctx, `
			SELECT version, created_at
			FROM tenant_config_versions
			WHERE tenant_id = $1
			ORDER BY version DESC
			LIMIT $2 OFFSET $3
		`, filter.TenantID, limit, offset)
	default:
		return nil, fmt.Errorf("invalid scope %q", filter.Scope)
	}
	if err != nil {
		return nil, fmt.Errorf("list config versions: %w", err)
	}
	defer rows.Close()

	var result []ConfigVersionRow
	for rows.Next() {
		var r ConfigVersionRow
		if err := rows.Scan(&r.Version, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan config version row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetConfigAtVersion returns the config JSON at a specific version for diff/view.
func (s *PostgresStorage) GetConfigAtVersion(ctx context.Context, scope, tenantID string, version int) (json.RawMessage, error) {
	var raw json.RawMessage
	var err error
	switch scope {
	case "global":
		err = s.db.QueryRowContext(ctx, `
			SELECT config_json FROM global_config_versions WHERE id = $1
		`, version).Scan(&raw)
	case "tenant":
		if tenantID == "" {
			return nil, fmt.Errorf("tenant_id required when scope=tenant")
		}
		var configText string
		err = s.db.QueryRowContext(ctx, `
			SELECT config_yaml FROM tenant_config_versions WHERE tenant_id = $1 AND version = $2
		`, tenantID, version).Scan(&configText)
		if err == nil {
			raw = json.RawMessage(configText)
		}
	default:
		return nil, fmt.Errorf("invalid scope %q", scope)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrConfigVersionNotFound
		}
		return nil, fmt.Errorf("get config at version: %w", err)
	}
	return raw, nil
}

// SeedTenantConfig initializes tenant config in DB with version=0 if it doesn't exist.
// This is used when a tenant exists in YAML but not yet in the database.
// IMPORTANT: Does NOT create a config_change_log entry - seeding is silent.
// Returns true if seeded, false if already exists.
func (s *PostgresStorage) SeedTenantConfig(ctx context.Context, tenantID string, configJSON json.RawMessage) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO tenants_config (tenant_id, version, config_json, updated_by)
		VALUES ($1, 0, $2, 'seed-from-yaml')
		ON CONFLICT (tenant_id) DO NOTHING
	`, tenantID, configJSON)

	if err != nil {
		return false, fmt.Errorf("seed tenant config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check rows affected: %w", err)
	}

	seeded := rowsAffected > 0
	if seeded {
		s.log.Info("seeded tenant config from YAML", "tenant_id", tenantID)
	}

	return seeded, nil
}

// ============================================================================
// Dynamic Global Configuration Methods
// ============================================================================

// GetGlobalConfig retrieves the current active global config from the database.
// Current config is defined by global_active_config.active_version (not "latest row").
// When the active pointer is missing or orphaned, we repair by setting it to the latest
// version using ORDER BY id DESC LIMIT 1 (never ORDER BY created_at, never LIMIT without ORDER).
// Returns (configJSON, version, exists, error).
func (s *PostgresStorage) GetGlobalConfig(ctx context.Context) (json.RawMessage, int, bool, error) {
	var configJSON json.RawMessage
	var version int

	err := s.db.QueryRowContext(ctx, `
		SELECT v.config_json, v.id
		FROM global_active_config a
		JOIN global_config_versions v ON v.id = a.active_version
		WHERE a.id = 1
	`).Scan(&configJSON, &version)

	if err == nil {
		return configJSON, version, true, nil
	}
	if err != sql.ErrNoRows {
		return nil, 0, false, fmt.Errorf("get global config: %w", err)
	}

	// No row: either global_active_config is empty or active_version points to a missing id.
	// Repair: if any version exists, set active to the latest by id (ORDER BY id DESC LIMIT 1).
	err = s.db.QueryRowContext(ctx, `
		SELECT config_json, id
		FROM global_config_versions
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&configJSON, &version)
	if err == sql.ErrNoRows {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("get latest global config: %w", err)
	}
	// Repair: ensure global_active_config points to this version (insert or update).
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO global_active_config (id, active_version, updated_at)
		VALUES (1, $1, now())
		ON CONFLICT (id) DO UPDATE SET active_version = $1, updated_at = now()
	`, version)
	// Best-effort; if repair fails we still return the config we found.
	return configJSON, version, true, nil
}

// SeedGlobalConfig inserts a new version and sets it as active, but ONLY if no active config exists.
// Returns true if seeded, false if already exists.
func (s *PostgresStorage) SeedGlobalConfig(ctx context.Context, configJSON json.RawMessage) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if active config already exists
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM global_active_config WHERE id = 1)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check global active config: %w", err)
	}
	if exists {
		return false, nil
	}

	// Insert a new version
	var versionID int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO global_config_versions (config_json, created_by)
		VALUES ($1, 'seed-from-yaml')
		RETURNING id
	`, configJSON).Scan(&versionID)
	if err != nil {
		return false, fmt.Errorf("insert global config version: %w", err)
	}

	// Set as active
	_, err = tx.ExecContext(ctx, `
		INSERT INTO global_active_config (id, active_version)
		VALUES (1, $1)
	`, versionID)
	if err != nil {
		return false, fmt.Errorf("insert global active config: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	s.log.InfoContext(ctx, "global config seeded from YAML", "version_id", versionID)
	return true, nil
}

// PutGlobalConfig replaces the global configuration using optimistic locking.
// ifMatchVersion must equal global_active_config.active_version.
// On success, a new row is inserted into global_config_versions and global_active_config is updated.
// Returns ErrVersionConflict when versions do not match.
func (s *PostgresStorage) PutGlobalConfig(
	ctx context.Context,
	ifMatchVersion int,
	configJSON json.RawMessage,
	actorSub string,
	actorRoles []string,
) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the singleton row and read current version.
	var currentVersion int
	err = tx.QueryRowContext(ctx,
		`SELECT active_version FROM global_active_config WHERE id = 1 FOR UPDATE`,
	).Scan(&currentVersion)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("global config has not been initialized; seed first")
	}
	if err != nil {
		return 0, fmt.Errorf("lock global_active_config: %w", err)
	}
	if currentVersion != ifMatchVersion {
		return 0, ErrVersionConflict{Expected: ifMatchVersion, Current: currentVersion}
	}

	// Insert new version.
	var newVersion int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO global_config_versions (config_json, created_by)
		VALUES ($1, $2)
		RETURNING id
	`, configJSON, actorSub).Scan(&newVersion)
	if err != nil {
		return 0, fmt.Errorf("insert global_config_versions: %w", err)
	}

	// Advance the active pointer.
	_, err = tx.ExecContext(ctx, `
		UPDATE global_active_config
		SET active_version = $1, updated_at = now()
		WHERE id = 1
	`, newVersion)
	if err != nil {
		return 0, fmt.Errorf("update global_active_config: %w", err)
	}

	// Audit log.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO global_config_change_log
		    (actor_sub, actor_roles, from_version, to_version, change_summary)
		VALUES ($1, $2, $3, $4, 'PUT global config')
	`, actorSub, pq.Array(actorRoles), currentVersion, newVersion)
	if err != nil {
		return 0, fmt.Errorf("insert global_config_change_log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return newVersion, nil
}

// PatchGlobalConfig applies a JSON Merge Patch (RFC 7396) to the current global config.
// Returns the new version number. Returns ErrVersionConflict on version mismatch.
func (s *PostgresStorage) PatchGlobalConfig(
	ctx context.Context,
	ifMatchVersion int,
	mergePatchJSON json.RawMessage,
	actorSub string,
	actorRoles []string,
) (int, error) {
	// 1. Fetch current config (non-transactional read; PutGlobalConfig re-checks with lock).
	currentJSON, currentVersion, exists, err := s.GetGlobalConfig(ctx)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, fmt.Errorf("global config has not been initialized; seed first")
	}
	if currentVersion != ifMatchVersion {
		return 0, ErrVersionConflict{Expected: ifMatchVersion, Current: currentVersion}
	}

	// 2. Normalize patch (camelCase → snake_case).
	normalizedPatch, err := NormalizeJSONConfig(mergePatchJSON)
	if err != nil {
		return 0, fmt.Errorf("normalize patch: %w", err)
	}

	// 3. Apply merge patch.
	var current, patch map[string]interface{}
	if err := json.Unmarshal(currentJSON, &current); err != nil {
		return 0, fmt.Errorf("unmarshal current global config: %w", err)
	}
	if err := json.Unmarshal(normalizedPatch, &patch); err != nil {
		return 0, fmt.Errorf("unmarshal patch: %w", err)
	}
	merged := mergeMaps(current, patch)
	newConfigJSON, err := json.Marshal(merged)
	if err != nil {
		return 0, fmt.Errorf("marshal merged global config: %w", err)
	}

	finalJSON, err := FinalizeGlobalConfigJSON(newConfigJSON)
	if err != nil {
		return 0, fmt.Errorf("finalize merged global config: %w", err)
	}

	// 4. Persist via PutGlobalConfig (which re-validates the version with a lock).
	return s.PutGlobalConfig(ctx, ifMatchVersion, finalJSON, actorSub, actorRoles)
}

// RollbackGlobalConfig sets global_active_config.active_version to a previous version.
// The target version must exist in global_config_versions.
// Returns ErrVersionConflict when ifMatchVersion does not match the current active version.
func (s *PostgresStorage) RollbackGlobalConfig(
	ctx context.Context,
	ifMatchVersion, targetVersion int,
	actorSub string,
	actorRoles []string,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock and read current version.
	var currentVersion int
	err = tx.QueryRowContext(ctx,
		`SELECT active_version FROM global_active_config WHERE id = 1 FOR UPDATE`,
	).Scan(&currentVersion)
	if err == sql.ErrNoRows {
		return fmt.Errorf("global config has not been initialized; seed first")
	}
	if err != nil {
		return fmt.Errorf("lock global_active_config: %w", err)
	}
	if currentVersion != ifMatchVersion {
		return ErrVersionConflict{Expected: ifMatchVersion, Current: currentVersion}
	}

	// Ensure target version exists.
	var exists bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM global_config_versions WHERE id = $1)`,
		targetVersion,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check target version: %w", err)
	}
	if !exists {
		return fmt.Errorf("version %d not found in global_config_versions", targetVersion)
	}

	// Roll back the active pointer (no new version row — we reuse the existing one).
	_, err = tx.ExecContext(ctx, `
		UPDATE global_active_config
		SET active_version = $1, updated_at = now()
		WHERE id = 1
	`, targetVersion)
	if err != nil {
		return fmt.Errorf("update global_active_config: %w", err)
	}

	// Audit log.
	summary := fmt.Sprintf("rollback from %d to %d", currentVersion, targetVersion)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO global_config_change_log
		    (actor_sub, actor_roles, from_version, to_version, change_summary)
		VALUES ($1, $2, $3, $4, $5)
	`, actorSub, pq.Array(actorRoles), currentVersion, targetVersion, summary)
	if err != nil {
		return fmt.Errorf("insert global_config_change_log: %w", err)
	}

	return tx.Commit()
}

// ApplyGlobalConfigVersion sets the active global config to an existing version (pointer-only).
// Used by POST /admin/config/global/apply. Version must exist in global_config_versions.
func (s *PostgresStorage) ApplyGlobalConfigVersion(ctx context.Context, version int, actorSub string, actorRoles []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Ensure target version exists (id-based, not created_at).
	var exists bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM global_config_versions WHERE id = $1)`,
		version,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check version exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("version %d not found in global_config_versions", version)
	}

	// Read current version for audit log.
	var currentVersion int
	err = tx.QueryRowContext(ctx,
		`SELECT active_version FROM global_active_config WHERE id = 1 FOR UPDATE`,
	).Scan(&currentVersion)
	if err == sql.ErrNoRows {
		return fmt.Errorf("global config has not been initialized; seed first")
	}
	if err != nil {
		return fmt.Errorf("lock global_active_config: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE global_active_config
		SET active_version = $1, updated_at = now()
		WHERE id = 1
	`, version)
	if err != nil {
		return fmt.Errorf("update global_active_config: %w", err)
	}

	summary := fmt.Sprintf("apply version %d", version)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO global_config_change_log
		    (actor_sub, actor_roles, from_version, to_version, change_summary)
		VALUES ($1, $2, $3, $4, $5)
	`, actorSub, pq.Array(actorRoles), currentVersion, version, summary)
	if err != nil {
		return fmt.Errorf("insert global_config_change_log: %w", err)
	}

	return tx.Commit()
}

// SeedTenantVersionedConfig seeds tenant_config_versions + tenant_active_config on startup.
// seedMode "if_empty": only when no version exists for the tenant.
// seedMode "always":   always insert a new version and update the active pointer.
// Returns true if a new version was inserted.
func (s *PostgresStorage) SeedTenantVersionedConfig(ctx context.Context, tenantID string, configJSON json.RawMessage, seedMode string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if seedMode == "if_empty" {
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM tenant_config_versions WHERE tenant_id = $1`,
			tenantID).Scan(&count); err != nil {
			return false, fmt.Errorf("check tenant_config_versions: %w", err)
		}
		if count > 0 {
			// Versions exist — repair orphaned tenant_active_config pointer if missing.
			// This covers tenants that have version rows but lost their active pointer
			// (e.g. manual DB surgery, partial deletes, or pre-SPEC_147 inconsistency).
			_, repairErr := tx.ExecContext(ctx, `
				INSERT INTO tenant_active_config (tenant_id, active_config_id, updated_by, change_reason)
				SELECT $1, id, 'bootstrap', 'repair orphaned active pointer'
				FROM tenant_config_versions
				WHERE tenant_id = $1
				ORDER BY version DESC
				LIMIT 1
				ON CONFLICT (tenant_id) DO NOTHING
			`, tenantID)
			if repairErr != nil {
				return false, fmt.Errorf("repair tenant_active_config: %w", repairErr)
			}
			if err := tx.Commit(); err != nil {
				return false, fmt.Errorf("commit repair: %w", err)
			}
			return false, nil
		}
	}

	// Compute next version number
	var maxVersion int
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM tenant_config_versions WHERE tenant_id = $1`,
		tenantID).Scan(&maxVersion)
	newVersion := int64(maxVersion + 1)

	// Compute SHA256 of the config JSON for integrity
	sum := sha256.Sum256(configJSON)
	hashStr := hex.EncodeToString(sum[:])

	// Insert new version row (config_yaml stores the JSON text — compatible, column is TEXT)
	var versionID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO tenant_config_versions
			(tenant_id, version, config_yaml, config_sha256, created_by)
		VALUES ($1, $2, $3, $4, 'bootstrap')
		RETURNING id::text
	`, tenantID, newVersion, string(configJSON), hashStr).Scan(&versionID)
	if err != nil {
		return false, fmt.Errorf("insert tenant_config_versions: %w", err)
	}

	// Upsert active config pointer
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tenant_active_config
			(tenant_id, active_config_id, updated_by, change_reason)
		VALUES ($1, $2::uuid, 'bootstrap', 'initial bootstrap')
		ON CONFLICT (tenant_id) DO UPDATE SET
			active_config_id = EXCLUDED.active_config_id,
			updated_by       = EXCLUDED.updated_by,
			change_reason    = EXCLUDED.change_reason,
			updated_at       = now()
	`, tenantID, versionID)
	if err != nil {
		return false, fmt.Errorf("upsert tenant_active_config: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	s.log.InfoContext(ctx, "tenant versioned config seeded", "tenant_id", tenantID, "version", newVersion)
	return true, nil
}

// SeedAPIKeyFromYAML inserts a plaintext YAML API key into the api_keys table.
// Idempotent: ON CONFLICT (key_hash) DO NOTHING.
// Returns true if inserted, false if the key was already present.
func (s *PostgresStorage) SeedAPIKeyFromYAML(ctx context.Context, tenantID, apiKey string) (bool, error) {
	keyHash := hashAPIKey(apiKey)
	prefix := apiKey
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	// Use hash suffix in name to avoid (tenant_id, name) unique-index conflicts
	// when a tenant has more than one bootstrapped key.
	name := fmt.Sprintf("bootstrap-%s", keyHash[:8])

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (tenant_id, name, key_hash, prefix, scopes)
		VALUES ($1, $2, $3, $4, '["inference"]'::jsonb)
		ON CONFLICT (key_hash) DO NOTHING
	`, tenantID, name, keyHash, prefix)
	if err != nil {
		return false, fmt.Errorf("seed api key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check rows affected: %w", err)
	}

	inserted := rowsAffected > 0
	if inserted {
		s.log.InfoContext(ctx, "api key seeded from YAML", "tenant_id", tenantID, "prefix", prefix)
	}
	return inserted, nil
}

// ============================================================================
// Helper functions for JSON normalization (PascalCase → snake_case)
// ============================================================================

// toSnakeCase converts a PascalCase or camelCase string to snake_case
func toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	var result []rune
	runes := []rune(s)

	for i := 0; i < len(runes); i++ {
		if i > 0 && isUpper(runes[i]) {
			// Add underscore before uppercase letter (unless previous was also uppercase)
			if !isUpper(runes[i-1]) || (i+1 < len(runes) && isLower(runes[i+1])) {
				result = append(result, '_')
			}
		}
		result = append(result, toLower(runes[i]))
	}

	return string(result)
}

func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func isLower(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// normalizeConfigKeys recursively converts all map keys from PascalCase to snake_case
// This handles legacy configs stored in PascalCase and normalizes them for consistent API
func normalizeConfigKeys(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			snakeKey := toSnakeCase(key)
			result[snakeKey] = normalizeConfigKeys(val)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = normalizeConfigKeys(item)
		}
		return result

	default:
		// Primitives (string, number, bool, nil) pass through
		return v
	}
}

// NormalizeJSONConfig normalizes a JSON config from PascalCase to snake_case and deduplicates keys.
// Use this for incoming global/tenant config so frontend-sent "Backend" and "backend" become a single "backend".
func NormalizeJSONConfig(configJSON json.RawMessage) (json.RawMessage, error) {
	if len(configJSON) == 0 {
		return configJSON, nil
	}

	var data interface{}
	if err := json.Unmarshal(configJSON, &data); err != nil {
		return nil, fmt.Errorf("unmarshal config for normalization: %w", err)
	}

	normalized := normalizeConfigKeys(data)

	result, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized config: %w", err)
	}

	return result, nil
}

// FinalizeGlobalConfigJSON normalizes keys (PascalCase → snake_case), ensures model entries
// use provider_model_id (not legacy ProviderModelID), re-encodes each entry under "providers"
// to canonical JSON field names (Type, BaseURL, APIKeyEnv, Enabled), and removes duplicate
// legacy keys (e.g. base_url next to BaseURL). Use for full global config payloads before
// persist (PUT and post-merge PATCH), not for small merge fragments alone.
func FinalizeGlobalConfigJSON(configJSON json.RawMessage) (json.RawMessage, error) {
	if len(configJSON) == 0 {
		return configJSON, nil
	}

	var data interface{}
	if err := json.Unmarshal(configJSON, &data); err != nil {
		return nil, fmt.Errorf("unmarshal config for finalize: %w", err)
	}

	data = normalizeConfigKeys(data)
	data = canonicalizeGlobalConfigModels(data)
	data = canonicalizeGlobalConfigProviders(data)

	out, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal finalized global config: %w", err)
	}
	return out, nil
}

// canonicalizeGlobalConfigModels copies legacy "ProviderModelID" into "provider_model_id" when
// needed and removes the PascalCase key so persisted global config uses snake_case only.
func canonicalizeGlobalConfigModels(data interface{}) interface{} {
	root, ok := data.(map[string]interface{})
	if !ok {
		return data
	}
	modelsRaw, ok := root["models"]
	if !ok {
		return data
	}
	arr, ok := modelsRaw.([]interface{})
	if !ok {
		return data
	}
	for i, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if _, hasSnake := m["provider_model_id"]; !hasSnake {
			if v, ok := m["ProviderModelID"]; ok {
				m["provider_model_id"] = v
			}
		}
		delete(m, "ProviderModelID")
		arr[i] = m
	}
	root["models"] = arr
	return data
}

// canonicalizeGlobalConfigProviders re-encodes each entry under "providers" using config.ProviderConfig
// so persisted JSON has only canonical Go/JSON field names (Type, BaseURL, APIKeyEnv, Enabled) and
// drops legacy duplicate keys (e.g. base_url next to BaseURL).
func canonicalizeGlobalConfigProviders(data interface{}) interface{} {
	root, ok := data.(map[string]interface{})
	if !ok {
		return data
	}
	provRaw, ok := root["providers"]
	if !ok {
		return data
	}
	provMap, ok := provRaw.(map[string]interface{})
	if !ok {
		return data
	}
	for name, entry := range provMap {
		if m, ok := entry.(map[string]interface{}); ok {
			provMap[name] = canonicalizeProviderConfigMap(m)
		}
	}
	root["providers"] = provMap
	return data
}

func canonicalizeProviderConfigMap(m map[string]interface{}) map[string]interface{} {
	if len(m) == 0 {
		return m
	}
	pc := providerConfigFromFlexibleMap(m)
	out, err := json.Marshal(&pc)
	if err != nil {
		return m
	}
	var cleaned map[string]interface{}
	if err := json.Unmarshal(out, &cleaned); err != nil {
		return m
	}
	return cleaned
}

func providerConfigFromFlexibleMap(m map[string]interface{}) config.ProviderConfig {
	var pc config.ProviderConfig
	pc.Type = stringFieldFromMap(m, "Type", "type")
	pc.BaseURL = stringFieldFromMap(m, "BaseURL", "base_url")
	pc.APIKeyEnv = stringFieldFromMap(m, "APIKeyEnv", "api_key_env")
	pc.Enabled = boolPtrFromMap(m, "Enabled", "enabled")
	pc.AwsAccessKeyID = stringFieldFromMap(m, "AwsAccessKeyID", "aws_access_key_id")
	pc.AwsSecretAccessKey = stringFieldFromMap(m, "AwsSecretAccessKey", "aws_secret_access_key")
	pc.AwsRegion = stringFieldFromMap(m, "AwsRegion", "aws_region")
	return pc
}

func stringFieldFromMap(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func boolPtrFromMap(m map[string]interface{}, keys ...string) *bool {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch b := v.(type) {
		case bool:
			return &b
		case float64:
			if b == 0 {
				f := false
				return &f
			}
			t := true
			return &t
		}
	}
	return nil
}

// ============================================================================
// Helper functions for JSON Merge Patch
// ============================================================================

// mergeMaps implements JSON Merge Patch (RFC 7396)
func mergeMaps(target, patch map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy target
	for k, v := range target {
		result[k] = v
	}

	// Apply patch
	for k, v := range patch {
		if v == nil {
			delete(result, k)
		} else if targetMap, ok := result[k].(map[string]interface{}); ok {
			if patchMap, ok := v.(map[string]interface{}); ok {
				result[k] = mergeMaps(targetMap, patchMap)
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}

	return result
}

// computeDiff generates a simple diff between old and new config
func computeDiff(old, new map[string]interface{}) map[string]map[string]interface{} {
	diff := make(map[string]map[string]interface{})

	// Track changed top-level keys
	for k, newVal := range new {
		oldVal, exists := old[k]
		if !exists || !reflect.DeepEqual(oldVal, newVal) {
			diff[k] = map[string]interface{}{
				"old": oldVal,
				"new": newVal,
			}
		}
	}

	// Track deleted keys
	for k := range old {
		if _, exists := new[k]; !exists {
			diff[k] = map[string]interface{}{
				"old": old[k],
				"new": nil,
			}
		}
	}

	return diff
}

// ============================================================================
// API Key Management
// ============================================================================

// generateAPIKey creates a new API key with format: rk_<env>_<random>
// Returns (plaintext, prefix, hash, error)
func generateAPIKey() (string, string, string, error) {
	env := os.Getenv("API_KEY_ENV")
	if env == "" {
		env = "live"
	}

	randomBytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", "", fmt.Errorf("generate random bytes: %w", err)
	}

	randomStr := base64.RawURLEncoding.EncodeToString(randomBytes) // 43 chars
	plaintext := fmt.Sprintf("rk_%s_%s", env, randomStr)
	prefix := plaintext[:min(12, len(plaintext))] // First 12 chars for display

	hash := sha256.Sum256([]byte(plaintext))
	hashStr := hex.EncodeToString(hash[:])

	return plaintext, prefix, hashStr, nil
}

// hashAPIKey computes SHA256 hash of plaintext key
func hashAPIKey(plaintext string) string {
	hash := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(hash[:])
}

// deriveAPIKeyMetadata extracts prefix and hash from an existing plaintext key.
// Used for bootstrapping an API key from an env var (plaintext).
func deriveAPIKeyMetadata(plaintext string) (string, string, error) {
	if plaintext == "" {
		return "", "", fmt.Errorf("plaintext key cannot be empty")
	}

	prefix := plaintext[:min(20, len(plaintext))] // First 20 chars for display
	hash := hashAPIKey(plaintext)

	return prefix, hash, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CreateAPIKey generates a new API key with SHA256 hashing
func (s *PostgresStorage) CreateAPIKey(ctx context.Context, tenantID, name string, scopes []string, expiresAt *time.Time, actorSub string, actorRoles []string) (APIKeyCreateResult, error) {
	// Generate key
	plaintext, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("generate key: %w", err)
	}

	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert API key
	keyID := uuid.New()
	scopesJSON, _ := json.Marshal(scopes)
	createdAt := time.Now().UTC()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO api_keys (id, tenant_id, name, key_hash, prefix, scopes, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		keyID, tenantID, name, keyHash, prefix, scopesJSON, expiresAt, createdAt)
	if err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("insert api key: %w", err)
	}

	// Insert audit log
	diffJSON, _ := json.Marshal(map[string]interface{}{
		"action":     "create_api_key",
		"api_key_id": keyID.String(),
		"prefix":     prefix,
		"scopes":     scopes,
		"expires_at": expiresAt,
	})

	_, err = tx.ExecContext(ctx,
		`INSERT INTO config_change_log (tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json)
		 VALUES ($1, $2, $3, 0, 0, $4, $5)`,
		tenantID, actorSub, pq.Array(actorRoles), "Create API key: "+name, diffJSON)
	if err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("insert audit log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("commit transaction: %w", err)
	}

	// Note: Logging is done at HTTP handler level (admin_api_keys.go) to avoid duplication

	return APIKeyCreateResult{
		APIKeyMeta: APIKeyMeta{
			ID:        keyID,
			TenantID:  tenantID,
			Name:      name,
			Prefix:    prefix,
			Scopes:    scopes,
			CreatedAt: createdAt,
			ExpiresAt: expiresAt,
		},
		Key: plaintext,
	}, nil
}

// CreateAPIKeyFromPlaintext creates an API key using a provided plaintext (e.g., from env var).
// Used by bootstrap to ensure env-based keys are stored with the correct hash.
// This is the same as CreateAPIKey except it uses provided plaintext instead of generating a random one.
func (s *PostgresStorage) CreateAPIKeyFromPlaintext(ctx context.Context, tenantID, name, plaintextKey string, scopes []string, expiresAt *time.Time, actorSub string, actorRoles []string) (APIKeyCreateResult, error) {
	if plaintextKey == "" {
		return APIKeyCreateResult{}, fmt.Errorf("plaintext key cannot be empty")
	}

	// Derive prefix and hash from provided plaintext
	prefix, keyHash, err := deriveAPIKeyMetadata(plaintextKey)
	if err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("derive key metadata: %w", err)
	}

	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert API key
	keyID := uuid.New()
	scopesJSON, _ := json.Marshal(scopes)
	createdAt := time.Now().UTC()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO api_keys (id, tenant_id, name, key_hash, prefix, scopes, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		keyID, tenantID, name, keyHash, prefix, scopesJSON, expiresAt, createdAt)
	if err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("insert api key: %w", err)
	}

	// Insert audit log
	diffJSON, _ := json.Marshal(map[string]interface{}{
		"action":     "create_api_key_from_plaintext",
		"api_key_id": keyID.String(),
		"prefix":     prefix,
		"scopes":     scopes,
		"expires_at": expiresAt,
	})

	_, err = tx.ExecContext(ctx,
		`INSERT INTO config_change_log (tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json)
		 VALUES ($1, $2, $3, 0, 0, $4, $5)`,
		tenantID, actorSub, pq.Array(actorRoles), "Create API key from plaintext: "+name, diffJSON)
	if err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("insert audit log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return APIKeyCreateResult{}, fmt.Errorf("commit transaction: %w", err)
	}

	return APIKeyCreateResult{
		APIKeyMeta: APIKeyMeta{
			ID:        keyID,
			TenantID:  tenantID,
			Name:      name,
			Prefix:    prefix,
			Scopes:    scopes,
			CreatedAt: createdAt,
			ExpiresAt: expiresAt,
		},
		Key: plaintextKey,
	}, nil
}

// CountAPIKeys returns the total number of API keys across all tenants
func (s *PostgresStorage) CountAPIKeys(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count api keys: %w", err)
	}
	return count, nil
}

// ListAPIKeys retrieves all API keys for a tenant (never includes plaintext or hash)
func (s *PostgresStorage) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKeyMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, prefix, scopes, created_at, expires_at, revoked_at, last_used_at
		 FROM api_keys
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC`,
		tenantID)
	if err != nil {
		return nil, fmt.Errorf("query api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKeyMeta
	for rows.Next() {
		var k APIKeyMeta
		var scopesJSON []byte

		err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.Prefix, &scopesJSON, &k.CreatedAt, &k.ExpiresAt, &k.RevokedAt, &k.LastUsedAt)
		if err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}

		if err := json.Unmarshal(scopesJSON, &k.Scopes); err != nil {
			return nil, fmt.Errorf("unmarshal scopes: %w", err)
		}

		keys = append(keys, k)
	}

	return keys, rows.Err()
}

// ListAPIKeysPaged retrieves API keys for a tenant with pagination and optional revoked filter.
func (s *PostgresStorage) ListAPIKeysPaged(ctx context.Context, tenantID string, includeRevoked bool, limit, offset int) ([]APIKeyMeta, bool, error) {
	query := `SELECT id, tenant_id, name, prefix, scopes, created_at, expires_at, revoked_at, last_used_at
		FROM api_keys
		WHERE tenant_id = $1`
	if !includeRevoked {
		query += ` AND revoked_at IS NULL`
	}
	query += ` ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := s.db.QueryContext(ctx, query, tenantID, limit+1, offset)
	if err != nil {
		return nil, false, fmt.Errorf("query api keys paged: %w", err)
	}
	defer rows.Close()

	var keys []APIKeyMeta
	for rows.Next() {
		var k APIKeyMeta
		var scopesJSON []byte
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.Prefix, &scopesJSON, &k.CreatedAt, &k.ExpiresAt, &k.RevokedAt, &k.LastUsedAt); err != nil {
			return nil, false, fmt.Errorf("scan api key: %w", err)
		}
		if err := json.Unmarshal(scopesJSON, &k.Scopes); err != nil {
			return nil, false, fmt.Errorf("unmarshal scopes: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(keys) > limit
	if hasMore {
		keys = keys[:limit]
	}
	return keys, hasMore, nil
}

// RevokeAPIKey marks an API key as revoked (sets revoked_at timestamp)
func (s *PostgresStorage) RevokeAPIKey(ctx context.Context, tenantID string, keyID uuid.UUID, actorSub string, actorRoles []string) (*time.Time, error) {
	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get key info for audit
	var name, prefix string
	err = tx.QueryRowContext(ctx,
		`SELECT name, prefix FROM api_keys WHERE id = $1 AND tenant_id = $2`,
		keyID, tenantID).Scan(&name, &prefix)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("api key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query api key: %w", err)
	}

	// Revoke key
	revokedAt := time.Now().UTC()
	result, err := tx.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = $1 WHERE id = $2 AND tenant_id = $3 AND revoked_at IS NULL`,
		revokedAt, keyID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("revoke api key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, fmt.Errorf("api key already revoked or not found")
	}

	// Insert audit log
	diffJSON, _ := json.Marshal(map[string]interface{}{
		"action":     "revoke_api_key",
		"api_key_id": keyID.String(),
		"prefix":     prefix,
		"revoked_at": revokedAt,
	})

	_, err = tx.ExecContext(ctx,
		`INSERT INTO config_change_log (tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json)
		 VALUES ($1, $2, $3, 0, 0, $4, $5)`,
		tenantID, actorSub, pq.Array(actorRoles), "Revoke API key: "+name, diffJSON)
	if err != nil {
		return nil, fmt.Errorf("insert audit log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	s.log.Info("api key revoked",
		"tenant_id", tenantID,
		"key_id", keyID.String(),
		"name", name,
		"prefix", prefix)

	return &revokedAt, nil
}

// RotateAPIKey atomically creates a new key and revokes the old one
func (s *PostgresStorage) RotateAPIKey(ctx context.Context, tenantID string, keyID uuid.UUID, actorSub string, actorRoles []string) (uuid.UUID, APIKeyCreateResult, error) {
	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get old key info
	var oldName string
	var oldPrefix string
	var oldScopes []byte
	var oldExpiresAt *time.Time
	err = tx.QueryRowContext(ctx,
		`SELECT name, prefix, scopes, expires_at FROM api_keys WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL`,
		keyID, tenantID).Scan(&oldName, &oldPrefix, &oldScopes, &oldExpiresAt)
	if err == sql.ErrNoRows {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("api key not found or already revoked")
	}
	if err != nil {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("query api key: %w", err)
	}

	var scopes []string
	if err := json.Unmarshal(oldScopes, &scopes); err != nil {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("unmarshal scopes: %w", err)
	}

	// Generate new key
	plaintext, prefix, keyHash, err := generateAPIKey()
	if err != nil {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("generate key: %w", err)
	}

	// CRITICAL: Revoke old key FIRST to avoid unique constraint violation
	// The unique index idx_api_keys_tenant_name_active is on (tenant_id, name) WHERE revoked_at IS NULL
	// We must revoke the old key before inserting the new one with the same name
	newKeyID := uuid.New()
	createdAt := time.Now().UTC()
	revokedAt := createdAt

	_, err = tx.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = $1, metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{rotated_to}', $2)
		 WHERE id = $3 AND tenant_id = $4`,
		revokedAt, fmt.Sprintf(`"%s"`, newKeyID.String()), keyID, tenantID)
	if err != nil {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("revoke old api key: %w", err)
	}

	// Now insert new key (old key is revoked, no longer in unique index)
	scopesJSON, _ := json.Marshal(scopes)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO api_keys (id, tenant_id, name, key_hash, prefix, scopes, expires_at, created_at, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		newKeyID, tenantID, oldName, keyHash, prefix, scopesJSON, oldExpiresAt, createdAt,
		json.RawMessage(fmt.Sprintf(`{"rotated_from": "%s"}`, keyID.String())))
	if err != nil {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("insert new api key: %w", err)
	}

	// Insert audit log
	diffJSON, _ := json.Marshal(map[string]interface{}{
		"action":     "rotate_api_key",
		"old_key_id": keyID.String(),
		"old_prefix": oldPrefix,
		"new_key_id": newKeyID.String(),
		"new_prefix": prefix,
		"revoked_at": revokedAt,
		"scopes":     scopes,
		"expires_at": oldExpiresAt,
	})

	_, err = tx.ExecContext(ctx,
		`INSERT INTO config_change_log (tenant_id, actor_sub, actor_roles, from_version, to_version, change_summary, diff_json)
		 VALUES ($1, $2, $3, 0, 0, $4, $5)`,
		tenantID, actorSub, pq.Array(actorRoles), "Rotate API key: "+oldName, diffJSON)
	if err != nil {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("insert audit log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return uuid.Nil, APIKeyCreateResult{}, fmt.Errorf("commit transaction: %w", err)
	}

	// Note: Logging is done at HTTP handler level (admin_api_keys.go) to avoid duplication

	return keyID, APIKeyCreateResult{
		APIKeyMeta: APIKeyMeta{
			ID:        newKeyID,
			TenantID:  tenantID,
			Name:      oldName,
			Prefix:    prefix,
			Scopes:    scopes,
			CreatedAt: createdAt,
			ExpiresAt: oldExpiresAt,
		},
		Key: plaintext,
	}, nil
}

// LookupAPIKeyByHash retrieves an active API key by its SHA256 hash
func (s *PostgresStorage) LookupAPIKeyByHash(ctx context.Context, keyHash string) (APIKeyRecord, bool, error) {
	var k APIKeyRecord
	var scopesJSON []byte
	var metadataJSON []byte

	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, key_hash, prefix, scopes, created_at, expires_at, revoked_at, last_used_at, metadata
		 FROM api_keys
		 WHERE key_hash = $1
		   AND revoked_at IS NULL
		   AND (expires_at IS NULL OR expires_at > NOW())`,
		keyHash).Scan(&k.ID, &k.TenantID, &k.Name, &k.KeyHash, &k.Prefix, &scopesJSON, &k.CreatedAt, &k.ExpiresAt, &k.RevokedAt, &k.LastUsedAt, &metadataJSON)

	if err == sql.ErrNoRows {
		return APIKeyRecord{}, false, nil
	}
	if err != nil {
		return APIKeyRecord{}, false, fmt.Errorf("query api key: %w", err)
	}

	if err := json.Unmarshal(scopesJSON, &k.Scopes); err != nil {
		return APIKeyRecord{}, false, fmt.Errorf("unmarshal scopes: %w", err)
	}

	k.Metadata = metadataJSON

	return k, true, nil
}

// TouchAPIKeyLastUsed updates the last_used_at timestamp (best-effort, non-blocking)
func (s *PostgresStorage) TouchAPIKeyLastUsed(ctx context.Context, keyID uuid.UUID, ts time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = $1 WHERE id = $2`,
		ts, keyID)
	// Ignore errors - this is best-effort
	return err
}

// CreateTenant creates a new tenant with empty initial configuration.
// Inserts version=1 into tenant_config_versions and sets the active pointer.
// Returns ErrTenantAlreadyExists if the tenant already exists in either config table.
func (s *PostgresStorage) CreateTenant(ctx context.Context, tenantID string, initialConfig json.RawMessage, actorSub string, actorRoles []string) error {
	// Check existence in versioned tables first (authoritative).
	var existsVersioned bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM tenant_active_config WHERE tenant_id = $1)`,
		tenantID).Scan(&existsVersioned); err != nil {
		return fmt.Errorf("check tenant existence (versioned): %w", err)
	}
	if existsVersioned {
		return ErrTenantAlreadyExists{TenantID: tenantID}
	}

	// Also check flat legacy table.
	var existsFlat bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM tenants_config WHERE tenant_id = $1)`,
		tenantID).Scan(&existsFlat); err != nil {
		return fmt.Errorf("check tenant existence (flat): %w", err)
	}
	if existsFlat {
		return ErrTenantAlreadyExists{TenantID: tenantID}
	}

	// Use the supplied initial config (caller sets environment and defaults).
	sum := sha256.Sum256(initialConfig)
	hashStr := hex.EncodeToString(sum[:])

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	var versionID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO tenant_config_versions
			(tenant_id, version, config_yaml, config_sha256, created_by, comment)
		VALUES ($1, 1, $2, $3, $4, 'created via admin API')
		RETURNING id::text
	`, tenantID, string(initialConfig), hashStr, actorSub).Scan(&versionID); err != nil {
		return fmt.Errorf("insert tenant_config_versions: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tenant_active_config
			(tenant_id, active_config_id, updated_by, change_reason)
		VALUES ($1, $2::uuid, $3, 'created via admin API')
	`, tenantID, versionID, actorSub); err != nil {
		return fmt.Errorf("insert tenant_active_config: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	s.log.InfoContext(ctx, "tenant created", "tenant_id", tenantID, "actor", actorSub)
	return nil
}

// DeleteTenant removes all configuration data for a tenant from the database.
// Deletes from tenant_active_config (first, due to FK), tenant_config_versions,
// config_change_log, and tenants_config (flat/legacy table).
// Returns (true, nil) if deleted, (false, nil) if not found in any config table.
func (s *PostgresStorage) DeleteTenant(ctx context.Context, tenantID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check existence in both tables before committing.
	var existsVersioned, existsFlat bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM tenant_active_config WHERE tenant_id = $1)`,
		tenantID).Scan(&existsVersioned); err != nil {
		return false, fmt.Errorf("check tenant_active_config: %w", err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM tenants_config WHERE tenant_id = $1)`,
		tenantID).Scan(&existsFlat); err != nil {
		return false, fmt.Errorf("check tenants_config: %w", err)
	}
	if !existsVersioned && !existsFlat {
		return false, nil
	}

	// 1. Delete active config pointer first (FK RESTRICT on tenant_config_versions.id).
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tenant_active_config WHERE tenant_id = $1`, tenantID); err != nil {
		return false, fmt.Errorf("delete tenant_active_config: %w", err)
	}

	// 2. Delete all config versions.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tenant_config_versions WHERE tenant_id = $1`, tenantID); err != nil {
		return false, fmt.Errorf("delete tenant_config_versions: %w", err)
	}

	// 3. Delete change log entries.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM config_change_log WHERE tenant_id = $1`, tenantID); err != nil {
		return false, fmt.Errorf("delete config_change_log: %w", err)
	}

	// 4. Delete flat legacy config.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tenants_config WHERE tenant_id = $1`, tenantID); err != nil {
		return false, fmt.Errorf("delete tenants_config: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	s.log.InfoContext(ctx, "tenant deleted", "tenant_id", tenantID)
	return true, nil
}

// GetTenantUsageOverview returns total request count, token count, and cost for a tenant.
// Uses request_log for request counts (attempt=1) and usage for tokens/cost.
func (s *PostgresStorage) GetTenantUsageOverview(ctx context.Context, tenantID string, from, to time.Time) (TenantUsageOverview, error) {
	var ov TenantUsageOverview

	// Request count: one row per logical request (attempt=1).
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM request_log
		WHERE tenant_id = $1 AND ts >= $2 AND ts < $3 AND attempt = 1
	`, tenantID, from, to).Scan(&ov.TotalRequests); err != nil {
		return ov, fmt.Errorf("count requests: %w", err)
	}

	// Token + cost totals from usage table.
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total_tokens), 0), COALESCE(SUM(cost_usd), 0)
		FROM usage
		WHERE tenant_id = $1 AND ts >= $2 AND ts < $3
	`, tenantID, from, to).Scan(&ov.TotalTokens, &ov.TotalCostUSD); err != nil {
		return ov, fmt.Errorf("sum tokens/cost: %w", err)
	}

	return ov, nil
}

// GetModelRequestCounts returns total request count per model for the last windowDays days,
// aggregated across all tenants from model_stats_daily.
func (s *PostgresStorage) GetModelRequestCounts(ctx context.Context, windowDays int) (map[string]int64, error) {
	startDate := time.Now().UTC().AddDate(0, 0, -windowDays)

	rows, err := s.db.QueryContext(ctx, `
		SELECT model, SUM(request_count) AS total
		FROM model_stats_daily
		WHERE date >= $1
		GROUP BY model
		ORDER BY total DESC
	`, startDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("query model_stats_daily: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var model string
		var total int64
		if err := rows.Scan(&model, &total); err != nil {
			return nil, fmt.Errorf("scan model row: %w", err)
		}
		counts[model] = total
	}
	return counts, rows.Err()
}

// ListRecentRequests returns request_log rows (attempt=1) in the last windowHours hours,
// ordered by timestamp descending. tenantID="" returns all tenants. windowHours <= 0 defaults to 24.
func (s *PostgresStorage) ListRecentRequests(ctx context.Context, tenantID string, windowHours, limit, offset int) ([]RequestListRow, bool, error) {
	if windowHours <= 0 {
		windowHours = 24
	}
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	// Fetch one extra row to determine hasMore.
	rows, err := s.db.QueryContext(ctx, `
		SELECT request_id, tenant_id, model, ts, provider, status, latency_ms, strategy, fallback_used,
		       error_type, decision_reason
		FROM request_log
		WHERE ($1 = '' OR tenant_id = $1) AND attempt = 1 AND ts >= $2
		ORDER BY ts DESC
		LIMIT $3 OFFSET $4
	`, tenantID, since, limit+1, offset)
	if err != nil {
		return nil, false, fmt.Errorf("query request_log: %w", err)
	}
	defer rows.Close()

	var result []RequestListRow
	for rows.Next() {
		var r RequestListRow
		var latencyMs sql.NullInt64
		var errorType, decisionReason sql.NullString
		if err := rows.Scan(
			&r.RequestID, &r.TenantID, &r.Model, &r.CreatedAt,
			&r.Provider, &r.Status, &latencyMs, &r.Strategy, &r.FallbackUsed,
			&errorType, &decisionReason,
		); err != nil {
			return nil, false, fmt.Errorf("scan request row: %w", err)
		}
		if latencyMs.Valid {
			r.LatencyMs = int(latencyMs.Int64)
		}
		if errorType.Valid {
			r.ErrorType = &errorType.String
		}
		if decisionReason.Valid {
			r.DecisionReason = &decisionReason.String
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

// ListRequestLogRecent returns paginated request_log rows with optional filters.
// When From and To are both nil, defaults to last 24h. Order: ts DESC.
func (s *PostgresStorage) ListRequestLogRecent(ctx context.Context, filter RequestLogRecentFilter, limit, offset int) ([]RequestLogRecentRow, int, error) {
	fromT := time.Now().UTC().Add(-24 * time.Hour)
	toT := time.Now().UTC()
	if filter.From != nil {
		fromT = *filter.From
	}
	if filter.To != nil {
		toT = *filter.To
	}

	args := []interface{}{
		nilToNullString(filter.TenantID),
		nilToNullString(filter.JWTSub),
		nilToNullString(filter.Model),
		nilToNullString(filter.Provider),
		nilToNullString(filter.Status),
		nilToNullBool(filter.FallbackUsed),
		fromT,
		toT,
		nilToNullString(filter.Strategy),
		nilToNullString(filter.WorkflowID),
		nilToNullString(filter.ConversationID),
	}

	baseQuery := `
		SELECT request_id, ts, tenant_id,
		       COALESCE(jwt_sub, '') AS jwt_sub,
		       COALESCE(api_key_id::text, '') AS api_key_id,
		       COALESCE(api_key_name, '') AS api_key_name,
		       model, provider, strategy, latency_ms, status, fallback_used, attempt,
		       COALESCE(decision_reason, '') AS decision_reason,
		       COALESCE(error_type, '') AS error_type,
		       COALESCE(error, '') AS error,
		       routing_snapshot,
		       decision_snapshot,
		       metadata,
		       COALESCE(workflow_id, '') AS workflow_id,
		       COALESCE(conversation_id, '') AS conversation_id
		FROM request_log
		WHERE ($1::text IS NULL OR tenant_id = $1)
		  AND ($2::text IS NULL OR jwt_sub = $2)
		  AND ($3::text IS NULL OR model = $3)
		  AND ($4::text IS NULL OR provider = $4)
		  AND ($5::text IS NULL OR status = $5)
		  AND ($6::bool IS NULL OR fallback_used = $6)
		  AND ts >= $7 AND ts <= $8
		  AND ($9::text IS NULL OR strategy = $9)
		  AND ($10::text IS NULL OR workflow_id = $10)
		  AND ($11::text IS NULL OR conversation_id = $11)
		ORDER BY ts DESC
	`

	// Count total matching rows
	var total int
	countQuery := `SELECT COUNT(*) FROM request_log WHERE
		($1::text IS NULL OR tenant_id = $1)
		AND ($2::text IS NULL OR jwt_sub = $2)
		AND ($3::text IS NULL OR model = $3)
		AND ($4::text IS NULL OR provider = $4)
		AND ($5::text IS NULL OR status = $5)
		AND ($6::bool IS NULL OR fallback_used = $6)
		AND ts >= $7 AND ts <= $8
		AND ($9::text IS NULL OR strategy = $9)
		AND ($10::text IS NULL OR workflow_id = $10)
		AND ($11::text IS NULL OR conversation_id = $11)`
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count request_log: %w", err)
	}

	// Fetch page
	dataQuery := baseQuery + ` LIMIT $12 OFFSET $13`
	dataArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query request_log: %w", err)
	}
	defer rows.Close()

	var result []RequestLogRecentRow
	for rows.Next() {
		var r RequestLogRecentRow
		var routingSnap, decisionSnap, metaBytes []byte
		if err := rows.Scan(
			&r.RequestID, &r.Timestamp, &r.TenantID, &r.JWTSub, &r.APIKeyID, &r.APIKeyName,
			&r.Model, &r.Provider, &r.Strategy, &r.LatencyMs, &r.Status, &r.FallbackUsed, &r.Attempt,
			&r.DecisionReason, &r.ErrorType, &r.Error,
			&routingSnap, &decisionSnap, &metaBytes,
			&r.WorkflowID, &r.ConversationID,
		); err != nil {
			return nil, 0, fmt.Errorf("scan request_log row: %w", err)
		}
		if len(routingSnap) > 0 {
			r.RoutingSnapshot = json.RawMessage(routingSnap)
		}
		if len(decisionSnap) > 0 {
			r.DecisionSnapshot = json.RawMessage(decisionSnap)
		}
		if len(metaBytes) > 0 {
			r.Metadata = json.RawMessage(metaBytes)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// ListComplianceEvents returns paginated compliance_event_log rows with optional filters.
func (s *PostgresStorage) ListComplianceEvents(ctx context.Context, filter ComplianceEventFilter, limit, offset int) ([]ComplianceEventLog, int, error) {
	fromT := interface{}(nil)
	toT := interface{}(nil)
	if filter.From != nil {
		fromT = *filter.From
	}
	if filter.To != nil {
		toT = *filter.To
	}
	args := []interface{}{
		fromT,
		toT,
		nilToNullString(filter.TenantID),
		nilToNullString(filter.RequestID),
		nilToNullString(filter.EventType),
	}
	baseWhere := `
		($1::timestamptz IS NULL OR created_at >= $1)
		AND ($2::timestamptz IS NULL OR created_at <= $2)
		AND ($3::text IS NULL OR tenant_id = $3)
		AND ($4::text IS NULL OR request_id = $4)
		AND ($5::text IS NULL OR event_type = $5)
	`
	var total int
	countQuery := `SELECT COUNT(*) FROM compliance_event_log WHERE ` + baseWhere
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count compliance events: %w", err)
	}
	dataQuery := `
		SELECT id, tenant_id, request_id, event_type, action_taken, metadata, created_at
		FROM compliance_event_log
		WHERE ` + baseWhere + `
		ORDER BY created_at DESC
		LIMIT $6 OFFSET $7`
	rows, err := s.db.QueryContext(ctx, dataQuery, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query compliance events: %w", err)
	}
	defer rows.Close()
	out := make([]ComplianceEventLog, 0, limit)
	for rows.Next() {
		var r ComplianceEventLog
		if err := rows.Scan(&r.ID, &r.TenantID, &r.RequestID, &r.EventType, &r.ActionTaken, &r.Metadata, &r.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan compliance event row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ListConversations returns paginated conversation_log rows with optional filters.
func (s *PostgresStorage) ListConversations(ctx context.Context, filter ConversationLogFilter, limit, offset int) ([]ConversationLog, int, error) {
	var fromT, toT interface{}
	if filter.From != nil {
		fromT = *filter.From
	}
	if filter.To != nil {
		toT = *filter.To
	}
	args := []interface{}{
		fromT,
		toT,
		nilToNullString(filter.TenantID),
		nilToNullString(filter.JWTSub),
		nilToNullString(filter.WorkflowID),
		nilToNullString(filter.ConversationID),
	}
	baseWhere := `
		($1::timestamptz IS NULL OR created_at >= $1)
		AND ($2::timestamptz IS NULL OR created_at <= $2)
		AND ($3::text IS NULL OR tenant_id = $3)
		AND ($4::text IS NULL OR jwt_sub = $4)
		AND ($5::text IS NULL OR workflow_id = $5)
		AND ($6::text IS NULL OR conversation_id = $6)
	`
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_log WHERE `+baseWhere, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count conversations: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, request_id, tenant_id, jwt_sub, workflow_id, conversation_id, customer_id,
		       prompt_preview, response_preview, prompt_redacted, response_redacted, prompt_full, response_full,
		       pii_detected, logging_mode, created_at, enc_key_version, prompt_redacted_enc, response_redacted_enc, prompt_full_enc, response_full_enc
		FROM conversation_log
		WHERE `+baseWhere+`
		ORDER BY created_at DESC
		LIMIT $7 OFFSET $8`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()
	out := make([]ConversationLog, 0, limit)
	decryptField := func(ciphertext []byte, keyVersion sql.NullString, fallback sql.NullString) (*string, error) {
		if ciphertext != nil {
			if len(ciphertext) == 0 {
				return nil, fmt.Errorf("encrypted field is empty")
			}
			if !keyVersion.Valid || keyVersion.String == "" {
				return nil, fmt.Errorf("missing key version")
			}
			if s.encService == nil {
				return nil, fmt.Errorf("conversation log encryption not configured")
			}
			plain, err := s.encService.DecryptString(ciphertext, keyVersion.String)
			if err != nil {
				return nil, err
			}
			return &plain, nil
		}
		if fallback.Valid {
			v := fallback.String
			return &v, nil
		}
		return nil, nil
	}

	for rows.Next() {
		var r ConversationLog
		var workflowID, conversationID sql.NullString
		var promptRedacted, responseRedacted, promptFull, responseFull sql.NullString
		var encKeyVersion sql.NullString
		var promptRedactedEnc, responseRedactedEnc, promptFullEnc, responseFullEnc []byte
		if err := rows.Scan(
			&r.ID, &r.RequestID, &r.TenantID, &r.JWTSub, &workflowID, &conversationID, &r.CustomerID,
			&r.PromptPreview, &r.ResponsePreview,
			&promptRedacted, &responseRedacted, &promptFull, &responseFull,
			&r.PIIDetected, &r.LoggingMode, &r.CreatedAt,
			&encKeyVersion, &promptRedactedEnc, &responseRedactedEnc, &promptFullEnc, &responseFullEnc,
		); err != nil {
			return nil, 0, fmt.Errorf("scan conversations row: %w", err)
		}
		if workflowID.Valid {
			r.WorkflowID = &workflowID.String
		}
		if conversationID.Valid {
			r.ConversationID = &conversationID.String
		}
		var decErr error
		r.PromptRedacted, decErr = decryptField(promptRedactedEnc, encKeyVersion, promptRedacted)
		if decErr != nil {
			return nil, 0, fmt.Errorf("decrypt prompt_redacted: %w", decErr)
		}
		r.ResponseRedacted, decErr = decryptField(responseRedactedEnc, encKeyVersion, responseRedacted)
		if decErr != nil {
			return nil, 0, fmt.Errorf("decrypt response_redacted: %w", decErr)
		}
		r.PromptFull, decErr = decryptField(promptFullEnc, encKeyVersion, promptFull)
		if decErr != nil {
			return nil, 0, fmt.Errorf("decrypt prompt_full: %w", decErr)
		}
		r.ResponseFull, decErr = decryptField(responseFullEnc, encKeyVersion, responseFull)
		if decErr != nil {
			return nil, 0, fmt.Errorf("decrypt response_full: %w", decErr)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ListAPIKeyRawUsage returns raw per-request rows from request_log (with api_key_id) joined to usage.
func (s *PostgresStorage) ListAPIKeyRawUsage(ctx context.Context, filter APIKeyRawUsageFilter) ([]APIKeyRawUsageRow, int, error) {
	var fromT, toT interface{}
	if filter.From != nil {
		fromT = *filter.From
	}
	if filter.To != nil {
		toT = *filter.To
	}
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}
	apiKeyNameArg := interface{}(nil)
	if filter.APIKeyName != "" {
		apiKeyNameArg = filter.APIKeyName
	}
	modelArg := interface{}(nil)
	if filter.Model != "" {
		modelArg = filter.Model
	}
	providerArg := interface{}(nil)
	if filter.Provider != "" {
		providerArg = filter.Provider
	}
	statusArg := interface{}(nil)
	if filter.Status != "" {
		statusArg = filter.Status
	}
	args := []interface{}{fromT, toT, tenantArg, apiKeyNameArg, modelArg, providerArg, statusArg}

	baseWhere := `rl.api_key_id IS NOT NULL
	  AND ($1::timestamptz IS NULL OR rl.ts >= $1)
	  AND ($2::timestamptz IS NULL OR rl.ts <= $2)
	  AND ($3::text IS NULL OR rl.tenant_id = $3)
	  AND ($4::text IS NULL OR rl.api_key_name = $4)
	  AND ($5::text IS NULL OR rl.model = $5)
	  AND ($6::text IS NULL OR rl.provider = $6)
	  AND ($7::text IS NULL OR rl.status = $7)`

	countQuery := `SELECT COUNT(*) FROM request_log rl
		INNER JOIN usage u ON u.request_id = rl.request_id AND u.api_key_id = rl.api_key_id AND u.model = rl.model
		WHERE ` + baseWhere
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count api key raw usage: %w", err)
	}

	dataQuery := `
		SELECT rl.ts, rl.tenant_id, rl.api_key_id, COALESCE(rl.api_key_name, ''), rl.request_id, rl.model, rl.provider, rl.status,
			COALESCE(rl.latency_ms, 0),
			COALESCE(u.cost_usd, 0), COALESCE(u.prompt_tokens, 0), COALESCE(u.completion_tokens, 0), COALESCE(u.total_tokens, 0)
		FROM request_log rl
		INNER JOIN usage u ON u.request_id = rl.request_id AND u.api_key_id = rl.api_key_id AND u.model = rl.model
		WHERE ` + baseWhere + `
		ORDER BY rl.ts DESC
		LIMIT $8 OFFSET $9`
	dataArgs := append(args, filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query api key raw usage: %w", err)
	}
	defer rows.Close()

	var result []APIKeyRawUsageRow
	for rows.Next() {
		var r APIKeyRawUsageRow
		if err := rows.Scan(&r.Timestamp, &r.TenantID, &r.APIKeyID, &r.APIKeyName, &r.RequestID, &r.Model, &r.Provider, &r.Status,
			&r.LatencyMs, &r.CostUSD, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens); err != nil {
			return nil, 0, fmt.Errorf("scan api key raw usage row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// GetJWTSubUsage returns aggregated usage grouped by jwt_sub.
func (s *PostgresStorage) GetJWTSubUsage(ctx context.Context, filter JWTSubUsageFilter) ([]JWTSubUsageRow, int, error) {
	var fromT, toT interface{}
	if filter.From != nil {
		fromT = *filter.From
	}
	if filter.To != nil {
		toT = *filter.To
	}
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}
	modelArg := interface{}(nil)
	if filter.Model != "" {
		modelArg = filter.Model
	}
	providerArg := interface{}(nil)
	if filter.Provider != "" {
		providerArg = filter.Provider
	}
	args := []interface{}{fromT, toT, tenantArg, modelArg, providerArg}

	baseWhere := `jwt_sub IS NOT NULL
	  AND ($1::timestamptz IS NULL OR ts >= $1)
	  AND ($2::timestamptz IS NULL OR ts <= $2)
	  AND ($3::text IS NULL OR tenant_id = $3)
	  AND ($4::text IS NULL OR model = $4)
	  AND ($5::text IS NULL OR provider = $5)`

	countQuery := `SELECT COUNT(*) FROM (SELECT jwt_sub, tenant_id FROM usage WHERE ` + baseWhere + ` GROUP BY jwt_sub, tenant_id) sub`
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count jwt_sub usage: %w", err)
	}

	sortColumn := "total_cost_usd"
	if filter.SortBy == "requests" {
		sortColumn = "requests"
	} else if filter.SortBy == "total_tokens" {
		sortColumn = "total_tokens"
	}
	sortOrder := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		sortOrder = "ASC"
	}

	dataQuery := `
		SELECT jwt_sub,
			tenant_id,
			COUNT(*)::int AS requests,
			COALESCE(SUM(prompt_tokens), 0)::int AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0)::int AS completion_tokens,
			COALESCE(SUM(total_tokens), 0)::int AS total_tokens,
			COALESCE(SUM(cost_usd), 0) AS total_cost_usd,
			MIN(ts) AS first_seen,
			MAX(ts) AS last_seen
		FROM usage
		WHERE ` + baseWhere + `
		GROUP BY jwt_sub, tenant_id
		ORDER BY ` + sortColumn + ` ` + sortOrder + `, jwt_sub ASC, tenant_id ASC
		LIMIT $6 OFFSET $7`
	dataArgs := append(args, filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query jwt_sub usage: %w", err)
	}
	defer rows.Close()

	var result []JWTSubUsageRow
	for rows.Next() {
		var row JWTSubUsageRow
		if err := rows.Scan(&row.JWTSub, &row.TenantID, &row.Requests, &row.PromptTokens, &row.CompletionTokens, &row.TotalTokens, &row.TotalCostUSD, &row.FirstSeen, &row.LastSeen); err != nil {
			return nil, 0, fmt.Errorf("scan jwt_sub usage row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// GetJWTSubUsageDetail returns summary and breakdown for one jwt_sub.
func (s *PostgresStorage) GetJWTSubUsageDetail(ctx context.Context, jwtSub string, filter JWTSubUsageDetailFilter) (JWTSubUsageDetailSummary, []JWTSubUsageBreakdownRow, error) {
	var fromT, toT interface{}
	if filter.From != nil {
		fromT = *filter.From
	}
	if filter.To != nil {
		toT = *filter.To
	}
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}
	args := []interface{}{jwtSub, fromT, toT, tenantArg}

	baseWhere := `jwt_sub = $1
	  AND ($2::timestamptz IS NULL OR ts >= $2)
	  AND ($3::timestamptz IS NULL OR ts <= $3)
	  AND ($4::text IS NULL OR tenant_id = $4)`

	var summary JWTSubUsageDetailSummary
	summaryQuery := `
		SELECT
			COUNT(*)::int AS requests,
			COALESCE(SUM(prompt_tokens), 0)::int AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0)::int AS completion_tokens,
			COALESCE(SUM(total_tokens), 0)::int AS total_tokens,
			COALESCE(SUM(cost_usd), 0) AS total_cost_usd
		FROM usage
		WHERE ` + baseWhere
	if err := s.db.QueryRowContext(ctx, summaryQuery, args...).Scan(&summary.Requests, &summary.PromptTokens, &summary.CompletionTokens, &summary.TotalTokens, &summary.TotalCostUSD); err != nil {
		return JWTSubUsageDetailSummary{}, nil, fmt.Errorf("jwt_sub usage summary: %w", err)
	}

	groupExpr := "model"
	if filter.GroupBy == "provider" {
		groupExpr = "provider"
	} else if filter.GroupBy == "day" {
		groupExpr = "to_char(date_trunc('day', ts) AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"')"
	}

	dataQuery := `
		SELECT ` + groupExpr + ` AS group_label,
			COUNT(*)::int AS requests,
			COALESCE(SUM(total_tokens), 0)::int AS total_tokens,
			COALESCE(SUM(cost_usd), 0) AS total_cost_usd
		FROM usage
		WHERE ` + baseWhere + `
		GROUP BY group_label
		ORDER BY group_label ASC`
	rows, err := s.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return JWTSubUsageDetailSummary{}, nil, fmt.Errorf("jwt_sub usage breakdown: %w", err)
	}
	defer rows.Close()

	var breakdown []JWTSubUsageBreakdownRow
	for rows.Next() {
		var group sql.NullString
		var row JWTSubUsageBreakdownRow
		if err := rows.Scan(&group, &row.Requests, &row.TotalTokens, &row.TotalCostUSD); err != nil {
			return JWTSubUsageDetailSummary{}, nil, fmt.Errorf("scan jwt_sub usage breakdown row: %w", err)
		}
		if group.Valid {
			row.Group = group.String
		}
		breakdown = append(breakdown, row)
	}
	if err := rows.Err(); err != nil {
		return JWTSubUsageDetailSummary{}, nil, err
	}
	return summary, breakdown, nil
}

// ListJWTSubRawUsage returns raw per-request rows attributed to jwt_sub.
func (s *PostgresStorage) ListJWTSubRawUsage(ctx context.Context, filter JWTSubRawUsageFilter) ([]JWTSubRawUsageRow, int, error) {
	var fromT, toT interface{}
	if filter.From != nil {
		fromT = *filter.From
	}
	if filter.To != nil {
		toT = *filter.To
	}
	tenantArg := interface{}(nil)
	if filter.TenantID != "" {
		tenantArg = filter.TenantID
	}
	jwtSubArg := interface{}(nil)
	if filter.JWTSub != "" {
		jwtSubArg = filter.JWTSub
	}
	modelArg := interface{}(nil)
	if filter.Model != "" {
		modelArg = filter.Model
	}
	providerArg := interface{}(nil)
	if filter.Provider != "" {
		providerArg = filter.Provider
	}
	statusArg := interface{}(nil)
	if filter.Status != "" {
		statusArg = filter.Status
	}
	args := []interface{}{fromT, toT, tenantArg, jwtSubArg, modelArg, providerArg, statusArg}

	baseWhere := `r.jwt_sub IS NOT NULL
	  AND ($1::timestamptz IS NULL OR r.ts >= $1)
	  AND ($2::timestamptz IS NULL OR r.ts <= $2)
	  AND ($3::text IS NULL OR r.tenant_id = $3)
	  AND ($4::text IS NULL OR r.jwt_sub = $4)
	  AND ($5::text IS NULL OR r.model = $5)
	  AND ($6::text IS NULL OR r.provider = $6)
	  AND ($7::text IS NULL OR r.status = $7)`

	countQuery := `SELECT COUNT(*) FROM request_log r
		INNER JOIN usage u ON u.request_id = r.request_id AND u.model = r.model
		WHERE ` + baseWhere
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count jwt_sub raw usage: %w", err)
	}

	dataQuery := `
		SELECT r.ts, r.tenant_id, r.jwt_sub, r.request_id, r.model, r.provider, r.status,
			COALESCE(r.latency_ms, 0),
			COALESCE(u.cost_usd, 0), COALESCE(u.prompt_tokens, 0), COALESCE(u.completion_tokens, 0), COALESCE(u.total_tokens, 0)
		FROM request_log r
		INNER JOIN usage u ON u.request_id = r.request_id AND u.model = r.model
		WHERE ` + baseWhere + `
		ORDER BY r.ts DESC
		LIMIT $8 OFFSET $9`
	dataArgs := append(args, filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query jwt_sub raw usage: %w", err)
	}
	defer rows.Close()

	var result []JWTSubRawUsageRow
	for rows.Next() {
		var row JWTSubRawUsageRow
		if err := rows.Scan(&row.Timestamp, &row.TenantID, &row.JWTSub, &row.RequestID, &row.Model, &row.Provider, &row.Status, &row.LatencyMs,
			&row.CostUSD, &row.PromptTokens, &row.CompletionTokens, &row.TotalTokens); err != nil {
			return nil, 0, fmt.Errorf("scan jwt_sub raw usage row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func nilToNullString(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func nilToNullBool(b *bool) interface{} {
	if b == nil {
		return nil
	}
	return *b
}

func nullFloat64ToFloat(v sql.NullFloat64) float64 {
	if v.Valid {
		return v.Float64
	}
	return 0
}

// GetRequestStats returns aggregated request telemetry for the given window and optional tenant.
// bucket must be "minute" or "hour"; windowHours is used for the time window.
func (s *PostgresStorage) GetRequestStats(ctx context.Context, tenantID string, windowHours int, bucket string) (RequestStats, error) {
	trunc := "hour"
	if bucket == "minute" {
		trunc = "minute"
	}

	tenantArg := interface{}(nil)
	if tenantID != "" {
		tenantArg = tenantID
	}
	// Use integer * INTERVAL '1 hour' so small windows (e.g. 24) work; ($1::text || ' hours')::interval was fragile.
	args := []interface{}{windowHours, tenantArg}

	// Summary
	var totalRequests, fallbackRequests int
	var avgLatencyMs, successRate sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total_requests,
			AVG(latency_ms) AS avg_latency_ms,
			AVG(CASE WHEN status = 'ok' THEN 1.0 ELSE 0.0 END) AS success_rate,
			COALESCE(SUM(CASE WHEN fallback_used THEN 1 ELSE 0 END), 0)::int AS fallback_requests
		FROM request_log
		WHERE ts >= NOW() - ($1 * INTERVAL '1 hour')
		  AND ($2::text IS NULL OR tenant_id = $2)
	`, args...).Scan(&totalRequests, &avgLatencyMs, &successRate, &fallbackRequests)
	if err != nil {
		return RequestStats{}, fmt.Errorf("request_stats summary: %w", err)
	}

	summary := RequestStatsSummary{
		TotalRequests:    totalRequests,
		FallbackRate:     0,
		FallbackRequests: fallbackRequests,
		CacheHitRate:     nil,
	}
	if totalRequests > 0 {
		summary.FallbackRate = float64(fallbackRequests) / float64(totalRequests)
	}
	if successRate.Valid {
		summary.SuccessRate = successRate.Float64
	}
	if avgLatencyMs.Valid {
		summary.AvgLatencyMs = avgLatencyMs.Float64
	}

	// Traffic over time
	trafficQuery := fmt.Sprintf(`
		SELECT date_trunc('%s', ts) AS bucket,
			COUNT(*) AS requests,
			SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END)::int AS successes,
			SUM(CASE WHEN status <> 'ok' THEN 1 ELSE 0 END)::int AS errors
		FROM request_log
		WHERE ts >= NOW() - ($1 * INTERVAL '1 hour')
		  AND ($2::text IS NULL OR tenant_id = $2)
		GROUP BY 1
		ORDER BY 1
	`, trunc)
	trafficRows, err := s.db.QueryContext(ctx, trafficQuery, args...)
	if err != nil {
		return RequestStats{}, fmt.Errorf("request_stats traffic: %w", err)
	}
	defer trafficRows.Close()

	var traffic []TrafficBucket
	for trafficRows.Next() {
		var b TrafficBucket
		if err := trafficRows.Scan(&b.Bucket, &b.Requests, &b.Successes, &b.Errors); err != nil {
			return RequestStats{}, fmt.Errorf("scan traffic row: %w", err)
		}
		traffic = append(traffic, b)
	}
	if err := trafficRows.Err(); err != nil {
		return RequestStats{}, err
	}

	// Provider health
	providerRows, err := s.db.QueryContext(ctx, `
		SELECT provider,
			COUNT(*) AS total_requests,
			AVG(latency_ms) AS avg_latency_ms,
			AVG(CASE WHEN status = 'ok' THEN 1.0 ELSE 0.0 END) AS success_rate
		FROM request_log
		WHERE ts >= NOW() - ($1 * INTERVAL '1 hour')
		  AND ($2::text IS NULL OR tenant_id = $2)
		GROUP BY provider
		ORDER BY provider
	`, args...)
	if err != nil {
		return RequestStats{}, fmt.Errorf("request_stats provider_health: %w", err)
	}
	defer providerRows.Close()

	var providers []ProviderHealthRow
	for providerRows.Next() {
		var p ProviderHealthRow
		if err := providerRows.Scan(&p.Provider, &p.TotalRequests, &p.AvgLatencyMs, &p.SuccessRate); err != nil {
			return RequestStats{}, fmt.Errorf("scan provider row: %w", err)
		}
		providers = append(providers, p)
	}
	if err := providerRows.Err(); err != nil {
		return RequestStats{}, err
	}

	// Status breakdown (return "ok" and "error" as stored; handler can map to success/error for API)
	statusRows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM request_log
		WHERE ts >= NOW() - ($1 * INTERVAL '1 hour')
		  AND ($2::text IS NULL OR tenant_id = $2)
		GROUP BY status
	`, args...)
	if err != nil {
		return RequestStats{}, fmt.Errorf("request_stats status_breakdown: %w", err)
	}
	defer statusRows.Close()

	statusBreakdown := make(map[string]int)
	for statusRows.Next() {
		var status string
		var count int
		if err := statusRows.Scan(&status, &count); err != nil {
			return RequestStats{}, fmt.Errorf("scan status row: %w", err)
		}
		statusBreakdown[status] = count
	}
	if err := statusRows.Err(); err != nil {
		return RequestStats{}, err
	}

	return RequestStats{
		WindowHours:     windowHours,
		Summary:         summary,
		TrafficOverTime: traffic,
		ProviderHealth:  providers,
		StatusBreakdown: statusBreakdown,
	}, nil
}

// GetRouterPerformance returns router-only performance metrics for the given filter.
func (s *PostgresStorage) GetRouterPerformance(ctx context.Context, filter RouterPerformanceFilter) (RouterPerformanceMetrics, error) {
	fromArg := interface{}(nil)
	if filter.From != nil {
		fromArg = *filter.From
	}
	toArg := interface{}(nil)
	if filter.To != nil {
		toArg = *filter.To
	}
	tenantArg := interface{}(nil)
	if filter.TenantID != nil {
		tenantArg = *filter.TenantID
	}
	modelArg := interface{}(nil)
	if filter.Model != nil {
		modelArg = *filter.Model
	}
	providerArg := interface{}(nil)
	if filter.Provider != nil {
		providerArg = *filter.Provider
	}
	statusArg := interface{}(nil)
	if filter.Status != nil {
		statusArg = *filter.Status
	}

	args := []interface{}{fromArg, toArg, tenantArg, modelArg, providerArg, statusArg}
	baseWhere := `
		($1::timestamptz IS NULL OR ts >= $1)
		AND ($2::timestamptz IS NULL OR ts <= $2)
		AND ($3::text IS NULL OR tenant_id = $3)
		AND ($4::text IS NULL OR model = $4)
		AND ($5::text IS NULL OR provider = $5)
		AND ($6::text IS NULL OR status = $6)
		AND router_pre_ms IS NOT NULL
		AND llm_latency_ms IS NOT NULL
		AND router_post_ms IS NOT NULL
	`

	var summary RouterPerformanceSummary
	var avgRouterPre, minRouterPre, maxRouterPre, p50RouterPre, p95RouterPre sql.NullFloat64
	var avgLLM, minLLM, maxLLM, p50LLM, p95LLM sql.NullFloat64
	var avgRouterPost, minRouterPost, maxRouterPost, p50RouterPost, p95RouterPost sql.NullFloat64
	var avgTotal, p50Total, p95Total sql.NullFloat64
	var successRate, errorRate sql.NullFloat64
	var avgPreTenant, avgCfgToolRoutes, avgCfgDynamic, avgCfgDecisionOps, avgCfgBudget, avgCfgSemantic, avgCfgModelResolution sql.NullFloat64

	row := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)::int AS requests,
			AVG(router_pre_ms) AS avg_router_pre_ms,
			MIN(router_pre_ms) AS min_router_pre_ms,
			MAX(router_pre_ms) AS max_router_pre_ms,
			percentile_cont(0.5) WITHIN GROUP (ORDER BY router_pre_ms) AS p50_router_pre_ms,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY router_pre_ms) AS p95_router_pre_ms,
			AVG(llm_latency_ms) AS avg_llm_latency_ms,
			MIN(llm_latency_ms) AS min_llm_latency_ms,
			MAX(llm_latency_ms) AS max_llm_latency_ms,
			percentile_cont(0.5) WITHIN GROUP (ORDER BY llm_latency_ms) AS p50_llm_latency_ms,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY llm_latency_ms) AS p95_llm_latency_ms,
			AVG(router_post_ms) AS avg_router_post_ms,
			MIN(router_post_ms) AS min_router_post_ms,
			MAX(router_post_ms) AS max_router_post_ms,
			percentile_cont(0.5) WITHIN GROUP (ORDER BY router_post_ms) AS p50_router_post_ms,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY router_post_ms) AS p95_router_post_ms,
			AVG(latency_ms) AS avg_total_latency_ms,
			percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) AS p50_total_latency_ms,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) AS p95_total_latency_ms,
			AVG(CASE WHEN status = 'ok' THEN 1.0 ELSE 0.0 END) AS success_rate,
			AVG(CASE WHEN status <> 'ok' THEN 1.0 ELSE 0.0 END) AS error_rate,
			AVG(pre_tenant_config_ms) AS avg_pre_tenant_config_ms,
			AVG(cfg_tool_routes_ms) AS avg_cfg_tool_routes_ms,
			AVG(cfg_dynamic_routes_ms) AS avg_cfg_dynamic_routes_ms,
			AVG(cfg_decision_ops_ms) AS avg_cfg_decision_ops_ms,
			AVG(cfg_budget_pressure_ms) AS avg_cfg_budget_pressure_ms,
			AVG(cfg_semantic_ms) AS avg_cfg_semantic_ms,
			AVG(cfg_model_resolution_ms) AS avg_cfg_model_resolution_ms
		FROM request_log
		WHERE `+baseWhere, args...)

	if err := row.Scan(
		&summary.Requests,
		&avgRouterPre, &minRouterPre, &maxRouterPre, &p50RouterPre, &p95RouterPre,
		&avgLLM, &minLLM, &maxLLM, &p50LLM, &p95LLM,
		&avgRouterPost, &minRouterPost, &maxRouterPost, &p50RouterPost, &p95RouterPost,
		&avgTotal, &p50Total, &p95Total,
		&successRate, &errorRate,
		&avgPreTenant, &avgCfgToolRoutes, &avgCfgDynamic, &avgCfgDecisionOps, &avgCfgBudget, &avgCfgSemantic, &avgCfgModelResolution,
	); err != nil {
		return RouterPerformanceMetrics{}, fmt.Errorf("router performance summary: %w", err)
	}

	summary.AvgRouterPreMs = nullFloat64ToFloat(avgRouterPre)
	summary.MinRouterPreMs = nullFloat64ToFloat(minRouterPre)
	summary.MaxRouterPreMs = nullFloat64ToFloat(maxRouterPre)
	summary.P50RouterPreMs = nullFloat64ToFloat(p50RouterPre)
	summary.P95RouterPreMs = nullFloat64ToFloat(p95RouterPre)

	summary.AvgLLMLatencyMs = nullFloat64ToFloat(avgLLM)
	summary.MinLLMLatencyMs = nullFloat64ToFloat(minLLM)
	summary.MaxLLMLatencyMs = nullFloat64ToFloat(maxLLM)
	summary.P50LLMLatencyMs = nullFloat64ToFloat(p50LLM)
	summary.P95LLMLatencyMs = nullFloat64ToFloat(p95LLM)

	summary.AvgRouterPostMs = nullFloat64ToFloat(avgRouterPost)
	summary.MinRouterPostMs = nullFloat64ToFloat(minRouterPost)
	summary.MaxRouterPostMs = nullFloat64ToFloat(maxRouterPost)
	summary.P50RouterPostMs = nullFloat64ToFloat(p50RouterPost)
	summary.P95RouterPostMs = nullFloat64ToFloat(p95RouterPost)

	summary.AvgTotalLatencyMs = nullFloat64ToFloat(avgTotal)
	summary.P50TotalLatencyMs = nullFloat64ToFloat(p50Total)
	summary.P95TotalLatencyMs = nullFloat64ToFloat(p95Total)

	summary.SuccessRate = nullFloat64ToFloat(successRate)
	summary.ErrorRate = nullFloat64ToFloat(errorRate)

	summary.AvgPreTenantConfigMs = nullFloat64ToFloat(avgPreTenant)
	summary.AvgCfgToolRoutesMs = nullFloat64ToFloat(avgCfgToolRoutes)
	summary.AvgCfgDynamicRoutesMs = nullFloat64ToFloat(avgCfgDynamic)
	summary.AvgCfgDecisionOpsMs = nullFloat64ToFloat(avgCfgDecisionOps)
	summary.AvgCfgBudgetPressureMs = nullFloat64ToFloat(avgCfgBudget)
	summary.AvgCfgSemanticMs = nullFloat64ToFloat(avgCfgSemantic)
	summary.AvgCfgModelResolutionMs = nullFloat64ToFloat(avgCfgModelResolution)

	trunc := "hour"
	if filter.Bucket == "minute" || filter.Bucket == "hour" || filter.Bucket == "day" {
		trunc = filter.Bucket
	}
	timeseriesQuery := fmt.Sprintf(`
		SELECT date_trunc('%s', ts) AS bucket_start,
			COUNT(*)::int AS requests,
			AVG(router_pre_ms) AS avg_router_pre_ms,
			AVG(llm_latency_ms) AS avg_llm_latency_ms,
			AVG(router_post_ms) AS avg_router_post_ms,
			AVG(latency_ms) AS avg_total_latency_ms,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY router_pre_ms) AS p95_router_pre_ms,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY llm_latency_ms) AS p95_llm_latency_ms,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY router_post_ms) AS p95_router_post_ms
		FROM request_log
		WHERE `+baseWhere+`
		GROUP BY 1
		ORDER BY 1
	`, trunc)

	rows, err := s.db.QueryContext(ctx, timeseriesQuery, args...)
	if err != nil {
		return RouterPerformanceMetrics{}, fmt.Errorf("router performance timeseries: %w", err)
	}
	defer rows.Close()

	var timeseries []RouterPerformanceTimeseriesRow
	for rows.Next() {
		var row RouterPerformanceTimeseriesRow
		var avgPre, avgLlm, avgPost, avgTotalTs, p95Pre, p95Llm, p95Post sql.NullFloat64
		if err := rows.Scan(
			&row.BucketStart,
			&row.Requests,
			&avgPre, &avgLlm, &avgPost, &avgTotalTs,
			&p95Pre, &p95Llm, &p95Post,
		); err != nil {
			return RouterPerformanceMetrics{}, fmt.Errorf("scan router performance timeseries row: %w", err)
		}
		row.AvgRouterPreMs = nullFloat64ToFloat(avgPre)
		row.AvgLLMLatencyMs = nullFloat64ToFloat(avgLlm)
		row.AvgRouterPostMs = nullFloat64ToFloat(avgPost)
		row.AvgTotalLatencyMs = nullFloat64ToFloat(avgTotalTs)
		row.P95RouterPreMs = nullFloat64ToFloat(p95Pre)
		row.P95LLMLatencyMs = nullFloat64ToFloat(p95Llm)
		row.P95RouterPostMs = nullFloat64ToFloat(p95Post)
		timeseries = append(timeseries, row)
	}
	if err := rows.Err(); err != nil {
		return RouterPerformanceMetrics{}, err
	}

	var breakdowns RouterPerformanceBreakdowns
	var avgToolEmbeddingModel, avgToolEmbeddingGenerate, avgToolSemanticDB, avgToolMatchEval sql.NullFloat64
	var avgBdDecisionOps sql.NullFloat64
	row = s.db.QueryRowContext(ctx, `
		SELECT
			AVG(pre_tenant_config_ms) AS avg_pre_tenant_config_ms,
			AVG(cfg_tool_routes_ms) AS avg_cfg_tool_routes_ms,
			AVG(cfg_dynamic_routes_ms) AS avg_cfg_dynamic_routes_ms,
			AVG(cfg_decision_ops_ms) AS avg_cfg_decision_ops_ms,
			AVG(cfg_budget_pressure_ms) AS avg_cfg_budget_pressure_ms,
			AVG(cfg_semantic_ms) AS avg_cfg_semantic_ms,
			AVG(cfg_model_resolution_ms) AS avg_cfg_model_resolution_ms,
			AVG(tool_routes_embedding_model_ms) AS avg_tool_routes_embedding_model_ms,
			AVG(tool_routes_embedding_generate_ms) AS avg_tool_routes_embedding_generate_ms,
			AVG(tool_routes_semantic_db_ms) AS avg_tool_routes_semantic_db_ms,
			AVG(tool_routes_match_eval_ms) AS avg_tool_routes_match_eval_ms
		FROM request_log
		WHERE `+baseWhere, args...)

	if err := row.Scan(
		&avgPreTenant, &avgCfgToolRoutes, &avgCfgDynamic, &avgBdDecisionOps, &avgCfgBudget, &avgCfgSemantic, &avgCfgModelResolution,
		&avgToolEmbeddingModel, &avgToolEmbeddingGenerate, &avgToolSemanticDB, &avgToolMatchEval,
	); err != nil {
		return RouterPerformanceMetrics{}, fmt.Errorf("router performance breakdowns: %w", err)
	}

	breakdowns.PreBreakdownAvgMs = RouterPreBreakdownAvgMs{
		TenantConfig:    nullFloat64ToFloat(avgPreTenant),
		ToolRoutes:      nullFloat64ToFloat(avgCfgToolRoutes),
		DynamicRoutes:   nullFloat64ToFloat(avgCfgDynamic),
		DecisionOps:     nullFloat64ToFloat(avgBdDecisionOps),
		BudgetPressure:  nullFloat64ToFloat(avgCfgBudget),
		Semantic:        nullFloat64ToFloat(avgCfgSemantic),
		ModelResolution: nullFloat64ToFloat(avgCfgModelResolution),
	}
	breakdowns.ToolRoutesBreakdownAvgMs = RouterToolRoutesBreakdownAvgMs{
		EmbeddingModel:    nullFloat64ToFloat(avgToolEmbeddingModel),
		EmbeddingGenerate: nullFloat64ToFloat(avgToolEmbeddingGenerate),
		SemanticDB:        nullFloat64ToFloat(avgToolSemanticDB),
		MatchEval:         nullFloat64ToFloat(avgToolMatchEval),
	}

	return RouterPerformanceMetrics{
		Summary:    summary,
		Timeseries: timeseries,
		Breakdowns: breakdowns,
	}, nil
}

// GetSemanticRoutingStats returns semantic routing analytics from request_log (routing_snapshot with semantic_anchor).
// tenantID optional; windowDays lookback; top lists limited to 10.
func (s *PostgresStorage) GetSemanticRoutingStats(ctx context.Context, tenantID string, windowDays int) (SemanticRoutingStats, error) {
	const topLimit = 10
	tenantArg := interface{}(nil)
	if tenantID != "" {
		tenantArg = tenantID
	}
	windowStart := time.Now().UTC().AddDate(0, 0, -windowDays)

	// Base filter: attempt=1 (one per logical request), ts in window, optional tenant
	baseWhere := `attempt = 1 AND ts >= $2 AND ($1::text IS NULL OR tenant_id = $1)`
	matchedWhere := baseWhere + ` AND routing_snapshot IS NOT NULL AND (routing_snapshot->>'semantic_anchor') IS NOT NULL AND TRIM(COALESCE(routing_snapshot->>'semantic_anchor', '')) != ''`
	argsWithWindow := []interface{}{tenantArg, windowStart}

	// Coverage: total requests and matched (semantic) requests
	var totalRequests, matchedRequests int
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE routing_snapshot IS NOT NULL AND (routing_snapshot->>'semantic_anchor') IS NOT NULL AND TRIM(COALESCE(routing_snapshot->>'semantic_anchor', '')) != '')::int
		FROM request_log
		WHERE `+baseWhere+`
	`, argsWithWindow...).Scan(&totalRequests, &matchedRequests)
	if err != nil {
		return SemanticRoutingStats{}, fmt.Errorf("semantic routing coverage: %w", err)
	}
	coverageRate := 0.0
	if totalRequests > 0 {
		coverageRate = float64(matchedRequests) / float64(totalRequests)
	}
	coverage := SemanticRoutingCoverage{
		TotalRequests:   totalRequests,
		MatchedRequests: matchedRequests,
		CoverageRate:    coverageRate,
	}

	// Top routes: by route_group among matched requests, order by matches DESC
	routeArgs := append(argsWithWindow, topLimit)
	routeRows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(TRIM(routing_snapshot->>'route_group'), ''), 'unknown') AS route_group, COUNT(*)::int AS matches
		FROM request_log
		WHERE `+matchedWhere+`
		GROUP BY 1
		ORDER BY matches DESC
		LIMIT $3
	`, routeArgs...)
	if err != nil {
		return SemanticRoutingStats{}, fmt.Errorf("semantic routing top routes: %w", err)
	}
	defer routeRows.Close()
	var topRoutes []SemanticRoutingTopRoute
	for routeRows.Next() {
		var r SemanticRoutingTopRoute
		if err := routeRows.Scan(&r.RouteGroup, &r.Matches); err != nil {
			return SemanticRoutingStats{}, fmt.Errorf("scan top route: %w", err)
		}
		topRoutes = append(topRoutes, r)
	}
	if err := routeRows.Err(); err != nil {
		return SemanticRoutingStats{}, err
	}

	// Top anchors: by semantic_anchor among matched requests, order by matches DESC
	anchorRows, err := s.db.QueryContext(ctx, `
		SELECT TRIM(routing_snapshot->>'semantic_anchor') AS anchor, COUNT(*)::int AS matches
		FROM request_log
		WHERE `+matchedWhere+`
		GROUP BY TRIM(routing_snapshot->>'semantic_anchor')
		ORDER BY matches DESC
		LIMIT $3
	`, routeArgs...)
	if err != nil {
		return SemanticRoutingStats{}, fmt.Errorf("semantic routing top anchors: %w", err)
	}
	defer anchorRows.Close()
	var topAnchors []SemanticRoutingTopAnchor
	for anchorRows.Next() {
		var a SemanticRoutingTopAnchor
		if err := anchorRows.Scan(&a.Anchor, &a.Matches); err != nil {
			return SemanticRoutingStats{}, fmt.Errorf("scan top anchor: %w", err)
		}
		topAnchors = append(topAnchors, a)
	}
	if err := anchorRows.Err(); err != nil {
		return SemanticRoutingStats{}, err
	}

	if topRoutes == nil {
		topRoutes = []SemanticRoutingTopRoute{}
	}
	if topAnchors == nil {
		topAnchors = []SemanticRoutingTopAnchor{}
	}

	return SemanticRoutingStats{
		TopRoutes:  topRoutes,
		TopAnchors: topAnchors,
		Coverage:   coverage,
	}, nil
}

// GetSemanticCorrelation correlates semantic cache hits and request counts by route_group.
// semantic_cache: SUM(hit_count) per route_group; request_log: COUNT(*) per route_group (routing_snapshot->>'route_group') in window.
func (s *PostgresStorage) GetSemanticCorrelation(ctx context.Context, tenantID string, windowDays int) (SemanticCorrelation, error) {
	tenantArg := interface{}(nil)
	if tenantID != "" {
		tenantArg = tenantID
	}
	windowStart := time.Now().UTC().AddDate(0, 0, -windowDays)

	// Cache: route_group -> total hit_count
	cacheRows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(TRIM(route_group), ''), 'unknown') AS route_group, COALESCE(SUM(hit_count), 0)::bigint AS cache_hits
		FROM semantic_cache
		WHERE $1::text IS NULL OR tenant_id = $1
		GROUP BY COALESCE(NULLIF(TRIM(route_group), ''), 'unknown')
	`, tenantArg)
	if err != nil {
		return SemanticCorrelation{}, fmt.Errorf("semantic correlation cache: %w", err)
	}
	defer cacheRows.Close()
	cacheByRG := make(map[string]int64)
	for cacheRows.Next() {
		var rg string
		var hits int64
		if err := cacheRows.Scan(&rg, &hits); err != nil {
			return SemanticCorrelation{}, fmt.Errorf("scan cache by route_group: %w", err)
		}
		cacheByRG[rg] = hits
	}
	if err := cacheRows.Err(); err != nil {
		return SemanticCorrelation{}, err
	}

	// Request log: route_group -> total_requests (attempt=1, has routing_snapshot)
	reqRows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(TRIM(routing_snapshot->>'route_group'), ''), 'unknown') AS route_group, COUNT(*)::int AS total_requests
		FROM request_log
		WHERE attempt = 1 AND ts >= $2 AND routing_snapshot IS NOT NULL
		  AND ($1::text IS NULL OR tenant_id = $1)
		GROUP BY 1
	`, tenantArg, windowStart)
	if err != nil {
		return SemanticCorrelation{}, fmt.Errorf("semantic correlation request_log: %w", err)
	}
	defer reqRows.Close()
	requestsByRG := make(map[string]int)
	for reqRows.Next() {
		var rg string
		var n int
		if err := reqRows.Scan(&rg, &n); err != nil {
			return SemanticCorrelation{}, fmt.Errorf("scan requests by route_group: %w", err)
		}
		requestsByRG[rg] = n
	}
	if err := reqRows.Err(); err != nil {
		return SemanticCorrelation{}, err
	}

	// All route_groups from either source
	seen := make(map[string]bool)
	for k := range cacheByRG {
		seen[k] = true
	}
	for k := range requestsByRG {
		seen[k] = true
	}
	var byRouteGroup []SemanticCorrelationByRouteGroup
	for rg := range seen {
		cacheHits := cacheByRG[rg]
		totalReq := requestsByRG[rg]
		hitRate := 0.0
		if totalReq > 0 {
			hitRate = float64(cacheHits) / float64(totalReq)
		}
		byRouteGroup = append(byRouteGroup, SemanticCorrelationByRouteGroup{
			RouteGroup:    rg,
			CacheHits:     cacheHits,
			TotalRequests: totalReq,
			HitRate:       hitRate,
		})
	}
	sort.Slice(byRouteGroup, func(i, j int) bool { return byRouteGroup[i].TotalRequests > byRouteGroup[j].TotalRequests })
	if byRouteGroup == nil {
		byRouteGroup = []SemanticCorrelationByRouteGroup{}
	}
	return SemanticCorrelation{ByRouteGroup: byRouteGroup}, nil
}

// ListTenants returns all tenant IDs present in tenant_active_config.
func (s *PostgresStorage) ListTenants(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tenant_id FROM tenant_active_config ORDER BY tenant_id`)
	if err != nil {
		return nil, fmt.Errorf("query tenant_active_config: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tenant_id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// PingDB sends a trivial query to verify the database is reachable.
func (s *PostgresStorage) PingDB(ctx context.Context) error {
	return s.db.QueryRowContext(ctx, "SELECT 1").Scan(new(int))
}

// ListTables returns all base table names in the public schema.
func (s *PostgresStorage) ListTables(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		 ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("query information_schema: %w", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table_name: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// ExpectedTables parses the embedded migration files and returns the unique set
// of table names found in CREATE TABLE statements. This list automatically grows
// when new migrations are added — no manual maintenance required.
func ExpectedTables() []string {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			// Match: CREATE TABLE [IF NOT EXISTS] <name>
			if !strings.HasPrefix(line, "create table") {
				continue
			}
			line = strings.TrimPrefix(line, "create table")
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "if not exists")
			line = strings.TrimSpace(line)
			// Table name ends at first whitespace or '('
			end := strings.IndexAny(line, " \t(")
			if end < 0 {
				end = len(line)
			}
			name := strings.Trim(line[:end], `"'`)
			if name != "" {
				seen[name] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
