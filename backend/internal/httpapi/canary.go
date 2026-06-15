package httpapi

import (
	"math/rand"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// weightedSelectModel picks one model from entries using weighted random selection.
// Returns ("", false) if entries are empty, contain non-positive weights, or empty model names.
func weightedSelectModel(entries []config.TrafficSplitEntry) (string, bool) {
	if len(entries) == 0 {
		return "", false
	}
	total := 0
	for _, e := range entries {
		if e.Model == "" || e.Weight <= 0 {
			return "", false
		}
		total += e.Weight
	}
	if total <= 0 {
		return "", false
	}

	// Uniform random in [1, total].
	r := rand.Intn(total) + 1
	cumulative := 0
	for _, e := range entries {
		cumulative += e.Weight
		if r <= cumulative {
			return e.Model, true
		}
	}
	// Unreachable, but guard against floating-point or integer edge cases.
	return entries[len(entries)-1].Model, true
}
