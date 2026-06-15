package router

import "context"

// SemanticAnchorResult holds the outcome of a nearest-anchor lookup.
type SemanticAnchorResult struct {
	Name            string
	RouteGroup      string
	PreferredModels []string
	Distance        float64
}

// SemanticLookupFunc queries the nearest semantic anchor for a given embedding.
// Returns (result, found, error). On error or not-found, semantic evaluation is skipped (fail open).
type SemanticLookupFunc func(ctx context.Context, tenantID string, embedding []float64) (SemanticAnchorResult, bool, error)
