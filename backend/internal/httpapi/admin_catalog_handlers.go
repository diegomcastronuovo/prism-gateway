package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/providers"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// catalogList is the OpenAI-style list envelope used by catalog endpoints.
type catalogList[T any] struct {
	Object string `json:"object"`
	Data   []T    `json:"data"`
}

type catalogTenantItem struct {
	TenantID string `json:"tenant_id"`
}

// catalogModelPricing is the API shape for model pricing (snake_case).
type catalogModelPricing struct {
	PromptPer1M     float64 `json:"prompt_per_1m,omitempty"`
	CompletionPer1M float64 `json:"completion_per_1m,omitempty"`
}

// catalogModelMock is the API shape for model mock config (snake_case).
type catalogModelMock struct {
	Enabled       bool    `json:"enabled"`
	DelayMinMs    int     `json:"delay_min_ms"`
	DelayMaxMs    int     `json:"delay_max_ms"`
	ErrorRate     float64 `json:"error_rate"`
	ErrorStatus   int     `json:"error_status"`
	ErrorMessage  string  `json:"error_message"`
	FixedResponse string  `json:"fixed_response"`
}

// catalogModelItem is the enriched model entry for GET /admin/models and GET /admin/models/{id}.
// Keeps id, provider, route_groups for backward compatibility; adds type, pricing, mock when present.
type catalogModelItem struct {
	ID                       string                     `json:"id"`
	Provider                 string                     `json:"provider"`
	RouteGroups              []string                   `json:"route_groups"`
	Type                     string                     `json:"type,omitempty"`
	Pricing                  *catalogModelPricing       `json:"pricing,omitempty"`
	Mock                     *catalogModelMock          `json:"mock,omitempty"`
	InfrastructureMonthlyUSD float64                    `json:"infrastructure_monthly_usd,omitempty"`
	MarkupPercentage         float64                    `json:"markup_percentage,omitempty"`
	Execution                *config.MLExecutionConfig  `json:"execution,omitempty"`
	Observable               *config.MLObservableConfig `json:"observable,omitempty"`
	BaseURL                  string                     `json:"base_url,omitempty"`
}

type catalogIDItem struct {
	ID string `json:"id"`
}

// providerRuntimeItem is the enriched per-provider entry returned by GET /admin/providers.
// It extends the legacy {id} shape with runtime metadata; no secrets are ever included.
type providerRuntimeItem struct {
	ID           string `json:"id"`
	Enabled      bool   `json:"enabled"`
	HasAPIKey    bool   `json:"has_api_key"`
	APIKeySource string `json:"api_key_source"` // "env" | "stored" | "missing"
	BaseURL      string `json:"base_url"`
	Type         string `json:"type"`
	Status       string `json:"status"` // "ready" | "missing_credentials" | "disabled"
}

// buildProviderRuntimeItem constructs the runtime metadata for a single provider.
// Credential detection order: env var → stored key → missing.
// The actual key value is never included in the returned struct.
func buildProviderRuntimeItem(name string, pc config.ProviderConfig, hasConfig bool) providerRuntimeItem {
	item := providerRuntimeItem{
		ID:      name,
		Enabled: true,
		Type:    name, // default: type = name
		BaseURL: pc.BaseURL,
	}
	if hasConfig {
		if pc.Type != "" {
			item.Type = pc.Type
		}
		if pc.Enabled != nil {
			item.Enabled = *pc.Enabled
		}
	}

	// Credential detection — never expose the actual key value.
	if pc.APIKeyEnv != "" {
		if os.Getenv(pc.APIKeyEnv) != "" {
			item.HasAPIKey = true
			item.APIKeySource = "env"
		}
	}
	if !item.HasAPIKey && pc.APIKey != "" {
		item.HasAPIKey = true
		item.APIKeySource = "stored"
	}
	if !item.HasAPIKey {
		item.APIKeySource = "missing"
	}

	// Derive status.
	switch {
	case !item.Enabled:
		item.Status = "disabled"
	case item.HasAPIKey:
		item.Status = "ready"
	default:
		item.Status = "missing_credentials"
	}
	return item
}

