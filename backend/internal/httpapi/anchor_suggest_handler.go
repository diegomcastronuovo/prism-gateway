package httpapi

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
)

// ── Request / Response types ─────────────────────────────────────────────────

type suggestRequest struct {
	Dataset     []string `json:"dataset"`
	MaxClusters *int     `json:"max_clusters,omitempty"` // pointer: nil = not provided
}

type suggestAnchorItem struct {
	Anchor   string `json:"anchor"`
	Examples int    `json:"examples"`
}

type suggestResponse struct {
	Anchors []suggestAnchorItem `json:"anchors"`
}

// internal — never serialised
type embeddedItem struct {
	text      string
	embedding []float64
}

type clusterResult struct {
	centroid []float64
	members  []embeddedItem
}

// ── Handler ──────────────────────────────────────────────────────────────────

// SuggestSemanticAnchors handles POST /admin/semantic/anchors/suggest.
//
// Given a list of raw prompt strings, generates embeddings, clusters them with
// k-means, and returns one suggested anchor per cluster (the member nearest to
// the centroid). Suggested anchors are NOT saved.
func (h *Handlers) SuggestSemanticAnchors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Resolve tenant
	tenant, err := h.resolveTenantForAdminSemantic(ctx, r)
	if err != nil {
		h.writeSemanticTenantResolveError(w, ctx, err)
		return
	}

	// 2. Parse request
	var req suggestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error")
		return
	}

	// 3. Validate dataset
	if len(req.Dataset) == 0 {
		writeError(w, http.StatusBadRequest, "dataset must not be empty", "invalid_request_error")
		return
	}
	if len(req.Dataset) > 500 {
		writeError(w, http.StatusBadRequest, "dataset exceeds maximum of 500 items", "invalid_request_error")
		return
	}
	for i, item := range req.Dataset {
		if item == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("dataset[%d] must not be empty", i), "invalid_request_error")
			return
		}
	}

	// 4. Validate + resolve max_clusters
	maxClusters := 10 // default
	if req.MaxClusters != nil {
		if *req.MaxClusters < 0 {
			writeError(w, http.StatusBadRequest, "max_clusters must not be negative", "invalid_request_error")
			return
		}
		if *req.MaxClusters > 50 {
			writeError(w, http.StatusBadRequest, "max_clusters must not exceed 50", "invalid_request_error")
			return
		}
		if *req.MaxClusters > 0 {
			maxClusters = *req.MaxClusters
		}
		// 0 → use default (10)
	}
	// Clamp k to dataset size
	k := maxClusters
	if k > len(req.Dataset) {
		k = len(req.Dataset)
	}

	// 5. Resolve embedding provider — uses tenant routing.semantic.embedding_model (SPEC_103).
	embModel, embErr := h.embeddingModelForModality(ctx, tenant, "text")
	if embModel == nil {
		msg := "no embedding model configured"
		if embErr != nil {
			msg = embErr.Error()
		}
		writeError(w, http.StatusBadRequest, msg, "invalid_request_error")
		return
	}

	var ep providers.EmbeddingProvider
	if embModel.Mock.Enabled {
		ep = providers.NewMockProvider(embModel.Mock, embModel.Name, tenant.ID, embModel.Pricing, nil)
	} else {
		var ok bool
		ep, ok = h.embeddingProviderFor(ctx, embModel.Provider)
		if !ok {
			writeError(w, http.StatusInternalServerError, "no embedding model configured", "internal_error")
			return
		}
	}

	// 6. Generate embeddings for each dataset item
	items := make([]embeddedItem, 0, len(req.Dataset))
	for _, item := range req.Dataset {
		embResp, err := ep.CreateEmbedding(ctx, providers.EmbeddingRequest{
			Input: []string{item},
			Model: embModel.Name,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "failed to generate embedding: "+err.Error(), "upstream_error")
			return
		}
		if len(embResp.Data) == 0 {
			writeError(w, http.StatusBadGateway, "no embedding data returned", "upstream_error")
			return
		}
		items = append(items, embeddedItem{
			text:      item,
			embedding: embResp.Data[0].Embedding,
		})
	}

	// 7. Cluster
	clusters := kMeansCluster(items, k)

	// 8. Build response, skip empty clusters
	anchors := make([]suggestAnchorItem, 0, len(clusters))
	for _, c := range clusters {
		if len(c.members) == 0 {
			continue
		}
		anchors = append(anchors, suggestAnchorItem{
			Anchor:   nearestToCentroid(c.centroid, c.members),
			Examples: len(c.members),
		})
	}

	// 9. Log and respond
	nonEmptyClusters := len(anchors)
	h.log.InfoContext(ctx, "semantic anchor suggestion complete",
		"tenant_id", tenant.ID,
		"dataset_size", len(req.Dataset),
		"k", k,
		"non_empty_clusters", nonEmptyClusters,
		"embedding_model", embModel.Name,
	)

	writeJSON(w, http.StatusOK, suggestResponse{Anchors: anchors})
}

