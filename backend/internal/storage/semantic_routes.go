package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// CreateSemanticRoute inserts all utterance rows for a named route in a single transaction.
// Returns ErrRouteAlreadyExists if any row for (tenant_id, name) already exists.
func (s *PostgresStorage) CreateSemanticRoute(
	ctx context.Context,
	tenantID, name, description, action string,
	threshold float64,
	utterances []string,
	embeddings [][]float64,
) error {
	if len(utterances) == 0 || len(utterances) != len(embeddings) {
		return fmt.Errorf("utterances and embeddings must be non-empty and equal length")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check for existing rows with this (tenant_id, name).
	var count int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM semantic_routes WHERE tenant_id = $1 AND name = $2`,
		tenantID, name,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check existing route: %w", err)
	}
	if count > 0 {
		return ErrRouteAlreadyExists
	}

	// Insert one row per utterance.
	for i, utterance := range utterances {
		vecStr := formatVector(embeddings[i])
		_, err = tx.ExecContext(ctx, `
			INSERT INTO semantic_routes
			    (tenant_id, name, description, action, utterance, embedding, threshold)
			VALUES ($1, $2, $3, $4, $5, $6::vector, $7)
		`, tenantID, name, description, action, utterance, vecStr, threshold)
		if err != nil {
			return fmt.Errorf("insert semantic route utterance %d: %w", i, err)
		}
	}

	return tx.Commit()
}

// GetNearestSemanticRoute finds the closest utterance embedding across all routes for the
// tenant. Returns (match, true, nil) on hit; (zero, false, nil) if no routes exist.
func (s *PostgresStorage) GetNearestSemanticRoute(
	ctx context.Context,
	tenantID string,
	embedding []float64,
) (SemanticRouteMatch, bool, error) {
	if len(embedding) == 0 {
		return SemanticRouteMatch{}, false, nil
	}

	vecStr := formatVector(embedding)

	const query = `
		SELECT name, action, threshold,
		       1 - (embedding <=> $2::vector) AS similarity
		FROM semantic_routes
		WHERE tenant_id = $1
		ORDER BY embedding <=> $2::vector
		LIMIT 1`

	var match SemanticRouteMatch
	err := s.db.QueryRowContext(ctx, query, tenantID, vecStr).
		Scan(&match.Name, &match.Action, &match.Threshold, &match.Similarity)
	if err != nil {
		if err == sql.ErrNoRows {
			return SemanticRouteMatch{}, false, nil
		}
		return SemanticRouteMatch{}, false, fmt.Errorf("get nearest semantic route: %w", err)
	}

	return match, true, nil
}

// DeleteSemanticRoute removes all utterance rows for (tenant_id, name).
// Returns (true, nil) if at least one row was deleted, (false, nil) if not found.
func (s *PostgresStorage) DeleteSemanticRoute(
	ctx context.Context,
	tenantID, name string,
) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM semantic_routes WHERE tenant_id = $1 AND name = $2`,
		tenantID, name,
	)
	if err != nil {
		return false, fmt.Errorf("delete semantic route: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return rows > 0, nil
}

// GetSemanticRoute returns the named route with its full utterance list.
// Returns (detail, true, nil) if found, (zero, false, nil) if the route does not exist.
func (s *PostgresStorage) GetSemanticRoute(ctx context.Context, tenantID, name string) (SemanticRouteDetail, bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT description, action, threshold, utterance
		FROM semantic_routes
		WHERE tenant_id = $1 AND name = $2
		ORDER BY utterance`,
		tenantID, name,
	)
	if err != nil {
		return SemanticRouteDetail{}, false, fmt.Errorf("get semantic route: %w", err)
	}
	defer rows.Close()

	var detail SemanticRouteDetail
	detail.Name = name
	for rows.Next() {
		var utterance string
		if err := rows.Scan(&detail.Description, &detail.Action, &detail.Threshold, &utterance); err != nil {
			return SemanticRouteDetail{}, false, fmt.Errorf("scan semantic route: %w", err)
		}
		detail.Utterances = append(detail.Utterances, utterance)
	}
	if err := rows.Err(); err != nil {
		return SemanticRouteDetail{}, false, err
	}
	if len(detail.Utterances) == 0 {
		return SemanticRouteDetail{}, false, nil // not found
	}
	return detail, true, nil
}

// UpdateSemanticRoute applies a partial update to an existing route atomically.
//
// Merge semantics: nil patch fields preserve the current persisted value.
//
// If patch.Utterances is non-nil, existing utterance rows are deleted and replaced
// with the new set in a single transaction (embeddings must match len(patch.Utterances)).
// If patch.Utterances is nil, only metadata columns (description, action, threshold)
// are updated in-place on all rows for the route.
//
// Returns (true, nil) on success, (false, nil) if the route does not exist.
func (s *PostgresStorage) UpdateSemanticRoute(
	ctx context.Context,
	tenantID, name string,
	patch SemanticRoutePatch,
	embeddings [][]float64,
) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Load current metadata to support merge for absent patch fields.
	var curDesc, curAction string
	var curThreshold float64
	err = tx.QueryRowContext(ctx,
		`SELECT description, action, threshold FROM semantic_routes WHERE tenant_id = $1 AND name = $2 LIMIT 1`,
		tenantID, name,
	).Scan(&curDesc, &curAction, &curThreshold)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load current route: %w", err)
	}

	// Compute effective values (patch overrides current).
	effectiveDesc := curDesc
	if patch.Description != nil {
		effectiveDesc = *patch.Description
	}
	effectiveAction := curAction
	if patch.Action != nil {
		effectiveAction = *patch.Action
	}
	effectiveThreshold := curThreshold
	if patch.Threshold != nil {
		effectiveThreshold = *patch.Threshold
	}

	if patch.Utterances != nil {
		// Delete + insert: replaces utterance set atomically.
		if _, err = tx.ExecContext(ctx,
			`DELETE FROM semantic_routes WHERE tenant_id = $1 AND name = $2`,
			tenantID, name,
		); err != nil {
			return false, fmt.Errorf("delete old utterances: %w", err)
		}
		for i, utterance := range patch.Utterances {
			vecStr := formatVector(embeddings[i])
			if _, err = tx.ExecContext(ctx, `
				INSERT INTO semantic_routes
				    (tenant_id, name, description, action, utterance, embedding, threshold)
				VALUES ($1, $2, $3, $4, $5, $6::vector, $7)
			`, tenantID, name, effectiveDesc, effectiveAction, utterance, vecStr, effectiveThreshold); err != nil {
				return false, fmt.Errorf("insert utterance %d: %w", i, err)
			}
		}
	} else {
		// Metadata-only update in place.
		result, err := tx.ExecContext(ctx, `
			UPDATE semantic_routes
			SET description = $3, action = $4, threshold = $5
			WHERE tenant_id = $1 AND name = $2`,
			tenantID, name, effectiveDesc, effectiveAction, effectiveThreshold,
		)
		if err != nil {
			return false, fmt.Errorf("update route metadata: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
	}

	return true, tx.Commit()
}

// ListSemanticRoutes returns one row per distinct (name) for the tenant.
func (s *PostgresStorage) ListSemanticRoutes(
	ctx context.Context,
	tenantID string,
) ([]SemanticRouteRow, error) {
	const query = `
		SELECT DISTINCT ON (name) name, description, action, threshold, created_at
		FROM semantic_routes
		WHERE tenant_id = $1
		ORDER BY name, created_at`

	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list semantic routes: %w", err)
	}
	defer rows.Close()

	var results []SemanticRouteRow
	for rows.Next() {
		var r SemanticRouteRow
		if err := rows.Scan(&r.Name, &r.Description, &r.Action, &r.Threshold, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan semantic route: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
