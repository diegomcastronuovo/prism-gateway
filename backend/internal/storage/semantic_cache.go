package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// FindNearestSemanticCache returns the nearest cache entry within the threshold for
// the given tenant and query vector. Scope controls whether matching is by model or route_group.
// Returns found=false (nil error) when no entry exceeds the threshold or the cache is empty.
func (s *PostgresStorage) FindNearestSemanticCache(
	ctx context.Context,
	tenantID string,
	queryVector []float64,
	scope SemanticCacheScope,
	model string,
	routeGroup string,
	threshold float64,
) (*SemanticCacheEntry, bool, error) {
	if len(queryVector) == 0 {
		return nil, false, nil
	}
	// Guard: a zero/negative threshold would set distanceThreshold=1.0 and match
	// everything. Treat it as "no cache" (misconfigured or unset in stored JSON).
	if threshold <= 0 {
		return nil, false, nil
	}

	vecStr := formatVector(queryVector)
	// pgvector cosine distance = 1 - similarity, so distance <= (1 - threshold)
	distanceThreshold := 1.0 - threshold

	var row *sql.Row
	if scope == SemanticCacheScopeRouteGroup {
		const q = `
			SELECT id, tenant_id, model, route_group, response_json,
			       1 - (embedding <=> $2::vector) AS similarity
			FROM semantic_cache
			WHERE tenant_id = $1
			  AND route_group = $3
			  AND expires_at > NOW()
			  AND (embedding <=> $2::vector) <= $4
			ORDER BY embedding <=> $2::vector
			LIMIT 1`
		row = s.db.QueryRowContext(ctx, q, tenantID, vecStr, routeGroup, distanceThreshold)
	} else {
		const q = `
			SELECT id, tenant_id, model, route_group, response_json,
			       1 - (embedding <=> $2::vector) AS similarity
			FROM semantic_cache
			WHERE tenant_id = $1
			  AND model = $3
			  AND expires_at > NOW()
			  AND (embedding <=> $2::vector) <= $4
			ORDER BY embedding <=> $2::vector
			LIMIT 1`
		row = s.db.QueryRowContext(ctx, q, tenantID, vecStr, model, distanceThreshold)
	}

	var entry SemanticCacheEntry
	err := row.Scan(&entry.ID, &entry.TenantID, &entry.Model, &entry.RouteGroup, &entry.ResponseJSON, &entry.Similarity)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("find nearest semantic cache: %w", err)
	}

	return &entry, true, nil
}

// InsertSemanticCacheEntry stores a new cache entry in the semantic_cache table.
func (s *PostgresStorage) InsertSemanticCacheEntry(ctx context.Context, entry SemanticCacheInsert) error {
	vecStr := formatVector(entry.Embedding)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO semantic_cache (tenant_id, model, route_group, embedding, request_text, response_json, expires_at)
		VALUES ($1, $2, $3, $4::vector, $5, $6, $7)
	`, entry.TenantID, entry.Model, entry.RouteGroup, vecStr, entry.RequestText, entry.ResponseJSON, entry.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert semantic cache entry: %w", err)
	}
	return nil
}

// TouchSemanticCacheHit updates last_hit_at and increments hit_count for the given cache entry.
func (s *PostgresStorage) TouchSemanticCacheHit(ctx context.Context, id uuid.UUID, ts time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE semantic_cache SET last_hit_at = $2, hit_count = hit_count + 1 WHERE id = $1`,
		id, ts)
	if err != nil {
		return fmt.Errorf("touch semantic cache hit: %w", err)
	}
	return nil
}

