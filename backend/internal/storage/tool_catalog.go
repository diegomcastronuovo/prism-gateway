package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// ListToolCatalog returns all tool_catalog entries matching the filter,
// ordered by provider then id.
func (s *PostgresStorage) ListToolCatalog(ctx context.Context, filter ToolCatalogFilter) ([]ToolCatalogEntry, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if filter.Provider != nil {
		conditions = append(conditions, fmt.Sprintf("provider = $%d", argIdx))
		args = append(args, *filter.Provider)
		argIdx++
	}
	if filter.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *filter.IsActive)
		argIdx++
	}

	query := `
		SELECT id, provider, display_name, tool_type, unit, price_per_unit, is_active
		FROM tool_catalog`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY provider, id"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tool catalog: %w", err)
	}
	defer rows.Close()

	var entries []ToolCatalogEntry
	for rows.Next() {
		var e ToolCatalogEntry
		if err := rows.Scan(
			&e.ID, &e.Provider, &e.DisplayName, &e.ToolType, &e.Unit, &e.PricePerUnit, &e.IsActive,
		); err != nil {
			return nil, fmt.Errorf("scan tool catalog entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tool catalog rows: %w", err)
	}
	if entries == nil {
		entries = []ToolCatalogEntry{}
	}
	return entries, nil
}

// GetToolCatalogEntry returns a single entry by (provider, id).
// Returns (nil, nil) if not found.
func (s *PostgresStorage) GetToolCatalogEntry(ctx context.Context, provider, id string) (*ToolCatalogEntry, error) {
	const q = `
		SELECT id, provider, display_name, tool_type, unit, price_per_unit, is_active
		FROM tool_catalog
		WHERE provider = $1 AND id = $2`

	var e ToolCatalogEntry
	err := s.db.QueryRowContext(ctx, q, provider, id).Scan(
		&e.ID, &e.Provider, &e.DisplayName, &e.ToolType, &e.Unit, &e.PricePerUnit, &e.IsActive,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get tool catalog entry: %w", err)
	}
	return &e, nil
}

// CreateToolCatalogEntry inserts a new entry. Returns ErrToolAlreadyExists when the
// (provider, id) pair already exists.
func (s *PostgresStorage) CreateToolCatalogEntry(ctx context.Context, entry ToolCatalogEntry) error {
	const q = `
		INSERT INTO tool_catalog
		    (id, provider, display_name, tool_type, unit, price_per_unit, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (provider, id) DO NOTHING`

	res, err := s.db.ExecContext(ctx, q,
		entry.ID, entry.Provider, entry.DisplayName, entry.ToolType,
		entry.Unit, entry.PricePerUnit, entry.IsActive,
	)
	if err != nil {
		return fmt.Errorf("create tool catalog entry: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("create tool catalog rows affected: %w", err)
	}
	if n == 0 {
		return ErrToolAlreadyExists
	}
	return nil
}

// UpdateToolCatalogEntry replaces a tool catalog entry's mutable fields.
// Returns ErrNotFound if no row matches (provider, id).
func (s *PostgresStorage) UpdateToolCatalogEntry(ctx context.Context, provider, id string, entry ToolCatalogEntry) error {
	const q = `
		UPDATE tool_catalog
		SET display_name  = $3,
		    tool_type     = $4,
		    unit          = $5,
		    price_per_unit = $6,
		    is_active     = $7,
		    updated_at    = NOW()
		WHERE provider = $1 AND id = $2`

	res, err := s.db.ExecContext(ctx, q,
		provider, id,
		entry.DisplayName, entry.ToolType, entry.Unit, entry.PricePerUnit, entry.IsActive,
	)
	if err != nil {
		return fmt.Errorf("update tool catalog entry: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update tool catalog rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListToolCatalogPricing returns a minimal pricing projection of all active tool_catalog rows.
// Used by config.ToolCatalogEnricher to enrich GlobalConfig.ToolPricing at cache-load time.
func (s *PostgresStorage) ListToolCatalogPricing(ctx context.Context) ([]config.ToolCatalogPricingRow, error) {
	const q = `SELECT provider, id, tool_type, price_per_unit, unit FROM tool_catalog WHERE is_active = TRUE`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []config.ToolCatalogPricingRow
	for rows.Next() {
		var r config.ToolCatalogPricingRow
		if err := rows.Scan(&r.Provider, &r.ID, &r.ToolType, &r.PricePerUnit, &r.Unit); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// DeleteToolCatalogEntry removes an entry by (provider, id).
// Returns ErrNotFound if no row matched.
func (s *PostgresStorage) DeleteToolCatalogEntry(ctx context.Context, provider, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM tool_catalog WHERE provider = $1 AND id = $2`,
		provider, id,
	)
	if err != nil {
		return fmt.Errorf("delete tool catalog entry: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete tool catalog rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
