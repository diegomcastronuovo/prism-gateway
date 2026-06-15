package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// AdminGetTenantConfig retrieves the current configuration for a tenant
// GET /admin/tenants/{tenant_id}/config
func (h *Handlers) AdminGetTenantConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	configJSON, version, exists, err := h.store.GetTenantConfig(r.Context(), tenantID)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to fetch tenant config", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to fetch config", "internal_error")
		return
	}
	if !exists {
		// SPEC_147: DB is the single source of truth. All tenants are seeded at startup.
		writeError(w, http.StatusNotFound, "tenant not found", "not_found")
		return
	}

	var configMap map[string]interface{}
	json.Unmarshal(configJSON, &configMap)

	// SPEC_146: guarantee config.environment is always set and normalized (DB value or "DEV").
	env, _ := configMap["environment"].(string)
	if env == "" {
		env = "DEV"
	}
	configMap["environment"] = strings.ToUpper(env)

	// Redact webhook secrets (api_key, auth.token) before returning to caller.
	if hooks, ok := configMap["hooks"].(map[string]interface{}); ok {
		deleteNestedSecrets(hooks)
	}

	response := map[string]interface{}{
		"tenant_id": tenantID,
		"version":   version,
		"config":    configMap,
	}

	writeJSON(w, http.StatusOK, response)
}

