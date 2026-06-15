package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// GetNearestSemanticAnchor returns the closest semantic anchor (by cosine distance) for
// the given tenant and prompt embedding. Uses the HNSW index (idx_semantic_anchors_hnsw).
//
// The embedding is passed as a pgvector literal string so no extra library is needed.
// Returns found=false (nil error) when there are no anchors for the tenant.
func (s *PostgresStorage) GetNearestSemanticAnchor(
	ctx context.Context,
	tenantID string,
	embedding []float64,
	modality string,
) (name, routeGroup string, preferredModels []string, distance float64, found bool, err error) {
	if len(embedding) == 0 {
		return "", "", nil, 0, false, nil
	}

	vecStr := formatVector(embedding)

	const query = `
		SELECT name, route_group, preferred_models,
		       embedding <=> $2::vector AS distance
		FROM semantic_anchors
		WHERE tenant_id = $1 AND modality = $3
		ORDER BY embedding <=> $2::vector
		LIMIT 1`

	var rawModels []byte
	row := s.db.QueryRowContext(ctx, query, tenantID, vecStr, modality)
	if err = row.Scan(&name, &routeGroup, &rawModels, &distance); err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil, 0, false, nil
		}
		return "", "", nil, 0, false, err
	}

	if jsonErr := json.Unmarshal(rawModels, &preferredModels); jsonErr != nil {
		preferredModels = nil
	}

	return name, routeGroup, preferredModels, distance, true, nil
}