// ── Math helpers ─────────────────────────────────────────────────────────────

// euclidSq returns the squared Euclidean distance between a and b.
// No sqrt needed for argmin comparisons.
func euclidSq(a, b []float64) float64 {
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return sum
}

// computeCentroid returns the element-wise mean of items' embeddings.
func computeCentroid(items []embeddedItem) []float64 {
	if len(items) == 0 {
		return nil
	}
	dims := len(items[0].embedding)
	centroid := make([]float64, dims)
	for _, it := range items {
		for j, v := range it.embedding {
			centroid[j] += v
		}
	}
	n := float64(len(items))
	for j := range centroid {
		centroid[j] /= n
	}
	return centroid
}

// nearestToCentroid returns the text of the member with minimum euclidSq to centroid.
func nearestToCentroid(centroid []float64, members []embeddedItem) string {
	best := 0
	bestDist := euclidSq(centroid, members[0].embedding)
	for i := 1; i < len(members); i++ {
		d := euclidSq(centroid, members[i].embedding)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return members[best].text
}

// kMeansCluster runs k-means++ initialisation followed by Lloyd's algorithm
// (max 100 iterations). Uses a fixed seed (42) for determinism.
// Empty clusters remain in the result slice (members==nil); callers filter them.
func kMeansCluster(items []embeddedItem, k int) []clusterResult {
	if len(items) == 0 || k <= 0 {
		return nil
	}

	n := len(items)
	rng := rand.New(rand.NewSource(42)) //nolint:gosec

	// ── k-means++ initialisation ──────────────────────────────────────────────
	centroids := make([][]float64, 0, k)

	// Pick first centroid uniformly at random
	first := make([]float64, len(items[rng.Intn(n)].embedding))
	copy(first, items[rng.Intn(n)].embedding)
	centroids = append(centroids, first)

	for len(centroids) < k {
		dists := make([]float64, n)
		total := 0.0
		for i, it := range items {
			minD := euclidSq(it.embedding, centroids[0])
			for _, c := range centroids[1:] {
				if d := euclidSq(it.embedding, c); d < minD {
					minD = d
				}
			}
			dists[i] = minD
			total += minD
		}

		var chosen []float64
		if total == 0 {
			// Degenerate: all identical embeddings — pick items[0]
			chosen = make([]float64, len(items[0].embedding))
			copy(chosen, items[0].embedding)
		} else {
			threshold := rng.Float64() * total
			cumulative := 0.0
			idx := 0
			for i, d := range dists {
				cumulative += d
				if cumulative >= threshold {
					idx = i
					break
				}
			}
			chosen = make([]float64, len(items[idx].embedding))
			copy(chosen, items[idx].embedding)
		}
		centroids = append(centroids, chosen)
	}

	// ── Lloyd's iterations ────────────────────────────────────────────────────
	assignments := make([]int, n)

	for iter := 0; iter < 100; iter++ {
		changed := false

		// Assign each item to nearest centroid
		for i, it := range items {
			best := 0
			bestDist := euclidSq(it.embedding, centroids[0])
			for c := 1; c < len(centroids); c++ {
				if d := euclidSq(it.embedding, centroids[c]); d < bestDist {
					bestDist = d
					best = c
				}
			}
			if assignments[i] != best {
				assignments[i] = best
				changed = true
			}
		}

		if !changed {
			break
		}

		// Recompute centroids as mean of members; empty cluster keeps prior centroid
		sums := make([][]float64, k)
		counts := make([]int, k)
		dims := len(items[0].embedding)
		for c := range sums {
			sums[c] = make([]float64, dims)
		}
		for i, it := range items {
			c := assignments[i]
			counts[c]++
			for j, v := range it.embedding {
				sums[c][j] += v
			}
		}
		for c := range centroids {
			if counts[c] > 0 {
				for j := range centroids[c] {
					centroids[c][j] = sums[c][j] / float64(counts[c])
				}
			}
			// empty cluster: keep prior centroid unchanged
		}
	}

	// ── Build results ─────────────────────────────────────────────────────────
	// Collect members per cluster
	memberLists := make([][]embeddedItem, k)
	for i, it := range items {
		c := assignments[i]
		memberLists[c] = append(memberLists[c], it)
	}

	results := make([]clusterResult, k)
	for c := range results {
		members := memberLists[c]
		if len(members) == 0 {
			results[c] = clusterResult{centroid: centroids[c], members: nil}
			continue
		}
		// Recompute centroid from actual members for consistent nearestToCentroid
		results[c] = clusterResult{
			centroid: computeCentroid(members),
			members:  members,
		}
	}

	return results
}