// loadAllTenantConfigs fetches all tenant configs from DB.
// Fail-open: tenants whose config cannot be loaded are returned with zero-value TenantConfig.
func (h *Handlers) loadAllTenantConfigs(ctx context.Context) []config.TenantConfig {
	if h.store == nil {
		return nil
	}
	ids, err := h.store.ListTenants(ctx)
	if err != nil || len(ids) == 0 {
		return nil
	}
	tenants := make([]config.TenantConfig, 0, len(ids))
	for _, id := range ids {
		cfgJSON, _, exists, err := h.store.GetTenantConfig(ctx, id)
		if err != nil || !exists {
			tenants = append(tenants, config.TenantConfig{ID: id})
			continue
		}
		var tc config.TenantConfig
		if err := json.Unmarshal(cfgJSON, &tc); err != nil {
			tenants = append(tenants, config.TenantConfig{ID: id})
			continue
		}
		tc.ID = id
		tenants = append(tenants, tc)
	}
	return tenants
}

// AdminListTenants returns all tenant IDs from DB (single source of truth per SPEC_147).
// Result is sorted alphabetically.
// When the request context has allowed_tenants (local_admin / user JWT) and the caller
// does not have full admin, the list is filtered to that allowlist only.
// GET /admin/tenants
func (h *Handlers) AdminListTenants(w http.ResponseWriter, r *http.Request) {
	ids, err := h.store.ListTenants(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to list tenants from DB", "error", err)
		ids = []string{} // fail-open: return empty list
	}
	sort.Strings(ids)

	// local_admin / user: only tenants in JWT allowed list (defense in depth vs middleware).
	allowed := auth.AllowedTenantsFromContext(r.Context())
	if len(allowed) > 0 && !auth.HasAnyRole(auth.RolesFromContext(r.Context()), adminBypassRoles) {
		allowedSet := make(map[string]struct{}, len(allowed))
		for _, a := range allowed {
			allowedSet[a] = struct{}{}
		}
		filtered := make([]string, 0, len(allowed))
		for _, id := range ids {
			if _, ok := allowedSet[id]; ok {
				filtered = append(filtered, id)
			}
		}
		ids = filtered
	}

	items := make([]catalogTenantItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, catalogTenantItem{TenantID: id})
	}
	writeJSON(w, http.StatusOK, catalogList[catalogTenantItem]{Object: "list", Data: items})
}

// buildCatalogModelItem converts a ModelConfig and its route groups into the API shape.
func buildCatalogModelItem(m config.ModelConfig, routeGroups []string) catalogModelItem {
	item := catalogModelItem{
		ID:          m.Name,
		Provider:    m.Provider,
		RouteGroups: routeGroups,
	}
	if m.Type != "" {
		item.Type = m.Type
	}
	if m.Pricing.PromptPer1M != 0 || m.Pricing.CompletionPer1M != 0 {
		item.Pricing = &catalogModelPricing{
			PromptPer1M:     m.Pricing.PromptPer1M,
			CompletionPer1M: m.Pricing.CompletionPer1M,
		}
	}
	// Always include mock block so the UI can show and edit it.
	item.Mock = &catalogModelMock{
		Enabled:       m.Mock.Enabled,
		DelayMinMs:    m.Mock.DelayMinMs,
		DelayMaxMs:    m.Mock.DelayMaxMs,
		ErrorRate:     m.Mock.ErrorRate,
		ErrorStatus:   m.Mock.ErrorStatus,
		ErrorMessage:  m.Mock.ErrorMessage,
		FixedResponse: m.Mock.FixedResponse,
	}
	if m.InfrastructureMonthlyUSD != 0 {
		item.InfrastructureMonthlyUSD = m.InfrastructureMonthlyUSD
	}
	if m.MarkupPercentage != 0 {
		item.MarkupPercentage = m.MarkupPercentage
	}
	if m.Execution != nil {
		item.Execution = m.Execution
	}
	if m.Observable != nil {
		item.Observable = m.Observable
	}
	if m.BaseURL != "" {
		item.BaseURL = m.BaseURL
	}
	return item
}

// AdminListModels returns all models configured in the gateway with full metadata:
// id, provider, route_groups, type (if set), pricing (if set), mock. Uses resolved global config.
// GET /admin/models
func (h *Handlers) AdminListModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gc := h.resolveGlobalConfig(ctx)

	// Build model → set-of-route-groups from DB tenants (SPEC_147).
	modelGroups := make(map[string]map[string]struct{})
	for _, t := range h.loadAllTenantConfigs(ctx) {
		for groupName, models := range t.Selection.RouteGroups {
			for _, m := range models {
				if modelGroups[m] == nil {
					modelGroups[m] = make(map[string]struct{})
				}
				modelGroups[m][groupName] = struct{}{}
			}
		}
	}

	items := make([]catalogModelItem, 0, len(gc.Models))
	for _, m := range gc.Models {
		groups := make([]string, 0, len(modelGroups[m.Name]))
		for g := range modelGroups[m.Name] {
			groups = append(groups, g)
		}
		sort.Strings(groups)
		items = append(items, buildCatalogModelItem(m, groups))
	}
	writeJSON(w, http.StatusOK, catalogList[catalogModelItem]{Object: "list", Data: items})
}