// UpsertSemanticAnchor inserts a new row into semantic_anchors.
// Returns ErrAnchorAlreadyExists if the unique index on (tenant_id, name) fires.
func (s *PostgresStorage) UpsertSemanticAnchor(
	ctx context.Context,
	tenantID, name string,
	embedding []float64,
	routeGroup string,
	preferredModels []string,
	anchorText *string,
	modality string,
) error {
	if preferredModels == nil {
		preferredModels = []string{}
	}
	modelsJSON, err := json.Marshal(preferredModels)
	if err != nil {
		return fmt.Errorf("marshal preferred_models: %w", err)
	}

	vecStr := formatVector(embedding)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO semantic_anchors (tenant_id, name, embedding, route_group, preferred_models, anchor_text, modality)
		VALUES ($1, $2, $3::vector, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, name) DO NOTHING
	`, tenantID, name, vecStr, routeGroup, modelsJSON, anchorText, modality)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return ErrAnchorAlreadyExists
		}
		return fmt.Errorf("insert semantic anchor: %w", err)
	}
	return nil
}

// ListSemanticAnchorsPaged returns anchors for a tenant ordered by name with cursor-style pagination.
// Fetches limit+1 rows to determine hasMore. If includeAnchorText is false, AnchorText is set to nil.
func (s *PostgresStorage) ListSemanticAnchorsPaged(
	ctx context.Context,
	tenantID string,
	includeAnchorText bool,
	limit, offset int,
) ([]SemanticAnchorMeta, bool, error) {
	const query = `
		SELECT name, route_group, preferred_models, anchor_text, vector_dims(embedding) AS dims, modality
		FROM semantic_anchors
		WHERE tenant_id = $1
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`

	rows, err := s.db.QueryContext(ctx, query, tenantID, limit+1, offset)
	if err != nil {
		return nil, false, fmt.Errorf("list semantic anchors paged: %w", err)
	}
	defer rows.Close()

	var results []SemanticAnchorMeta
	for rows.Next() {
		var m SemanticAnchorMeta
		var rawModels []byte
		var anchorText *string
		if err := rows.Scan(&m.Name, &m.RouteGroup, &rawModels, &anchorText, &m.VectorDims, &m.Modality); err != nil {
			return nil, false, fmt.Errorf("scan semantic anchor: %w", err)
		}
		if err := json.Unmarshal(rawModels, &m.PreferredModels); err != nil {
			m.PreferredModels = []string{}
		}
		if m.PreferredModels == nil {
			m.PreferredModels = []string{}
		}
		if includeAnchorText {
			m.AnchorText = anchorText
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}
	if results == nil {
		results = []SemanticAnchorMeta{}
	}
	return results, hasMore, nil
}

// UpdateSemanticAnchor applies a partial update to an existing anchor.
// Only non-nil patch fields are written. Returns (true, nil) on success,
// (false, nil) if the anchor does not exist.
func (s *PostgresStorage) UpdateSemanticAnchor(
	ctx context.Context,
	tenantID, name string,
	patch SemanticAnchorPatch,
) (bool, error) {
	var setClauses []string
	// args[0] = tenantID ($1), args[1] = name ($2) — used in WHERE clause.
	args := []interface{}{tenantID, name}

	if patch.RouteGroup != nil {
		args = append(args, *patch.RouteGroup)
		setClauses = append(setClauses, fmt.Sprintf("route_group = $%d", len(args)))
	}
	if patch.PreferredModels != nil {
		modelsJSON, err := json.Marshal(*patch.PreferredModels)
		if err != nil {
			return false, fmt.Errorf("marshal preferred_models: %w", err)
		}
		args = append(args, modelsJSON)
		setClauses = append(setClauses, fmt.Sprintf("preferred_models = $%d", len(args)))
	}
	if patch.AnchorText != nil {
		args = append(args, *patch.AnchorText)
		setClauses = append(setClauses, fmt.Sprintf("anchor_text = $%d", len(args)))
	}
	if patch.Embedding != nil {
		args = append(args, formatVector(*patch.Embedding))
		setClauses = append(setClauses, fmt.Sprintf("embedding = $%d::vector", len(args)))
	}
	if patch.Modality != nil {
		args = append(args, *patch.Modality)
		setClauses = append(setClauses, fmt.Sprintf("modality = $%d", len(args)))
	}

	if len(setClauses) == 0 {
		// Nothing to update — treat as a no-op success.
		return true, nil
	}

	query := fmt.Sprintf(
		"UPDATE semantic_anchors SET %s WHERE tenant_id = $1 AND name = $2",
		strings.Join(setClauses, ", "))

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("update semantic anchor: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DeleteSemanticAnchor removes an anchor row.
// Returns (true, nil) on success, (false, nil) if not found.
func (s *PostgresStorage) DeleteSemanticAnchor(
	ctx context.Context,
	tenantID, name string,
) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM semantic_anchors WHERE tenant_id = $1 AND name = $2",
		tenantID, name)
	if err != nil {
		return false, fmt.Errorf("delete semantic anchor: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListSemanticAnchorsSorted returns all anchors for a tenant ordered by cosine distance
// (ascending) to the given embedding. Distance and vector dims are computed in SQL.
// Returns at most topK results.
func (s *PostgresStorage) ListSemanticAnchorsSorted(
	ctx context.Context,
	tenantID string,
	embedding []float64,
	topK int,
	modality string,
) ([]SemanticAnchorRow, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("embedding must not be empty")
	}

	vecStr := formatVector(embedding)

	const query = `
		SELECT name, route_group, preferred_models,
		       (embedding <=> $2::vector) AS distance,
		       vector_dims(embedding)     AS dims
		FROM semantic_anchors
		WHERE tenant_id = $1 AND modality = $4
		ORDER BY distance ASC
		LIMIT $3`

	rows, err := s.db.QueryContext(ctx, query, tenantID, vecStr, topK, modality)
	if err != nil {
		return nil, fmt.Errorf("query semantic anchors sorted: %w", err)
	}
	defer rows.Close()

	var results []SemanticAnchorRow
	for rows.Next() {
		var r SemanticAnchorRow
		var rawModels []byte
		if err := rows.Scan(&r.Name, &r.RouteGroup, &rawModels, &r.Distance, &r.VectorDims); err != nil {
			return nil, fmt.Errorf("scan semantic anchor: %w", err)
		}
		if err := json.Unmarshal(rawModels, &r.PreferredModels); err != nil {
			r.PreferredModels = []string{}
		}
		if r.PreferredModels == nil {
			r.PreferredModels = []string{}
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// formatVector converts a []float64 into the PostgreSQL vector literal format "[v1,v2,...,vn]".
func formatVector(v []float64) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(f, 'f', -1, 64))
	}
	sb.WriteByte(']')
	return sb.String()
}
