package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
)

// claudeCodeFamilies is the fixed set of supported Claude model families.
var claudeCodeFamilies = []string{"haiku", "sonnet", "opus"}

// codingModelItem is one entry in the GET/PUT request and response for coding model pricing.
type codingModelItem struct {
	Family      string  `json:"family"`
	InputPrice  float64 `json:"input_price"`
	OutputPrice float64 `json:"output_price"`
}

// claudeCodeLicenseGate always returns true in the community edition (no license gating).
func (h *Handlers) claudeCodeLicenseGate(_ http.ResponseWriter) bool {
	return true
}

// AdminGetClaudeCodeCodingModels returns the pricing for the 3 Claude model families.
// Always returns all 3 families; defaults to 0 when unconfigured.
// GET /admin/providers/claude_code/coding_models
func (h *Handlers) AdminGetClaudeCodeCodingModels(w http.ResponseWriter, r *http.Request) {
	if !h.claudeCodeLicenseGate(w) {
		return
	}

	pricing := h.readClaudeCodePricing(r)
	writeJSON(w, http.StatusOK, buildCodingModelItems(pricing))
}

// AdminPutClaudeCodeCodingModels updates pricing for one or more Claude model families.
// Only the provided families are updated; others are preserved.
// PUT /admin/providers/claude_code/coding_models
func (h *Handlers) AdminPutClaudeCodeCodingModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !h.claudeCodeLicenseGate(w) {
		return
	}

	var updates []codingModelItem
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}

	// Validate all entries before touching storage.
	validFamilies := map[string]bool{"haiku": true, "sonnet": true, "opus": true}
	for _, u := range updates {
		if !validFamilies[u.Family] {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("invalid family %q: must be one of haiku, sonnet, opus", u.Family),
				"validation_error")
			return
		}
		if u.InputPrice < 0 || u.OutputPrice < 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("prices must be >= 0 (family=%q)", u.Family),
				"validation_error")
			return
		}
	}

	// Fetch current global config.
	currentJSON, currentVersion, exists, err := h.store.GetGlobalConfig(ctx)
	if err != nil {
		h.log.ErrorContext(ctx, "claude_code pricing: failed to read global config", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read config", "internal_error")
		return
	}

	var gc config.GlobalConfig
	if exists {
		if err := json.Unmarshal(currentJSON, &gc); err != nil {
			h.log.ErrorContext(ctx, "claude_code pricing: failed to parse global config", "error", err)
			writeError(w, http.StatusInternalServerError, "global config is corrupt", "internal_error")
			return
		}
	} else {
		gc = *config.GlobalConfigFromYAML(h.cfg)
		currentVersion = 0
	}

	// Apply updates (merge — do not delete unconfigured families).
	if gc.ClaudeCodePricing == nil {
		gc.ClaudeCodePricing = make(map[string]config.ClaudeCodeFamilyPricing)
	}
	for _, u := range updates {
		gc.ClaudeCodePricing[u.Family] = config.ClaudeCodeFamilyPricing{
			Input:  u.InputPrice,
			Output: u.OutputPrice,
		}
	}

	newConfigJSON, _ := json.Marshal(gc)
	actorSub, actorRoles := extractActor(r)

	if _, err := h.store.PutGlobalConfig(ctx, currentVersion, newConfigJSON, actorSub, actorRoles); err != nil {
		h.log.ErrorContext(ctx, "claude_code pricing: failed to persist config", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to persist config", "internal_error")
		return
	}

	// Invalidate global config cache so next request sees the new pricing.
	if h.globalCfgCache != nil {
		h.globalCfgCache.Invalidate()
	}

	writeJSON(w, http.StatusOK, buildCodingModelItems(gc.ClaudeCodePricing))
}

// readClaudeCodePricing returns the current claude_code pricing from the global config,
// falling back to YAML-derived config when no DB config exists.
func (h *Handlers) readClaudeCodePricing(r *http.Request) map[string]config.ClaudeCodeFamilyPricing {
	ctx := r.Context()
	currentJSON, _, exists, err := h.store.GetGlobalConfig(ctx)
	if err != nil || !exists {
		return nil
	}
	var gc config.GlobalConfig
	if err := json.Unmarshal(currentJSON, &gc); err != nil {
		return nil
	}
	return gc.ClaudeCodePricing
}

// buildCodingModelItems constructs the response array from a pricing map,
// ensuring all 3 families are always present with 0 as default.
func buildCodingModelItems(pricing map[string]config.ClaudeCodeFamilyPricing) []codingModelItem {
	items := make([]codingModelItem, 0, len(claudeCodeFamilies))
	for _, f := range claudeCodeFamilies {
		p := pricing[f] // zero-value when missing
		items = append(items, codingModelItem{
			Family:      f,
			InputPrice:  p.Input,
			OutputPrice: p.Output,
		})
	}
	return items
}