// AdminPutTenantConfig replaces the entire tenant configuration
// PUT /admin/tenants/{tenant_id}/config
// Requires: If-Match-Version header
func (h *Handlers) AdminPutTenantConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := r.PathValue("tenant_id")

	// Check If-Match-Version header
	ifMatchVersionStr := r.Header.Get("If-Match-Version")
	if ifMatchVersionStr == "" {
		writeError(w, 428, "missing If-Match-Version header", "precondition_required")
		return
	}

	ifMatchVersion, err := strconv.Atoi(ifMatchVersionStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid If-Match-Version", "invalid_request_error")
		return
	}

	// Read body
	var newConfig config.TenantConfig
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", "invalid_request_error")
		return
	}

	// Validate
	if err := validateTenantConfig(&newConfig, h.cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}

	// Fetch current config for environment immutability check and compliance diff.
	beforeConfigJSON, _, _, _ := h.store.GetTenantConfig(ctx, tenantID)

	// Validate environment format if provided.
	if newConfig.Environment != "" {
		if err := validateTenantEnvironment(newConfig.Environment); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
			return
		}
		// Reject if it differs from the current stored environment (immutable).
		if curEnv := currentEnvironmentFromJSON(beforeConfigJSON); curEnv != "" && curEnv != newConfig.Environment {
			writeError(w, http.StatusBadRequest, "environment is immutable", "invalid_request_error")
			return
		}
	}

	// Prepare JSON (excluding API keys - they stay in YAML)
	configForDB := stripAPIKeys(newConfig)
	newConfigJSON, _ := json.Marshal(configForDB)

	// Extract actor from context
	actorSub := auth.SubFromContext(r.Context())
	if actorSub == "" {
		actorSub = "admin-token" // Fallback for X-Admin-Token auth
	}
	actorRoles := auth.RolesFromContext(r.Context())
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"} // Prevent NULL constraint violation
	}

	// Ensure tenant config exists in DB (seed from YAML if needed)
	if err := h.ensureTenantConfigInDB(ctx, tenantID); err != nil {
		h.log.ErrorContext(ctx, "failed to ensure tenant config exists", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to prepare config", "internal_error")
		return
	}

	summary := "Full config update via PUT"
	diffJSON := json.RawMessage(`{}`) // TODO: compute diff from old config

	// Save to DB
	newVersion, err := h.store.PutTenantConfig(
		ctx,
		tenantID,
		ifMatchVersion,
		newConfigJSON,
		actorSub,
		actorRoles,
		summary,
		diffJSON,
	)

	if err != nil {
		if _, ok := err.(storage.ErrVersionConflict); ok {
			writeError(w, 409, err.Error(), "version_conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to update tenant config", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to update config", "internal_error")
		return
	}

	// Invalidate cache
	if h.tenantCache != nil {
		h.tenantCache.Invalidate(tenantID)
	}

	response := map[string]interface{}{
		"tenant_id": tenantID,
		"version":   newVersion,
		"message":   "Configuration updated successfully",
	}

	writeJSON(w, http.StatusOK, response)
}

// AdminPatchTenantConfig partially updates tenant configuration
// PATCH /admin/tenants/{tenant_id}/config
// Requires: If-Match-Version header
// Body: JSON Merge Patch (RFC 7396)
func (h *Handlers) AdminPatchTenantConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	// Check If-Match-Version header
	ifMatchVersionStr := r.Header.Get("If-Match-Version")
	if ifMatchVersionStr == "" {
		writeError(w, 428, "missing If-Match-Version header", "precondition_required")
		return
	}

	ifMatchVersion, err := strconv.Atoi(ifMatchVersionStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid If-Match-Version", "invalid_request_error")
		return
	}

	// Read merge patch JSON
	var mergePatch json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&mergePatch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", "invalid_request_error")
		return
	}

	// Reject any attempt to change environment (immutable after creation).
	if patchEnv := currentEnvironmentFromJSON(mergePatch); patchEnv != "" {
		writeError(w, http.StatusBadRequest, "environment is immutable", "invalid_request_error")
		return
	}

	// Extract actor
	actorSub := auth.SubFromContext(r.Context())
	if actorSub == "" {
		actorSub = "admin-token"
	}
	actorRoles := auth.RolesFromContext(r.Context())
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"} // Prevent NULL constraint violation
	}

	// Ensure tenant config exists in DB (seed from YAML if needed)
	if err := h.ensureTenantConfigInDB(r.Context(), tenantID); err != nil {
		h.log.ErrorContext(r.Context(), "failed to ensure tenant config exists", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to prepare config", "internal_error")
		return
	}

	// Apply patch
	newVersion, err := h.store.PatchTenantConfig(
		r.Context(),
		tenantID,
		ifMatchVersion,
		mergePatch,
		actorSub,
		actorRoles,
	)

	if err != nil {
		if _, ok := err.(storage.ErrVersionConflict); ok {
			writeError(w, 409, err.Error(), "version_conflict_error")
			return
		}
		h.log.ErrorContext(r.Context(), "failed to patch tenant config", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to patch config", "internal_error")
		return
	}

	// Invalidate cache
	if h.tenantCache != nil {
		h.tenantCache.Invalidate(tenantID)
	}

	response := map[string]interface{}{
		"tenant_id": tenantID,
		"version":   newVersion,
		"message":   "Configuration patched successfully",
	}

	writeJSON(w, http.StatusOK, response)
}

// AdminListConfigChanges retrieves configuration change history
// GET /admin/tenants/{tenant_id}/config/changes?limit=50
func (h *Handlers) AdminListConfigChanges(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant_id")

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	changes, err := h.store.ListTenantConfigChanges(r.Context(), tenantID, limit)
	if err != nil {
		h.log.ErrorContext(r.Context(), "failed to fetch config changes", "error", err, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to fetch changes", "internal_error")
		return
	}

	response := map[string]interface{}{
		"tenant_id": tenantID,
		"changes":   changes,
	}

	writeJSON(w, http.StatusOK, response)
}

// AdminConfigHistory returns normalized config change history (global + tenant) for GET /admin/config/history.
// Roles without full admin (and with JWT roles present) only receive tenant-scoped history for
// allowed tenants; global rows are never included. API keys and X-Admin-Token keep full access when roles are empty.
func (h *Handlers) AdminConfigHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	scope := q.Get("scope")
	tenantID := q.Get("tenant_id")
	limit := 50
	if s := q.Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
			if limit > 200 {
				limit = 200
			}
		}
	}
	offset := 0
	if s := q.Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}

	filter := storage.ConfigHistoryFilter{Scope: scope, TenantID: tenantID, Limit: limit, Offset: offset}
	roles := auth.RolesFromContext(ctx)
	fullAccess := len(roles) == 0 || auth.HasAnyRole(roles, adminBypassRoles)
	if !fullAccess {
		if scope == "global" {
			writeError(w, http.StatusBadRequest, "global scope is not available for your role", "invalid_request_error")
			return
		}
		allowed := auth.AllowedTenantsFromContext(ctx)
		if len(allowed) == 0 {
			writeError(w, http.StatusForbidden, "tenant assignment required for config history", "authorization_error")
			return
		}
		if tenantID != "" && !auth.TenantInRequestAllowed(tenantID, allowed) {
			writeError(w, http.StatusForbidden, "access denied: tenant not in allowed list", "authorization_error")
			return
		}
		filter.ExcludeGlobal = true
		filter.AllowedTenantIDs = allowed
	}

	rows, err := h.store.ListConfigHistory(ctx, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list config history", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch config history", "internal_error")
		return
	}

	data := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]interface{}{
			"scope":        row.Scope,
			"tenant_id":    row.TenantID,
			"changed_at":   row.ChangedAt.Format(time.RFC3339),
			"changed_by":   row.ChangedBy,
			"change_type":  row.ChangeType,
			"from_version": row.FromVersion,
			"to_version":   row.ToVersion,
			"is_rollback":  row.IsRollback,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object": "config_history",
		"data":   data,
		"pagination": map[string]interface{}{
			"limit":    limit,
			"offset":   offset,
			"returned": len(data),
		},
	})
}

