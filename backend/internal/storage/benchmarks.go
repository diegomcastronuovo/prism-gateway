package storage

import (
	"context"
	"time"
)

// InsertModelBenchmark stores one benchmark result row.
func (p *PostgresStorage) InsertModelBenchmark(ctx context.Context, row ModelBenchmarkRow) error {
	const q = `
INSERT INTO model_benchmarks
    (id, ts, provider, model, success, latency_ms,
     prompt_tokens, completion_tokens, total_tokens,
     cost_usd, error_type, benchmark_name)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`
	_, err := p.db.ExecContext(ctx, q,
		row.ID, row.Ts, row.Provider, row.Model, row.Success, row.LatencyMs,
		row.PromptTokens, row.CompletionTokens, row.TotalTokens,
		row.CostUSD, row.ErrorType, row.BenchmarkName,
	)
	return err
}

// GetModelBenchmarkAggregates returns per-model aggregate stats over the last
// windowHours hours. If windowHours <= 0 it defaults to 24.
func (p *PostgresStorage) GetModelBenchmarkAggregates(ctx context.Context, windowHours int) ([]BenchmarkAggregate, error) {
	if windowHours <= 0 {
		windowHours = 24
	}
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)

	const q = `
SELECT
    provider,
    model,
    AVG(latency_ms)                                         AS avg_latency_ms,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms) AS p95_latency_ms,
    AVG(CASE WHEN success THEN 1.0 ELSE 0.0 END)           AS success_rate,
    AVG(cost_usd)                                           AS avg_cost_usd,
    COUNT(*)                                                AS samples
FROM model_benchmarks
WHERE ts >= $1
GROUP BY provider, model
ORDER BY model`

	rows, err := p.db.QueryContext(ctx, q, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BenchmarkAggregate
	for rows.Next() {
		var a BenchmarkAggregate
		if err := rows.Scan(
			&a.Provider, &a.Model,
			&a.AvgLatencyMs, &a.P95LatencyMs,
			&a.SuccessRate, &a.AvgCostUSD, &a.Samples,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TruncateModelBenchmarks deletes all rows from model_benchmarks (admin reset).
func (p *PostgresStorage) TruncateModelBenchmarks(ctx context.Context) (int64, error) {
	res, err := p.db.ExecContext(ctx, `DELETE FROM model_benchmarks`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteOldModelBenchmarks removes rows older than retainDays days.
func (p *PostgresStorage) DeleteOldModelBenchmarks(ctx context.Context, retainDays int) (int64, error) {
	if retainDays <= 0 {
		retainDays = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retainDays)
	res, err := p.db.ExecContext(ctx,
		`DELETE FROM model_benchmarks WHERE ts < $1`, cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
