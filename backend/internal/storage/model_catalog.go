package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// ListModelCatalog returns all model_catalog entries matching the filter,
// ordered by provider then id.
func (s *PostgresStorage) ListModelCatalog(ctx context.Context, filter ModelCatalogFilter) ([]ModelCatalogEntry, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if filter.Provider != nil {
		conditions = append(conditions, fmt.Sprintf("provider = $%d", argIdx))
		args = append(args, *filter.Provider)
		argIdx++
	}
	if filter.Type != nil {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, *filter.Type)
		argIdx++
	}
	if filter.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *filter.IsActive)
		argIdx++
	}

	query := `
		SELECT id, provider, display_name, type,
		       prompt_per_1m, cached_input_per_1m, completion_per_1m, infrastructure_monthly_usd,
		       is_active, long_context, long_context_start_tokens,
		       long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m,
		       created_at, updated_at
		FROM model_catalog`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY provider, id"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list model catalog: %w", err)
	}
	defer rows.Close()

	var entries []ModelCatalogEntry
	for rows.Next() {
		var e ModelCatalogEntry
		if err := rows.Scan(
			&e.ID, &e.Provider, &e.DisplayName, &e.Type,
			&e.PromptPer1M, &e.CachedInputPer1M, &e.CompletionPer1M, &e.InfrastructureMonthlyUSD,
			&e.IsActive, &e.LongContext, &e.LongContextStartTokens,
			&e.LongContextPromptPer1M, &e.LongContextCachedInputPer1M, &e.LongContextCompletionPer1M,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan model catalog entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list model catalog rows: %w", err)
	}
	if entries == nil {
		entries = []ModelCatalogEntry{}
	}
	return entries, nil
}

// GetModelCatalogEntry returns a single entry by (provider, id).
func (s *PostgresStorage) GetModelCatalogEntry(ctx context.Context, provider, id string) (ModelCatalogEntry, bool, error) {
	const q = `
		SELECT id, provider, display_name, type,
		       prompt_per_1m, cached_input_per_1m, completion_per_1m, infrastructure_monthly_usd,
		       is_active, long_context, long_context_start_tokens,
		       long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m,
		       created_at, updated_at
		FROM model_catalog
		WHERE provider = $1 AND id = $2`

	var e ModelCatalogEntry
	err := s.db.QueryRowContext(ctx, q, provider, id).Scan(
		&e.ID, &e.Provider, &e.DisplayName, &e.Type,
		&e.PromptPer1M, &e.CachedInputPer1M, &e.CompletionPer1M, &e.InfrastructureMonthlyUSD,
		&e.IsActive, &e.LongContext, &e.LongContextStartTokens,
		&e.LongContextPromptPer1M, &e.LongContextCachedInputPer1M, &e.LongContextCompletionPer1M,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return ModelCatalogEntry{}, false, nil
		}
		return ModelCatalogEntry{}, false, fmt.Errorf("get model catalog entry: %w", err)
	}
	return e, true, nil
}

// CreateModelCatalogEntry inserts a new entry. Returns an error containing
// "already exists" when the (provider, id) pair already exists.
func (s *PostgresStorage) CreateModelCatalogEntry(ctx context.Context, entry ModelCatalogEntry) (ModelCatalogEntry, error) {
	const q = `
		INSERT INTO model_catalog
		    (id, provider, display_name, type,
		     prompt_per_1m, cached_input_per_1m, completion_per_1m, infrastructure_monthly_usd,
		     is_active, long_context, long_context_start_tokens,
		     long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m,
		     created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (provider, id) DO NOTHING
		RETURNING id, provider, display_name, type,
		          prompt_per_1m, cached_input_per_1m, completion_per_1m, infrastructure_monthly_usd,
		          is_active, long_context, long_context_start_tokens,
		          long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m,
		          created_at, updated_at`

	now := time.Now().UTC()
	entry.CreatedAt = now
	entry.UpdatedAt = now

	var inserted ModelCatalogEntry
	err := s.db.QueryRowContext(ctx, q,
		entry.ID, entry.Provider, entry.DisplayName, entry.Type,
		entry.PromptPer1M, entry.CachedInputPer1M, entry.CompletionPer1M, entry.InfrastructureMonthlyUSD,
		entry.IsActive, entry.LongContext, entry.LongContextStartTokens,
		entry.LongContextPromptPer1M, entry.LongContextCachedInputPer1M, entry.LongContextCompletionPer1M,
		entry.CreatedAt, entry.UpdatedAt,
	).Scan(
		&inserted.ID, &inserted.Provider, &inserted.DisplayName, &inserted.Type,
		&inserted.PromptPer1M, &inserted.CachedInputPer1M, &inserted.CompletionPer1M, &inserted.InfrastructureMonthlyUSD,
		&inserted.IsActive, &inserted.LongContext, &inserted.LongContextStartTokens,
		&inserted.LongContextPromptPer1M, &inserted.LongContextCachedInputPer1M, &inserted.LongContextCompletionPer1M,
		&inserted.CreatedAt, &inserted.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			// ON CONFLICT DO NOTHING — row already existed.
			return ModelCatalogEntry{}, fmt.Errorf("model catalog entry already exists: provider=%s id=%s", entry.Provider, entry.ID)
		}
		return ModelCatalogEntry{}, fmt.Errorf("create model catalog entry: %w", err)
	}
	return inserted, nil
}