// AdminConfigVersions returns version timeline for GET /admin/config/versions.
func (h *Handlers) AdminConfigVersions(w http.ResponseWriter, r *http.Request) {
	if !requireGlobalConfigAdminRole(w, r) {
		return
	}
	ctx := r.Context()
	q := r.URL.Query()
	scope := q.Get("scope")
	tenantID := q.Get("tenant_id")

	if scope != "global" && scope != "tenant" {
		writeError(w, http.StatusBadRequest, "invalid or missing scope (required: global or tenant)", "invalid_request_error")
		return
	}
	if scope == "tenant" && tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required when scope=tenant", "invalid_request_error")
		return
	}

	limit := 50
	if s := q.Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
			if limit > 200 {
				limit = 200
			}
		}
	}
	offset := 0
	if s := q.Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}

	// Current version from same source as config APIs
	var currentVersion int
	if scope == "global" {
		_, ver, _, err := h.store.GetGlobalConfig(ctx)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to get global config for versions", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get current version", "internal_error")
			return
		}
		currentVersion = ver
	} else {
		_, ver, exists, err := h.store.GetTenantConfig(ctx, tenantID)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to get tenant config for versions", "error", err, "tenant_id", tenantID)
			writeError(w, http.StatusInternalServerError, "failed to get current version", "internal_error")
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "tenant not found", "not_found")
			return
		}
		currentVersion = ver
	}

	filter := storage.ConfigVersionFilter{Scope: scope, TenantID: tenantID, Limit: limit, Offset: offset}
	rows, err := h.store.ListConfigVersions(ctx, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list config versions", "error", err, "scope", scope, "tenant_id", tenantID)
		writeError(w, http.StatusInternalServerError, "failed to fetch config versions", "internal_error")
		return
	}

	// Spec: 404 if tenant scope and no version history
	if scope == "tenant" && len(rows) == 0 {
		writeError(w, http.StatusNotFound, "tenant has no version history", "not_found")
		return
	}

	data := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]interface{}{
			"version":    row.Version,
			"created_at": row.CreatedAt.Format(time.RFC3339),
			"is_current": row.Version == currentVersion,
		})
	}

	respTenantID := ""
	if scope == "tenant" {
		respTenantID = tenantID
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object":          "config_versions",
		"scope":           scope,
		"tenant_id":       respTenantID,
		"current_version": currentVersion,
		"data":            data,
		"pagination": map[string]interface{}{
			"limit":    limit,
			"offset":   offset,
			"returned": len(data),
		},
	})
}

