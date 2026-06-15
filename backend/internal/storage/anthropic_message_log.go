package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// AnthropicMessageLog is the audit record for a single POST /v1/messages call.
// Stored exclusively in the anthropic_message_log table (SPEC_154).
// Never mixed with request_log, conversation_log, or compliance_event_log.
type AnthropicMessageLog struct {
	// Identity / audit
	TenantID  string    `db:"tenant_id"`
	RequestID string    `db:"request_id"`
	CreatedAt time.Time `db:"created_at"`

	// Auth attribution
	APIKeyID    *string `db:"api_key_id"`    // UUID string of the DB-backed API key, nil for YAML / JWT
	APIKeyValue *string `db:"api_key_value"` // masked value: prefix + ****last4, nil when not an API-key call
	APIKeyName  *string `db:"api_key_name"`  // human-readable display name, nil when unavailable (SPEC_156)
	JwtSub      *string `db:"jwt_sub"`       // JWT subject, nil for API-key-only flows

	// Routing metadata
	Provider   string `db:"provider"`    // always "anthropic" for this endpoint
	Endpoint   string `db:"endpoint"`    // "/v1/messages"
	HTTPMethod string `db:"http_method"` // "POST"

	// Model
	ModelRequested *string `db:"model_requested"` // from request body
	ModelUsed      *string `db:"model_used"`       // from response body

	// Upstream IDs
	AnthropicMessageID *string `db:"anthropic_message_id"` // response.id
	UpstreamRequestID  *string `db:"upstream_request_id"`  // reserved for future use

	// Result
	StatusCode *int `db:"status_code"`
	Success    bool `db:"success"`

	// Tokens
	InputTokens  *int `db:"input_tokens"`
	OutputTokens *int `db:"output_tokens"`
	TotalTokens  *int `db:"total_tokens"`

	// Cost (SPEC_156)
	Cost *float64 `db:"cost"` // NUMERIC(18,8); nil when model pricing is unavailable

	// Stop
	StopReason   *string `db:"stop_reason"`
	StopSequence *string `db:"stop_sequence"`

	// Content
	PromptText   *string `db:"prompt_text"`
	ResponseText *string `db:"response_text"`

	// Raw payloads
	RawRequestJSON  json.RawMessage `db:"raw_request_json"`  // JSONB
	RawResponseJSON json.RawMessage `db:"raw_response_json"` // JSONB

	// Error
	ErrorType    *string `db:"error_type"`
	ErrorMessage *string `db:"error_message"`

	// Latency
	LatencyMs *int `db:"latency_ms"`
}

// InsertAnthropicMessageLog inserts one row into anthropic_message_log.
func (s *PostgresStorage) InsertAnthropicMessageLog(ctx context.Context, row AnthropicMessageLog) error {
	const q = `
INSERT INTO anthropic_message_log (
    created_at,
    tenant_id, request_id,
    api_key_id, api_key_value, api_key_name, jwt_sub,
    provider, endpoint, http_method,
    model_requested, model_used,
    anthropic_message_id, upstream_request_id,
    status_code, success,
    input_tokens, output_tokens, total_tokens,
    cost,
    stop_reason, stop_sequence,
    prompt_text, response_text,
    raw_request_json, raw_response_json,
    error_type, error_message,
    latency_ms
) VALUES (
    $1,
    $2, $3,
    $4, $5, $6, $7,
    $8, $9, $10,
    $11, $12,
    $13, $14,
    $15, $16,
    $17, $18, $19,
    $20,
    $21, $22,
    $23, $24,
    $25, $26,
    $27, $28,
    $29
)`

	createdAt := row.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, q,
		createdAt,
		row.TenantID, row.RequestID,
		row.APIKeyID, row.APIKeyValue, row.APIKeyName, row.JwtSub,
		row.Provider, row.Endpoint, row.HTTPMethod,
		row.ModelRequested, row.ModelUsed,
		row.AnthropicMessageID, row.UpstreamRequestID,
		row.StatusCode, row.Success,
		row.InputTokens, row.OutputTokens, row.TotalTokens,
		row.Cost,
		row.StopReason, row.StopSequence,
		row.PromptText, row.ResponseText,
		jsonbText(row.RawRequestJSON), jsonbText(row.RawResponseJSON),
		row.ErrorType, row.ErrorMessage,
		row.LatencyMs,
	)
	return err
}