// UpdateModelCatalogEntry replaces a catalog entry and returns the updated row.
func (s *PostgresStorage) UpdateModelCatalogEntry(ctx context.Context, entry ModelCatalogEntry) (ModelCatalogEntry, bool, error) {
	const q = `
		UPDATE model_catalog
		SET display_name                      = $3,
		    type                              = $4,
		    prompt_per_1m                     = $5,
		    cached_input_per_1m               = $6,
		    completion_per_1m                 = $7,
		    infrastructure_monthly_usd        = $8,
		    is_active                         = $9,
		    long_context                      = $10,
		    long_context_start_tokens         = $11,
		    long_context_prompt_per_1m        = $12,
		    long_context_cached_input_per_1m  = $13,
		    long_context_completion_per_1m    = $14,
		    updated_at                        = $15
		WHERE provider = $1 AND id = $2
		RETURNING id, provider, display_name, type,
		          prompt_per_1m, cached_input_per_1m, completion_per_1m, infrastructure_monthly_usd,
		          is_active, long_context, long_context_start_tokens,
		          long_context_prompt_per_1m, long_context_cached_input_per_1m, long_context_completion_per_1m,
		          created_at, updated_at`

	entry.UpdatedAt = time.Now().UTC()

	var updated ModelCatalogEntry
	err := s.db.QueryRowContext(ctx, q,
		entry.Provider, entry.ID,
		entry.DisplayName, entry.Type,
		entry.PromptPer1M, entry.CachedInputPer1M, entry.CompletionPer1M, entry.InfrastructureMonthlyUSD,
		entry.IsActive, entry.LongContext, entry.LongContextStartTokens,
		entry.LongContextPromptPer1M, entry.LongContextCachedInputPer1M, entry.LongContextCompletionPer1M,
		entry.UpdatedAt,
	).Scan(
		&updated.ID, &updated.Provider, &updated.DisplayName, &updated.Type,
		&updated.PromptPer1M, &updated.CachedInputPer1M, &updated.CompletionPer1M, &updated.InfrastructureMonthlyUSD,
		&updated.IsActive, &updated.LongContext, &updated.LongContextStartTokens,
		&updated.LongContextPromptPer1M, &updated.LongContextCachedInputPer1M, &updated.LongContextCompletionPer1M,
		&updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return ModelCatalogEntry{}, false, nil
		}
		return ModelCatalogEntry{}, false, fmt.Errorf("update model catalog entry: %w", err)
	}
	return updated, true, nil
}

// DeleteModelCatalogEntry removes an entry. Returns true if a row was deleted.
func (s *PostgresStorage) DeleteModelCatalogEntry(ctx context.Context, provider, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM model_catalog WHERE provider = $1 AND id = $2`,
		provider, id,
	)
	if err != nil {
		return false, fmt.Errorf("delete model catalog entry: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete model catalog rows affected: %w", err)
	}
	return n > 0, nil
}

// ListCatalogPricing returns a minimal pricing projection of all active model_catalog rows.
// Used by config.CatalogEnricher to enrich ModelConfig.Pricing at cache-load time.
func (s *PostgresStorage) ListCatalogPricing(ctx context.Context) ([]config.CatalogPricingRow, error) {
	const q = `
		SELECT provider, id, cached_input_per_1m,
		       long_context, long_context_start_tokens,
		       long_context_prompt_per_1m, long_context_cached_input_per_1m,
		       long_context_completion_per_1m,
		       cache_write_5m_per_1m, cache_write_1h_per_1m, geo_multiplier_us
		FROM model_catalog
		WHERE is_active = TRUE`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []config.CatalogPricingRow
	for rows.Next() {
		var r config.CatalogPricingRow
		if err := rows.Scan(
			&r.Provider, &r.ID, &r.CachedInputPer1M,
			&r.LongContext, &r.LongContextStartTokens,
			&r.LongContextPromptPer1M, &r.LongContextCachedInputPer1M,
			&r.LongContextCompletionPer1M,
			&r.CacheWrite5mPer1M, &r.CacheWrite1hPer1M, &r.GeoMultiplierUS,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