// PruneExpiredSemanticCache removes all expired entries for the given tenant.
func (s *PostgresStorage) PruneExpiredSemanticCache(ctx context.Context, tenantID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM semantic_cache WHERE tenant_id = $1 AND expires_at <= NOW()`,
		tenantID)
	if err != nil {
		return fmt.Errorf("prune expired semantic cache: %w", err)
	}
	return nil
}

// GetSemanticCacheStats returns aggregated semantic cache analytics for GET /admin/observability/semantic-cache.
// tenantID empty = all tenants; limit caps top lists (default 10, max 50).
func (s *PostgresStorage) GetSemanticCacheStats(ctx context.Context, tenantID string, limit int) (SemanticCacheStats, error) {
	tenantArg := interface{}(nil)
	if tenantID != "" {
		tenantArg = tenantID
	}
	args := []interface{}{tenantArg}

	// Summary: total_entries, total_hits, avg_hits_per_entry, active_entries, expired_entries
	var totalEntries, activeEntries, expiredEntries int
	var totalHits int64
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)::int,
			COALESCE(SUM(hit_count), 0)::bigint,
			COUNT(*) FILTER (WHERE expires_at IS NULL OR expires_at > NOW())::int,
			COUNT(*) FILTER (WHERE expires_at IS NOT NULL AND expires_at <= NOW())::int
		FROM semantic_cache
		WHERE $1::text IS NULL OR tenant_id = $1
	`, args...).Scan(&totalEntries, &totalHits, &activeEntries, &expiredEntries)
	if err != nil {
		return SemanticCacheStats{}, fmt.Errorf("semantic cache stats summary: %w", err)
	}
	avgHits := 0.0
	if totalEntries > 0 {
		avgHits = float64(totalHits) / float64(totalEntries)
	}
	summary := SemanticCacheStatsSummary{
		TotalEntries:    totalEntries,
		TotalHits:       totalHits,
		AvgHitsPerEntry: avgHits,
		ActiveEntries:   activeEntries,
		ExpiredEntries:  expiredEntries,
	}

	// Top prompts: hit_count DESC, last_hit_at DESC
	topPromptsArgs := append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT request_text, hit_count, last_hit_at, expires_at, model, COALESCE(route_group, '')
		FROM semantic_cache
		WHERE $1::text IS NULL OR tenant_id = $1
		ORDER BY hit_count DESC, last_hit_at DESC NULLS LAST
		LIMIT $2
	`, topPromptsArgs...)
	if err != nil {
		return SemanticCacheStats{}, fmt.Errorf("semantic cache top prompts: %w", err)
	}
	defer rows.Close()
	var topPrompts []SemanticCacheTopPrompt
	for rows.Next() {
		var p SemanticCacheTopPrompt
		var lastHitAt sql.NullTime
		if err := rows.Scan(&p.RequestText, &p.HitCount, &lastHitAt, &p.ExpiresAt, &p.Model, &p.RouteGroup); err != nil {
			return SemanticCacheStats{}, fmt.Errorf("scan top prompt: %w", err)
		}
		if lastHitAt.Valid {
			p.LastHitAt = &lastHitAt.Time
		}
		topPrompts = append(topPrompts, p)
	}
	if err := rows.Err(); err != nil {
		return SemanticCacheStats{}, err
	}

	// Top models: total_hits DESC, entries DESC
	modelRows, err := s.db.QueryContext(ctx, `
		SELECT model, COUNT(*)::int, COALESCE(SUM(hit_count), 0)::bigint
		FROM semantic_cache
		WHERE $1::text IS NULL OR tenant_id = $1
		GROUP BY model
		ORDER BY SUM(hit_count) DESC, COUNT(*) DESC
		LIMIT $2
	`, topPromptsArgs...)
	if err != nil {
		return SemanticCacheStats{}, fmt.Errorf("semantic cache top models: %w", err)
	}
	defer modelRows.Close()
	var topModels []SemanticCacheTopModel
	for modelRows.Next() {
		var m SemanticCacheTopModel
		if err := modelRows.Scan(&m.Model, &m.Entries, &m.TotalHits); err != nil {
			return SemanticCacheStats{}, fmt.Errorf("scan top model: %w", err)
		}
		topModels = append(topModels, m)
	}
	if err := modelRows.Err(); err != nil {
		return SemanticCacheStats{}, err
	}

	// Top route groups: COALESCE(route_group, '') for consistency; total_hits DESC, entries DESC
	rgRows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(route_group, '') AS route_group, COUNT(*)::int, COALESCE(SUM(hit_count), 0)::bigint
		FROM semantic_cache
		WHERE $1::text IS NULL OR tenant_id = $1
		GROUP BY COALESCE(route_group, '')
		ORDER BY SUM(hit_count) DESC, COUNT(*) DESC
		LIMIT $2
	`, topPromptsArgs...)
	if err != nil {
		return SemanticCacheStats{}, fmt.Errorf("semantic cache top route groups: %w", err)
	}
	defer rgRows.Close()
	var topRouteGroups []SemanticCacheTopRouteGroup
	for rgRows.Next() {
		var r SemanticCacheTopRouteGroup
		if err := rgRows.Scan(&r.RouteGroup, &r.Entries, &r.TotalHits); err != nil {
			return SemanticCacheStats{}, fmt.Errorf("scan top route group: %w", err)
		}
		topRouteGroups = append(topRouteGroups, r)
	}
	if err := rgRows.Err(); err != nil {
		return SemanticCacheStats{}, err
	}

	// Ensure non-nil slices for empty response
	if topPrompts == nil {
		topPrompts = []SemanticCacheTopPrompt{}
	}
	if topModels == nil {
		topModels = []SemanticCacheTopModel{}
	}
	if topRouteGroups == nil {
		topRouteGroups = []SemanticCacheTopRouteGroup{}
	}

	return SemanticCacheStats{
		Summary:        summary,
		TopPrompts:     topPrompts,
		TopModels:      topModels,
		TopRouteGroups: topRouteGroups,
		Expiration:     SemanticCacheExpiration{Active: activeEntries, Expired: expiredEntries},
	}, nil
}