// AdminConfigDiff returns before/after config and changed field paths for GET /admin/config/diff.
func (h *Handlers) AdminConfigDiff(w http.ResponseWriter, r *http.Request) {
	if !requireGlobalConfigAdminRole(w, r) {
		return
	}
	ctx := r.Context()
	q := r.URL.Query()
	scope := q.Get("scope")
	tenantID := q.Get("tenant_id")
	fromStr := q.Get("from_version")
	toStr := q.Get("to_version")

	if scope != "global" && scope != "tenant" {
		writeError(w, http.StatusBadRequest, "invalid or missing scope (required: global or tenant)", "invalid_request_error")
		return
	}
	if scope == "tenant" && tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required when scope=tenant", "invalid_request_error")
		return
	}
	if fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "from_version and to_version are required", "invalid_request_error")
		return
	}
	fromVersion, err := strconv.Atoi(fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "from_version must be an integer", "invalid_request_error")
		return
	}
	toVersion, err := strconv.Atoi(toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "to_version must be an integer", "invalid_request_error")
		return
	}

	beforeRaw, err := h.store.GetConfigAtVersion(ctx, scope, tenantID, fromVersion)
	if err != nil {
		if errors.Is(err, storage.ErrConfigVersionNotFound) {
			writeError(w, http.StatusNotFound, "from_version not found", "not_found")
			return
		}
		h.log.ErrorContext(ctx, "failed to get config at from_version", "error", err, "scope", scope, "tenant_id", tenantID, "from_version", fromVersion)
		writeError(w, http.StatusInternalServerError, "failed to fetch config", "internal_error")
		return
	}
	afterRaw, err := h.store.GetConfigAtVersion(ctx, scope, tenantID, toVersion)
	if err != nil {
		if errors.Is(err, storage.ErrConfigVersionNotFound) {
			writeError(w, http.StatusNotFound, "to_version not found", "not_found")
			return
		}
		h.log.ErrorContext(ctx, "failed to get config at to_version", "error", err, "scope", scope, "tenant_id", tenantID, "to_version", toVersion)
		writeError(w, http.StatusInternalServerError, "failed to fetch config", "internal_error")
		return
	}

	var beforeMap, afterMap map[string]interface{}
	if len(beforeRaw) > 0 {
		if err := json.Unmarshal(beforeRaw, &beforeMap); err != nil {
			h.log.ErrorContext(ctx, "invalid before config JSON", "error", err)
			writeError(w, http.StatusInternalServerError, "invalid config at from_version", "internal_error")
			return
		}
	}
	if beforeMap == nil {
		beforeMap = make(map[string]interface{})
	}
	if len(afterRaw) > 0 {
		if err := json.Unmarshal(afterRaw, &afterMap); err != nil {
			h.log.ErrorContext(ctx, "invalid after config JSON", "error", err)
			writeError(w, http.StatusInternalServerError, "invalid config at to_version", "internal_error")
			return
		}
	}
	if afterMap == nil {
		afterMap = make(map[string]interface{})
	}

	changedFields := changedFieldPaths(beforeMap, afterMap, "")
	if changedFields == nil {
		changedFields = []string{}
	}

	respTenantID := ""
	if scope == "tenant" {
		respTenantID = tenantID
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"object":         "config_diff",
		"scope":          scope,
		"tenant_id":      respTenantID,
		"from_version":   fromVersion,
		"to_version":     toVersion,
		"before":         beforeMap,
		"after":          afterMap,
		"changed_fields": changedFields,
	})
}

// changedFieldPaths returns dot-notation paths of fields that differ between before and after (both JSON-like maps).
// Returns a non-nil slice so JSON marshaling never produces "changed_fields": null.
func changedFieldPaths(before, after map[string]interface{}, prefix string) []string {
	out := make([]string, 0)
	allKeys := make(map[string]bool)
	for k := range before {
		allKeys[k] = true
	}
	for k := range after {
		allKeys[k] = true
	}
	for key := range allKeys {
		p := prefix + key
		bv := before[key]
		av := after[key]
		bm, bok := bv.(map[string]interface{})
		am, aok := av.(map[string]interface{})
		if bok && aok {
			out = append(out, changedFieldPaths(bm, am, p+".")...)
		} else if !reflect.DeepEqual(bv, av) {
			out = append(out, p)
		}
	}
	return out
}

// ============================================================================
// Helper functions
// ============================================================================

// ensureTenantConfigInDB seeds the tenant config from YAML if it doesn't exist in DB yet.
// This allows PUT/PATCH to work seamlessly even when the tenant only exists in config.yaml.
//
// IMPORTANT: Seeding does NOT create a config_change_log entry - only subsequent updates do.
func (h *Handlers) ensureTenantConfigInDB(ctx context.Context, tenantID string) error {
	// Check if tenant already exists in DB
	_, _, exists, err := h.store.GetTenantConfig(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("check tenant config: %w", err)
	}
	if exists {
		return nil // Already in DB, nothing to do
	}

	// Tenant not in DB - try to seed from YAML
	yamlTenant := h.cfg.TenantByID(tenantID)
	if yamlTenant == nil {
		// Tenant doesn't exist in YAML either - this is fine, PUT will create it
		return nil
	}

	// Prepare initial config JSON (strip API keys - they stay in YAML)
	configForDB := stripAPIKeys(*yamlTenant)
	configJSON, err := json.Marshal(configForDB)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Seed from YAML (INSERT with version=0, no change log)
	// Uses ON CONFLICT DO NOTHING to handle race conditions
	_, err = h.store.SeedTenantConfig(ctx, tenantID, configJSON)
	if err != nil {
		return fmt.Errorf("seed tenant config: %w", err)
	}

	return nil
}

