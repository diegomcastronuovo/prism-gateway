package storage

import (
	"context"
	"fmt"
	"time"
)

// GetMonthlySpend returns the total cost_usd for a tenant in [from, to) — read-only, no reservation.
func (s *PostgresStorage) GetMonthlySpend(ctx context.Context, tenantID string, from, to time.Time) (float64, error) {
	var spend float64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0.0) FROM usage WHERE tenant_id = $1 AND ts >= $2 AND ts < $3`,
		tenantID, from, to,
	).Scan(&spend)
	if err != nil {
		return 0, fmt.Errorf("get monthly spend: %w", err)
	}
	return spend, nil
}

// GetTagMonthlySpend returns the total cost_usd for requests where metadata->>tagKey == tagValue.
func (s *PostgresStorage) GetTagMonthlySpend(
	ctx context.Context, tenantID, tagKey, tagValue string, from, to time.Time,
) (float64, error) {
	var spend float64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(u.cost_usd), 0.0)
         FROM request_log rl
         LEFT JOIN usage u ON rl.request_id = u.request_id
         WHERE rl.tenant_id = $1 AND rl.ts >= $2 AND rl.ts < $3
           AND rl.status = 'ok'
           AND rl.metadata->>$4 = $5`,
		tenantID, from, to, tagKey, tagValue,
	).Scan(&spend)
	if err != nil {
		return 0, fmt.Errorf("get tag monthly spend: %w", err)
	}
	return spend, nil
}