// GetCacheSavings estimates cost savings from semantic cache: hit_count * avg cost per request (from usage).
// tenantID optional; usage avg is over last 30 days.
func (s *PostgresStorage) GetCacheSavings(ctx context.Context, tenantID string) (CacheSavings, error) {
	const usageWindowDays = 30
	tenantArg := interface{}(nil)
	if tenantID != "" {
		tenantArg = tenantID
	}

	// Cache: model -> total hit_count
	cacheRows, err := s.db.QueryContext(ctx, `
		SELECT model, COALESCE(SUM(hit_count), 0)::bigint AS hits
		FROM semantic_cache
		WHERE $1::text IS NULL OR tenant_id = $1
		GROUP BY model
	`, tenantArg)
	if err != nil {
		return CacheSavings{}, fmt.Errorf("cache savings cache hits: %w", err)
	}
	defer cacheRows.Close()
	cacheHits := make(map[string]int64)
	var totalHits int64
	for cacheRows.Next() {
		var model string
		var hits int64
		if err := cacheRows.Scan(&model, &hits); err != nil {
			return CacheSavings{}, fmt.Errorf("scan cache hits: %w", err)
		}
		cacheHits[model] = hits
		totalHits += hits
	}
	if err := cacheRows.Err(); err != nil {
		return CacheSavings{}, err
	}

	// Usage: model -> avg cost per request (last 30 days)
	usageStart := time.Now().UTC().AddDate(0, 0, -usageWindowDays)
	usageRows, err := s.db.QueryContext(ctx, `
		SELECT model, AVG(cost_usd) AS avg_cost
		FROM usage
		WHERE ts >= $1 AND ($2::text IS NULL OR tenant_id = $2)
		GROUP BY model
	`, usageStart, tenantArg)
	if err != nil {
		return CacheSavings{}, fmt.Errorf("cache savings usage avg: %w", err)
	}
	defer usageRows.Close()
	avgCostByModel := make(map[string]float64)
	for usageRows.Next() {
		var model string
		var avgCost sql.NullFloat64
		if err := usageRows.Scan(&model, &avgCost); err != nil {
			return CacheSavings{}, fmt.Errorf("scan usage avg: %w", err)
		}
		if avgCost.Valid {
			avgCostByModel[model] = avgCost.Float64
		}
	}
	if err := usageRows.Err(); err != nil {
		return CacheSavings{}, err
	}

	// saved_usd per model = hits * avg_cost; total saved = sum
	var totalSaved float64
	var byModel []CacheSavingsByModel
	for model, hits := range cacheHits {
		avgCost := avgCostByModel[model]
		saved := float64(hits) * avgCost
		totalSaved += saved
		byModel = append(byModel, CacheSavingsByModel{Model: model, SavedUSD: saved})
	}
	sort.Slice(byModel, func(i, j int) bool { return byModel[i].SavedUSD > byModel[j].SavedUSD })

	avgCostPerRequest := 0.0
	if totalHits > 0 {
		avgCostPerRequest = totalSaved / float64(totalHits)
	}
	if byModel == nil {
		byModel = []CacheSavingsByModel{}
	}

	return CacheSavings{
		Summary: CacheSavingsSummary{
			TotalHits:             totalHits,
			EstimatedCostSavedUSD: totalSaved,
			AvgCostPerRequest:     avgCostPerRequest,
		},
		ByModel: byModel,
	}, nil
}