// validateTenantConfig validates tenant configuration
func validateTenantConfig(tc *config.TenantConfig, cfg *config.Config) error {
	// Validate allowed_models not empty
	if len(tc.AllowedModels) == 0 {
		return fmt.Errorf("allowed_models must not be empty")
	}

	// Validate models exist
	for _, modelName := range tc.AllowedModels {
		if cfg.ModelByName(modelName) == nil {
			return fmt.Errorf("invalid model: %s", modelName)
		}
	}

	// Validate routing strategy
	validStrategies := map[string]bool{
		"round_robin": true, "latency_based": true, "cost_based": true,
		"header_based": true, "smart": true,
	}
	if !validStrategies[tc.Routing.Strategy] {
		return fmt.Errorf("invalid routing strategy: %s", tc.Routing.Strategy)
	}

	// Validate rate limit ranges
	if tc.RateLimit.RPM < 1 || tc.RateLimit.RPM > 10000 {
		return fmt.Errorf("rate_limit.rpm must be between 1 and 10000")
	}
	if tc.RateLimit.Burst < 1 || tc.RateLimit.Burst > 1000 {
		return fmt.Errorf("rate_limit.burst must be between 1 and 1000")
	}

	// Validate compliance
	if tc.Compliance.RetentionDays < 7 || tc.Compliance.RetentionDays > 365 {
		return fmt.Errorf("compliance.retention_days must be between 7 and 365")
	}

	validLogModes := map[string]bool{"metadata_only": true, "redacted": true, "full": true}
	if !validLogModes[tc.Compliance.LogMode] {
		return fmt.Errorf("invalid compliance.log_mode: %s", tc.Compliance.LogMode)
	}

	// Validate timezone
	if _, err := time.LoadLocation(tc.Budgets.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %s", tc.Budgets.Timezone)
	}

	// Validate budgets
	if tc.Budgets.MonthlyUSD < 0 || tc.Budgets.MonthlyUSD > 1000000 {
		return fmt.Errorf("budgets.monthly_usd must be between 0 and 1000000")
	}

	// Validate routing.route_group exists in selection.route_groups (SPEC_148)
	if tc.Routing.RouteGroup != "" {
		if _, exists := tc.Selection.RouteGroups[tc.Routing.RouteGroup]; !exists {
			return fmt.Errorf("routing.route_group '%s' does not exist in selection.route_groups", tc.Routing.RouteGroup)
		}
	}

	return nil
}

// deleteNestedSecrets removes api_key and token fields at any depth within m.
func deleteNestedSecrets(m map[string]interface{}) {
	for k, v := range m {
		switch k {
		case "api_key", "token":
			delete(m, k)
		default:
			if nested, ok := v.(map[string]interface{}); ok {
				deleteNestedSecrets(nested)
			}
		}
	}
}

// stripAPIKeys removes api_keys field (stays in YAML, not stored in DB)
func stripAPIKeys(tc config.TenantConfig) map[string]interface{} {
	data, _ := json.Marshal(tc)
	var result map[string]interface{}
	json.Unmarshal(data, &result)

	delete(result, "api_keys")
	delete(result, "id") // ID is the primary key, not part of config_json
	// Strip webhook secrets (api_key, auth.token) from hooks subtree.
	if hooks, ok := result["hooks"].(map[string]interface{}); ok {
		deleteNestedSecrets(hooks)
	}

	return result
}

// ============================================================================
// Environment helpers (SPEC_133)
// ============================================================================

var validEnvironments = map[string]bool{"DEV": true, "STAGING": true, "PROD": true}

// validateTenantEnvironment returns an error if v is not a valid environment value.
func validateTenantEnvironment(v string) error {
	if !validEnvironments[v] {
		return fmt.Errorf("invalid environment: must be one of DEV, STAGING, PROD")
	}
	return nil
}

// normalizeTenantEnvironment returns DEV when v is empty; validates otherwise.
func normalizeTenantEnvironment(v string) (string, error) {
	if v == "" {
		return "DEV", nil
	}
	if err := validateTenantEnvironment(v); err != nil {
		return "", err
	}
	return v, nil
}

// currentEnvironmentFromJSON extracts the environment field from a stored config JSON blob.
// Returns "" if the field is absent or the value is not a string.
func currentEnvironmentFromJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, ok := m["environment"]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}
