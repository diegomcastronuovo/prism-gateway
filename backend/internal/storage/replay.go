package storage

import (
	"context"
	"fmt"
	"time"
)

// GetReplayRequests retrieves successful request_log rows joined with usage data
// for traffic replay simulation. Returns at most limit rows ordered by timestamp descending.
func (s *PostgresStorage) GetReplayRequests(
	ctx context.Context,
	tenantID string,
	from, to time.Time,
	limit int,
) ([]ReplayRow, error) {
	const q = `
		SELECT
			rl.request_id,
			rl.ts,
			rl.tenant_id,
			rl.model,
			rl.strategy,
			COALESCE(u.prompt_tokens, 0)     AS prompt_tokens,
			COALESCE(u.completion_tokens, 0) AS completion_tokens,
			COALESCE(u.cost_usd, 0.0)        AS cost_usd,
			rl.routing_snapshot
		FROM request_log rl
		LEFT JOIN usage u ON u.request_id = rl.request_id
		WHERE rl.tenant_id     = $1
		  AND rl.ts            >= $2
		  AND rl.ts            <  $3
		  AND rl.status        = 'ok'
		  AND rl.routing_snapshot IS NOT NULL
		ORDER BY rl.ts DESC
		LIMIT $4`

	rows, err := s.db.QueryContext(ctx, q, tenantID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("get replay requests: %w", err)
	}
	defer rows.Close()

	var results []ReplayRow
	for rows.Next() {
		var row ReplayRow
		if err := rows.Scan(
			&row.RequestID, &row.Timestamp, &row.TenantID,
			&row.Model, &row.Strategy,
			&row.PromptTokens, &row.CompletionTokens, &row.CostUSD,
			&row.RoutingSnapshot,
		); err != nil {
			return nil, fmt.Errorf("scan replay row: %w", err)
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		results = []ReplayRow{}
	}
	return results, nil
}
