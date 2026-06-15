package storage

import (
	"context"
	"fmt"
	"time"
)

// StreamBillingLineItems streams request-level billing rows for a tenant/month
// using a cursor pattern to avoid loading all rows into memory.
// fn is called once per row; if fn returns a non-nil error the iteration stops.
func (s *PostgresStorage) StreamBillingLineItems(ctx context.Context, tenantID string, from, to time.Time, fn func(BillingLineItem) error) error {
	const query = `
		SELECT
			rl.ts,
			rl.request_id,
			rl.tenant_id,
			rl.model,
			rl.provider,
			rl.status,
			COALESCE(u.total_tokens, 0),
			COALESCE(u.prompt_tokens, 0),
			COALESCE(u.completion_tokens, 0),
			COALESCE(u.cost_usd, 0.0),
			COALESCE(rl.metadata->>'project', ''),
			COALESCE(rl.metadata->>'cost_center', ''),
			COALESCE(rl.metadata->>'env', ''),
			COALESCE(rl.metadata->>'application', '')
		FROM request_log rl
		LEFT JOIN usage u ON rl.id = u.request_id
		WHERE rl.tenant_id = $1 AND rl.ts >= $2 AND rl.ts < $3
		ORDER BY rl.ts ASC`

	rows, err := s.db.QueryContext(ctx, query, tenantID, from, to)
	if err != nil {
		return fmt.Errorf("query billing line items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item BillingLineItem
		if err := rows.Scan(
			&item.Timestamp,
			&item.RequestID,
			&item.TenantID,
			&item.Model,
			&item.Provider,
			&item.Status,
			&item.TotalTokens,
			&item.PromptTokens,
			&item.CompletionTokens,
			&item.CostUSD,
			&item.Project,
			&item.CostCenter,
			&item.Env,
			&item.Application,
		); err != nil {
			return fmt.Errorf("scan billing line item: %w", err)
		}
		if err := fn(item); err != nil {
			return err
		}
	}
	return rows.Err()
}

// GetBillingGrouped returns aggregated billing rows grouped by the given field.
// groupBy must be one of: "model", "provider", "project", "cost_center", "env", "application".
// The group expression is selected via a switch to prevent SQL injection.
func (s *PostgresStorage) GetBillingGrouped(ctx context.Context, tenantID string, from, to time.Time, groupBy string) ([]BillingGroupedRow, error) {
	var groupExpr string
	switch groupBy {
	case "model":
		groupExpr = "rl.model"
	case "provider":
		groupExpr = "rl.provider"
	case "project":
		groupExpr = "COALESCE(rl.metadata->>'project', '')"
	case "cost_center":
		groupExpr = "COALESCE(rl.metadata->>'cost_center', '')"
	case "env":
		groupExpr = "COALESCE(rl.metadata->>'env', '')"
	case "application":
		groupExpr = "COALESCE(rl.metadata->>'application', '')"
	default:
		return nil, fmt.Errorf("unsupported group_by: %s", groupBy)
	}

	query := fmt.Sprintf(`
		SELECT %s AS group_key,
		       COUNT(*)                             AS requests_count,
		       COALESCE(SUM(u.prompt_tokens), 0)    AS prompt_tokens,
		       COALESCE(SUM(u.completion_tokens), 0) AS completion_tokens,
		       COALESCE(SUM(u.total_tokens), 0)     AS total_tokens,
		       COALESCE(SUM(u.cost_usd), 0.0)       AS cost_usd
		FROM request_log rl
		LEFT JOIN usage u ON rl.id = u.request_id
		WHERE rl.tenant_id = $1 AND rl.ts >= $2 AND rl.ts < $3
		GROUP BY group_key
		ORDER BY group_key ASC`, groupExpr)

	rows, err := s.db.QueryContext(ctx, query, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query billing grouped: %w", err)
	}
	defer rows.Close()

	var result []BillingGroupedRow
	for rows.Next() {
		var row BillingGroupedRow
		if err := rows.Scan(
			&row.GroupKey,
			&row.RequestsCount,
			&row.PromptTokens,
			&row.CompletionTokens,
			&row.TotalTokens,
			&row.CostUSD,
		); err != nil {
			return nil, fmt.Errorf("scan billing grouped row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate billing grouped rows: %w", err)
	}
	return result, nil
}

// GetUsageByTag aggregates usage for a tenant/time-range grouped by the given metadata tag key.
// tag is passed as a query parameter ($4) — no string interpolation — preventing SQL injection.
func (s *PostgresStorage) GetUsageByTag(ctx context.Context, tenantID string, from, to time.Time, tag string) ([]UsageByTagRow, error) {
	const query = `
		SELECT COALESCE(rl.metadata->>$4, '') AS tag_value,
		       COUNT(*)                         AS requests,
		       COALESCE(SUM(u.total_tokens), 0) AS total_tokens,
		       COALESCE(SUM(u.cost_usd), 0.0)   AS cost_usd
		FROM request_log rl
		LEFT JOIN usage u ON rl.id = u.request_id
		WHERE rl.tenant_id = $1 AND rl.ts >= $2 AND rl.ts < $3
		GROUP BY tag_value
		ORDER BY tag_value ASC`

	rows, err := s.db.QueryContext(ctx, query, tenantID, from, to, tag)
	if err != nil {
		return nil, fmt.Errorf("query usage by tag: %w", err)
	}
	defer rows.Close()

	var result []UsageByTagRow
	for rows.Next() {
		var row UsageByTagRow
		if err := rows.Scan(&row.Value, &row.Requests, &row.TotalTokens, &row.CostUSD); err != nil {
			return nil, fmt.Errorf("scan usage by tag row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage by tag rows: %w", err)
	}
	return result, nil
}