// AdminGetModel returns a single model by id with full metadata. GET /admin/models/{model_id}
func (h *Handlers) AdminGetModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelID := r.PathValue("model_id")
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "model_id is required", "invalid_request_error")
		return
	}

	gc := h.resolveGlobalConfig(ctx)
	m := gc.ModelByName(modelID)
	if m == nil {
		writeError(w, http.StatusNotFound, "model not found: "+modelID, "not_found")
		return
	}

	// Route groups for this model from DB tenants (SPEC_147).
	var groups []string
	seen := make(map[string]struct{})
	for _, t := range h.loadAllTenantConfigs(ctx) {
		for groupName, models := range t.Selection.RouteGroups {
			for _, name := range models {
				if name == modelID {
					if _, ok := seen[groupName]; !ok {
						seen[groupName] = struct{}{}
						groups = append(groups, groupName)
					}
					break
				}
			}
		}
	}
	sort.Strings(groups)

	writeJSON(w, http.StatusOK, buildCatalogModelItem(*m, groups))
}

// AdminListProviders returns runtime metadata for every configured provider.
// Primary source: cfg.Providers (enriched with credentials, base_url, type, status).
// Fallback: any provider referenced in cfg.Models but absent from cfg.Providers is
// included with minimal metadata (enabled=true, status="missing_credentials").
// Credentials are never exposed — only has_api_key and api_key_source are returned.
// GET /admin/providers
func (h *Handlers) AdminListProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	seen := make(map[string]struct{})
	for name := range h.cfg.Providers {
		seen[name] = struct{}{}
	}
	for _, m := range h.cfg.Models {
		if m.Provider != "" {
			seen[m.Provider] = struct{}{}
		}
	}
	if gc := h.resolveGlobalConfig(ctx); gc != nil && gc.Providers != nil {
		for name := range gc.Providers {
			seen[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]providerRuntimeItem, 0, len(names))
	for _, name := range names {
		pc := h.resolveProviderConfig(ctx, name)
		if pc == nil {
			items = append(items, buildProviderRuntimeItem(name, config.ProviderConfig{}, false))
		} else {
			items = append(items, buildProviderRuntimeItem(name, *pc, true))
		}
	}
	writeJSON(w, http.StatusOK, catalogList[providerRuntimeItem]{Object: "list", Data: items})
}