// GetClaudeCodeMonthlySpend returns the sum of cost from anthropic_message_log
// for a tenant in the half-open interval [from, to) where cost IS NOT NULL.
// Used for SPEC_163 Claude Code budget enforcement.
func (s *PostgresStorage) GetClaudeCodeMonthlySpend(ctx context.Context, tenantID string, from, to time.Time) (float64, error) {
	var spend float64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost), 0.0)
		 FROM anthropic_message_log
		 WHERE tenant_id = $1
		   AND created_at >= $2
		   AND created_at < $3
		   AND cost IS NOT NULL`,
		tenantID, from, to,
	).Scan(&spend)
	if err != nil {
		return 0, fmt.Errorf("get claude code monthly spend: %w", err)
	}
	return spend, nil
}

// ── SPEC_164 FinOps query types ───────────────────────────────────────────────

// ClaudeCodeUsageFilter holds all filter and pagination parameters for the
// GET /admin/claude_code/usage endpoint.
type ClaudeCodeUsageFilter struct {
	TenantID   string
	From       time.Time
	To         time.Time
	APIKeyName string // optional
	JWTSub     string // optional
	Limit      int    // default 50
	Offset     int
}

// ClaudeCodeUsageSummary holds aggregated totals for the summary section.
type ClaudeCodeUsageSummary struct {
	Requests          int64   `json:"requests"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalCost         float64 `json:"total_cost"`
	AvgCostPerRequest float64 `json:"avg_cost_per_request"`
}