// AdminListRouteGroups returns all route group names from DB tenants (SPEC_147).
// GET /admin/route-groups
func (h *Handlers) AdminListRouteGroups(w http.ResponseWriter, r *http.Request) {
	tenants := h.loadAllTenantConfigs(r.Context())
	seen := make(map[string]struct{})
	for _, t := range tenants {
		for groupName := range t.Selection.RouteGroups {
			seen[groupName] = struct{}{}
		}
	}

	items := make([]catalogIDItem, 0, len(seen))
	for g := range seen {
		items = append(items, catalogIDItem{ID: g})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writeJSON(w, http.StatusOK, catalogList[catalogIDItem]{Object: "list", Data: items})
}

// AdminDeleteRouteGroup removes a route group from a tenant's selection configuration.
// DELETE /admin/tenants/{tenant_id}/route-groups/{name}
func (h *Handlers) AdminDeleteRouteGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.PathValue("tenant_id")
	groupName := r.PathValue("name")
	if groupName == "" {
		writeError(w, http.StatusBadRequest, "route group name is required", "invalid_request_error")
		return
	}

	// Ensure tenant config is in DB (seeds from YAML on first call).
	if err := h.ensureTenantConfigInDB(ctx, tenantID); err != nil {
		h.log.ErrorContext(ctx, "failed to ensure tenant config exists", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to prepare config", "internal_error")
		return
	}

	// Read current config to verify existence and capture the current version.
	configJSON, version, exists, err := h.store.GetTenantConfig(ctx, tenantID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch tenant config", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to fetch config", "internal_error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("tenant %q not found", tenantID), "not_found")
		return
	}

	var tc config.TenantConfig
	if err := json.Unmarshal(configJSON, &tc); err != nil {
		h.log.ErrorContext(ctx, "failed to unmarshal tenant config", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "tenant config is corrupt", "internal_error")
		return
	}
	if _, ok := tc.Selection.RouteGroups[groupName]; !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("route group %q not found", groupName), "not_found")
		return
	}

	// JSON Merge Patch (RFC 7396): setting a key to null removes it.
	patch, _ := json.Marshal(map[string]any{
		"selection": map[string]any{
			"route_groups": map[string]any{
				groupName: nil,
			},
		},
	})

	actorSub := auth.SubFromContext(r.Context())
	if actorSub == "" {
		actorSub = "admin-token"
	}
	actorRoles := auth.RolesFromContext(r.Context())
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"}
	}

	if _, err := h.store.PatchTenantConfig(ctx, tenantID, version, patch, actorSub, actorRoles); err != nil {
		if _, ok := err.(storage.ErrVersionConflict); ok {
			writeError(w, http.StatusConflict, err.Error(), "version_conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to delete route group", "error", err, "tenant_id", tenantID, "group", groupName)
		writeError(w, http.StatusInternalServerError, "failed to delete route group", "internal_error")
		return
	}

	if h.tenantCache != nil {
		h.tenantCache.Invalidate(tenantID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminListFeatures returns a map of platform feature flags derived from the
// active configuration, letting the UI conditionally render advanced panels.
// GET /admin/features
func (h *Handlers) AdminListFeatures(w http.ResponseWriter, r *http.Request) {
	semanticRouting := false
	semanticCache := false
	budgetEnforcement := false

	for _, t := range h.loadAllTenantConfigs(r.Context()) {
		if t.Routing.Strategy == "smart" {
			semanticRouting = true
		}
		if t.SemanticCache.Enabled {
			semanticCache = true
		}
		if t.BudgetEnforcement.Enabled {
			budgetEnforcement = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{
		"semantic_routing":   semanticRouting,
		"semantic_cache":     semanticCache,
		"budget_enforcement": budgetEnforcement,
		"dynamic_routes":     h.cfg.DynamicConfig.Enabled,
	})
}

// claudeCodeProviderItem is the response shape for GET /admin/providers/claude_code.
type claudeCodeProviderItem struct {
	ID           string `json:"id"`
	Enabled      bool   `json:"enabled"`
	HasAPIKey    bool   `json:"has_api_key"`
	APIKeySource string `json:"api_key_source"`
	BaseURL      string `json:"base_url"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	Licensed     bool   `json:"licensed"`
	Feature      string `json:"feature"`
}

// claudeCodeModelsResponse is the response shape for GET /admin/providers/claude_code/models.
type claudeCodeModelsResponse struct {
	Object   string                   `json:"object"`
	Provider string                   `json:"provider"`
	Data     []providers.ClaudeModelItem `json:"data"`
}

// claudeCodeAnthropicBaseURL returns the Anthropic base URL from the provider config,
// falling back to the canonical Anthropic API URL.
func (h *Handlers) claudeCodeAnthropicBaseURL(ctx context.Context) string {
	if pc, ok := h.cfg.Providers["anthropic"]; ok && pc.BaseURL != "" {
		return pc.BaseURL
	}
	if gc := h.resolveGlobalConfig(ctx); gc != nil {
		if pc, ok := gc.Providers["anthropic"]; ok && pc.BaseURL != "" {
			return pc.BaseURL
		}
	}
	return "https://api.anthropic.com"
}

// AdminGetClaudeCodeModels queries the Anthropic /v1/models endpoint and returns
// the normalized list of Claude models available for Claude Code usage.
// GET /admin/providers/claude_code/models
func (h *Handlers) AdminGetClaudeCodeModels(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	baseURL := h.claudeCodeAnthropicBaseURL(r.Context())
	items, err := providers.FetchClaudeModels(r.Context(), baseURL, apiKey)
	if err != nil {
		h.log.ErrorContext(r.Context(), "claude_code models: upstream fetch failed", "error", err.Error())
		writeError(w, http.StatusBadGateway, "failed to fetch Claude models from upstream: "+err.Error(), "upstream_error")
		return
	}

	writeJSON(w, http.StatusOK, claudeCodeModelsResponse{
		Object:   "list",
		Provider: "claude_code",
		Data:     items,
	})
}

// AdminGetClaudeCodeProvider returns the Claude Code capability status.
// GET /admin/providers/claude_code
func (h *Handlers) AdminGetClaudeCodeProvider(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, claudeCodeProviderItem{
		ID:           "claude_code",
		Enabled:      true,
		HasAPIKey:    false,
		APIKeySource: "license",
		BaseURL:      "",
		Type:         "claude_code",
		Status:       "ready",
		Licensed:     true,
		Feature:      "claude_code",
	})
}