// ClaudeCodeTimeseriesBucket holds per-day aggregated metrics.
type ClaudeCodeTimeseriesBucket struct {
	Bucket       time.Time `json:"bucket"`
	Requests     int64     `json:"requests"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalCost    float64   `json:"total_cost"`
}

// ClaudeCodeUsageRow is one raw row returned in the `rows` section.
type ClaudeCodeUsageRow struct {
	CreatedAt      time.Time `json:"created_at"`
	RequestID      string    `json:"request_id"`
	TenantID       string    `json:"tenant_id"`
	APIKeyName     *string   `json:"api_key_name"`
	JWTSub         *string   `json:"jwt_sub"`
	ModelRequested *string   `json:"model_requested"`
	ModelUsed      *string   `json:"model_used"`
	InputTokens    *int      `json:"input_tokens"`
	OutputTokens   *int      `json:"output_tokens"`
	TotalTokens    *int      `json:"total_tokens"`
	Cost           *float64  `json:"cost"`
	StatusCode     *int      `json:"status_code"`
	Success        bool      `json:"success"`
	StopReason     *string   `json:"stop_reason,omitempty"`
	LatencyMs      *int      `json:"latency_ms,omitempty"`
}

// GetClaudeCodeUsageSummary returns aggregate totals for the given filter window.
func (s *PostgresStorage) GetClaudeCodeUsageSummary(ctx context.Context, f ClaudeCodeUsageFilter) (ClaudeCodeUsageSummary, error) {
	q := `SELECT
		COUNT(*),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(cost), 0)
	FROM anthropic_message_log
	WHERE tenant_id = $1
	  AND created_at >= $2
	  AND created_at < $3`
	args := []interface{}{f.TenantID, f.From, f.To}
	idx := 4
	if f.APIKeyName != "" {
		q += fmt.Sprintf(" AND api_key_name = $%d", idx)
		args = append(args, f.APIKeyName)
		idx++
	}
	if f.JWTSub != "" {
		q += fmt.Sprintf(" AND jwt_sub = $%d", idx)
		args = append(args, f.JWTSub)
	}

	var sum ClaudeCodeUsageSummary
	err := s.db.QueryRowContext(ctx, q, args...).Scan(
		&sum.Requests, &sum.InputTokens, &sum.OutputTokens, &sum.TotalTokens, &sum.TotalCost,
	)
	if err != nil {
		return ClaudeCodeUsageSummary{}, fmt.Errorf("get claude code usage summary: %w", err)
	}
	if sum.Requests > 0 {
		sum.AvgCostPerRequest = sum.TotalCost / float64(sum.Requests)
	}
	return sum, nil
}

// GetClaudeCodeUsageTimeseries returns daily-bucketed aggregates ordered by bucket ASC.
func (s *PostgresStorage) GetClaudeCodeUsageTimeseries(ctx context.Context, f ClaudeCodeUsageFilter) ([]ClaudeCodeTimeseriesBucket, error) {
	q := `SELECT
		DATE_TRUNC('day', created_at) AS bucket,
		COUNT(*),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cost), 0)
	FROM anthropic_message_log
	WHERE tenant_id = $1
	  AND created_at >= $2
	  AND created_at < $3`
	args := []interface{}{f.TenantID, f.From, f.To}
	idx := 4
	if f.APIKeyName != "" {
		q += fmt.Sprintf(" AND api_key_name = $%d", idx)
		args = append(args, f.APIKeyName)
		idx++
	}
	if f.JWTSub != "" {
		q += fmt.Sprintf(" AND jwt_sub = $%d", idx)
		args = append(args, f.JWTSub)
	}
	q += " GROUP BY bucket ORDER BY bucket ASC"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("get claude code usage timeseries: %w", err)
	}
	defer rows.Close()

	var buckets []ClaudeCodeTimeseriesBucket
	for rows.Next() {
		var b ClaudeCodeTimeseriesBucket
		if err := rows.Scan(&b.Bucket, &b.Requests, &b.InputTokens, &b.OutputTokens, &b.TotalCost); err != nil {
			return nil, fmt.Errorf("scan timeseries row: %w", err)
		}
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// GetClaudeCodeUsageRows returns paginated raw rows and total count.
func (s *PostgresStorage) GetClaudeCodeUsageRows(ctx context.Context, f ClaudeCodeUsageFilter) ([]ClaudeCodeUsageRow, int64, error) {
	// Build shared WHERE clause.
	where := "WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3"
	args := []interface{}{f.TenantID, f.From, f.To}
	idx := 4
	if f.APIKeyName != "" {
		where += fmt.Sprintf(" AND api_key_name = $%d", idx)
		args = append(args, f.APIKeyName)
		idx++
	}
	if f.JWTSub != "" {
		where += fmt.Sprintf(" AND jwt_sub = $%d", idx)
		args = append(args, f.JWTSub)
		idx++
	}

	// Total count.
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM anthropic_message_log "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("get claude code usage count: %w", err)
	}

	// Paginated rows.
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	rowQ := fmt.Sprintf(`SELECT
		created_at, request_id, tenant_id, api_key_name, jwt_sub,
		model_requested, model_used,
		input_tokens, output_tokens, total_tokens, cost,
		status_code, success, stop_reason, latency_ms
	FROM anthropic_message_log %s
	ORDER BY created_at DESC
	LIMIT $%d OFFSET $%d`, where, idx, idx+1)
	args = append(args, limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, rowQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("get claude code usage rows: %w", err)
	}
	defer rows.Close()

	var out []ClaudeCodeUsageRow
	for rows.Next() {
		var r ClaudeCodeUsageRow
		if err := rows.Scan(
			&r.CreatedAt, &r.RequestID, &r.TenantID, &r.APIKeyName, &r.JWTSub,
			&r.ModelRequested, &r.ModelUsed,
			&r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.Cost,
			&r.StatusCode, &r.Success, &r.StopReason, &r.LatencyMs,
		); err != nil {
			return nil, 0, fmt.Errorf("scan usage row: %w", err)
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}
