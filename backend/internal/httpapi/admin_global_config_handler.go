package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/diegomcastronuovo/prism-gateway/internal/auth"
	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
)

// requireGlobalConfigAdminRole enforces admin (full) for global config endpoints.
// If no roles are present in context (e.g. admin token/api key path), this check is neutral.
func requireGlobalConfigAdminRole(w http.ResponseWriter, r *http.Request) bool {
	roles := auth.RolesFromContext(r.Context())
	if len(roles) == 0 || auth.HasAnyRole(roles, adminBypassRoles) {
		return true
	}
	writeError(w, http.StatusForbidden, "insufficient permissions", "authorization_error")
	return false
}

// ── GET /admin/config/global ──────────────────────────────────────────────────

// AdminGetGlobalConfig returns the current active global configuration + its version.
//
// Response:
//
//	{ "version": 12, "config": { ... } }
func (h *Handlers) AdminGetGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if !requireGlobalConfigAdminRole(w, r) {
		return
	}
	ctx := r.Context()

	configJSON, version, exists, err := h.store.GetGlobalConfig(ctx)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch global config", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch global config", "internal_error")
		return
	}

	if !exists {
		// No DB record — fall back to YAML-derived config with version 0.
		gc := config.GlobalConfigFromYAML(h.cfg)
		configJSON, _ = json.Marshal(gc)
		version = 0
	}

	var configMap map[string]interface{}
	_ = json.Unmarshal(configJSON, &configMap)
	redactProviderSecretsInGlobalConfigMap(configMap)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version": version,
		"config":  configMap,
	})
}

// stripProviderSecretsForStorage returns a shallow copy of gc with provider credentials
// (APIKey, AwsSecretAccessKey) zeroed so they are never persisted to global_config_versions.
// Credentials must be sourced at runtime from environment variables (APIKeyEnv).
func stripProviderSecretsForStorage(gc config.GlobalConfig) config.GlobalConfig {
	if len(gc.Providers) == 0 {
		return gc
	}
	stripped := make(map[string]config.ProviderConfig, len(gc.Providers))
	for k, p := range gc.Providers {
		p.APIKey = ""
		p.AwsSecretAccessKey = ""
		stripped[k] = p
	}
	gc.Providers = stripped
	return gc
}

// redactProviderSecretsInGlobalConfigMap replaces stored api_key / aws_secret_access_key values
// so admin GET does not leak credentials (they remain persisted in the database).
func redactProviderSecretsInGlobalConfigMap(root map[string]interface{}) {
	if root == nil {
		return
	}
	provRaw, ok := root["providers"]
	if !ok {
		return
	}
	provMap, ok := provRaw.(map[string]interface{})
	if !ok {
		return
	}
	for _, entry := range provMap {
		pm, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range []string{"api_key", "APIKey", "aws_secret_access_key", "AwsSecretAccessKey"} {
			if v, ok := pm[key].(string); ok && v != "" {
				pm[key] = "***"
			}
		}
	}
}

// ── PUT /admin/config/global ──────────────────────────────────────────────────

// AdminPutGlobalConfig replaces the entire global configuration.
//
// Requires:  If-Match-Version header (optimistic locking).
// Validates: JSON schema + config.ValidateGlobalConfig warnings.
// On success returns { "version": <new>, "message": "..." }.
func (h *Handlers) AdminPutGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if !requireGlobalConfigAdminRole(w, r) {
		return
	}
	ctx := r.Context()

	ifMatchVersion, ok := parseIfMatchVersion(w, r)
	if !ok {
		return
	}

	// Read raw body; normalize keys and canonicalize providers (no duplicate legacy keys).
	rawBody, err := readRequestBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}
	finalized, err := storage.FinalizeGlobalConfigJSON(rawBody)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid config JSON: "+err.Error(), "invalid_request_error")
		return
	}
	var newGC config.GlobalConfig
	if err := json.Unmarshal(finalized, &newGC); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}

	if validationErr := validateGlobalConfigOrError(&newGC); validationErr != nil {
		writeError(w, http.StatusUnprocessableEntity, validationErr.Error(), "validation_error")
		return
	}

	warnings := config.ValidateGlobalConfig(&newGC)
	for _, w2 := range warnings {
		h.log.WarnContext(ctx, "global config warning", "warning", w2)
	}

	configJSON, _ := json.Marshal(stripProviderSecretsForStorage(newGC))

	actorSub, actorRoles := extractActor(r)

	newVersion, err := h.store.PutGlobalConfig(ctx, ifMatchVersion, configJSON, actorSub, actorRoles)
	if err != nil {
		if _, ok := err.(storage.ErrVersionConflict); ok {
			writeError(w, http.StatusConflict, err.Error(), "version_conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to put global config", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update global config", "internal_error")
		return
	}

	h.invalidateGlobalConfigCache()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":  newVersion,
		"message":  "Global configuration updated successfully",
		"warnings": warnings,
	})
}

// ── PATCH /admin/config/global ────────────────────────────────────────────────

// AdminPatchGlobalConfig partially updates the global configuration.
//
// Two modes, distinguished by body shape:
//   - Rollback:    { "rollback_to_version": <N> }  — no If-Match-Version required
//     Actually the spec still shows If-Match-Version for rollback.
//   - Merge patch: any other JSON object — applies RFC 7396 merge patch.
//
// Requires: If-Match-Version header in both modes.
func (h *Handlers) AdminPatchGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if !requireGlobalConfigAdminRole(w, r) {
		return
	}
	ctx := r.Context()

	ifMatchVersion, ok := parseIfMatchVersion(w, r)
	if !ok {
		return
	}

	// Read raw body so we can inspect it for rollback_to_version.
	rawBody, err := readRequestBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}

	actorSub, actorRoles := extractActor(r)

	// ── Rollback path ──────────────────────────────────────────────────────
	var rollbackBody struct {
		RollbackToVersion *int `json:"rollback_to_version"`
	}
	if err := json.Unmarshal(rawBody, &rollbackBody); err == nil && rollbackBody.RollbackToVersion != nil {
		targetVersion := *rollbackBody.RollbackToVersion
		if targetVersion <= 0 {
			writeError(w, http.StatusBadRequest, "rollback_to_version must be a positive integer", "invalid_request_error")
			return
		}

		err := h.store.RollbackGlobalConfig(ctx, ifMatchVersion, targetVersion, actorSub, actorRoles)
		if err != nil {
			if _, ok := err.(storage.ErrVersionConflict); ok {
				writeError(w, http.StatusConflict, err.Error(), "version_conflict_error")
				return
			}
			// Check for "not found" message (version doesn't exist).
			if isNotFoundErr(err) {
				writeError(w, http.StatusNotFound, err.Error(), "not_found")
				return
			}
			h.log.ErrorContext(ctx, "failed to rollback global config", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to rollback global config", "internal_error")
			return
		}

		h.invalidateGlobalConfigCache()

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"version": targetVersion,
			"message": fmt.Sprintf("Global configuration rolled back to version %d", targetVersion),
		})
		return
	}

	// ── Merge-patch path ───────────────────────────────────────────────────
	// Normalize patch keys to snake_case so frontend-sent PascalCase/duplicates don't break config.
	normalizedPatch, err := storage.NormalizeJSONConfig(rawBody)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid patch JSON: "+err.Error(), "invalid_request_error")
		return
	}
	newVersion, err := h.store.PatchGlobalConfig(ctx, ifMatchVersion, normalizedPatch, actorSub, actorRoles)
	if err != nil {
		if _, ok := err.(storage.ErrVersionConflict); ok {
			writeError(w, http.StatusConflict, err.Error(), "version_conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to patch global config", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to patch global config", "internal_error")
		return
	}

	h.invalidateGlobalConfigCache()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version": newVersion,
		"message": "Global configuration patched successfully",
	})
}

// ── POST /admin/config/global/apply ───────────────────────────────────────────

// AdminApplyGlobalConfigVersion applies an existing config version as the active one (hot rollback).
// Request body: { "version": <N> }. Validates version exists, updates the pointer, invalidates cache.
func (h *Handlers) AdminApplyGlobalConfigVersion(w http.ResponseWriter, r *http.Request) {
	if !requireGlobalConfigAdminRole(w, r) {
		return
	}
	ctx := r.Context()
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error")
		return
	}
	var body struct {
		Version *int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}
	if body.Version == nil {
		writeError(w, http.StatusBadRequest, "version is required", "invalid_request_error")
		return
	}
	version := *body.Version
	if version <= 0 {
		writeError(w, http.StatusBadRequest, "version must be a positive integer", "invalid_request_error")
		return
	}
	actorSub, actorRoles := extractActor(r)
	err := h.store.ApplyGlobalConfigVersion(ctx, version, actorSub, actorRoles)
	if err != nil {
		if isNotFoundErr(err) || strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error(), "not_found")
			return
		}
		h.log.ErrorContext(ctx, "failed to apply global config version", "error", err, "version", version)
		writeError(w, http.StatusInternalServerError, "failed to apply version", "internal_error")
		return
	}
	h.invalidateGlobalConfigCache()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version": version,
		"message": "Global configuration applied (active version updated)",
	})
}

// ── PATCH /admin/models/{model_name} ─────────────────────────────────────────

// AdminPatchModel partially updates a single model's configuration within the global config.
//
// The model patch is applied to the model's entry in GlobalConfig.Models.
// Requires: If-Match-Version header.
// Returns:  { "version": <new>, "model_name": "...", "message": "..." }
func (h *Handlers) AdminPatchModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	modelName := r.PathValue("model_name")
	if modelName == "" {
		writeError(w, http.StatusBadRequest, "model_name is required", "invalid_request_error")
		return
	}

	ifMatchVersion, ok := parseIfMatchVersion(w, r)
	if !ok {
		return
	}

	// Parse patch body — treat as a partial ModelConfig.
	var patchMap map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patchMap); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}

	// Fetch current global config.
	currentJSON, currentVersion, exists, err := h.store.GetGlobalConfig(ctx)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch global config for model patch", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch global config", "internal_error")
		return
	}

	var gc config.GlobalConfig
	if exists {
		if err := json.Unmarshal(currentJSON, &gc); err != nil {
			h.log.ErrorContext(ctx, "failed to unmarshal global config", "error", err)
			writeError(w, http.StatusInternalServerError, "global config is corrupt", "internal_error")
			return
		}
	} else {
		// Fall back to YAML-derived config.
		gc = *config.GlobalConfigFromYAML(h.cfg)
		currentVersion = 0
	}

	// Verify If-Match-Version against the actual current version.
	if currentVersion != ifMatchVersion {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("version conflict: expected %d, current %d", ifMatchVersion, currentVersion),
			"version_conflict_error")
		return
	}

	// Find the model.
	modelIdx := -1
	for i, m := range gc.Models {
		if m.Name == modelName {
			modelIdx = i
			break
		}
	}
	if modelIdx == -1 {
		writeError(w, http.StatusNotFound, "model not found: "+modelName, "not_found")
		return
	}

	// Apply patch to the model: marshal model → map, merge, unmarshal back.
	modelJSON, _ := json.Marshal(gc.Models[modelIdx])
	var modelMap map[string]interface{}
	_ = json.Unmarshal(modelJSON, &modelMap)

	merged := mergeMapsPublic(modelMap, patchMap)

	mergedJSON, _ := json.Marshal(merged)
	var patchedModel config.ModelConfig
	if err := json.Unmarshal(mergedJSON, &patchedModel); err != nil {
		writeError(w, http.StatusBadRequest, "invalid model patch: "+err.Error(), "invalid_request_error")
		return
	}
	// Path model_name is authoritative: do not allow PATCH to change model id.
	patchedModel.Name = modelName
	gc.Models[modelIdx] = patchedModel

	// Validate model fields (provider, pricing, mock).
	if err := validateModelConfigOrError(&patchedModel, &gc, h.cfg.Providers); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "validation_error")
		return
	}

	// Validate the whole config after patching.
	warnings := config.ValidateGlobalConfig(&gc)
	for _, w2 := range warnings {
		h.log.WarnContext(ctx, "global config warning after model patch", "warning", w2)
	}

	newConfigJSON, _ := json.Marshal(stripProviderSecretsForStorage(gc))
	actorSub, actorRoles := extractActor(r)

	newVersion, err := h.store.PutGlobalConfig(ctx, ifMatchVersion, newConfigJSON, actorSub, actorRoles)
	if err != nil {
		if _, ok := err.(storage.ErrVersionConflict); ok {
			writeError(w, http.StatusConflict, err.Error(), "version_conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to update global config after model patch", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update model config", "internal_error")
		return
	}

	h.invalidateGlobalConfigCache()


	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":    newVersion,
		"model_name": modelName,
		"message":    fmt.Sprintf("Model %q configuration updated successfully", modelName),
		"warnings":   warnings,
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// parseIfMatchVersion reads and parses the If-Match-Version header.
// Writes 428/400 and returns (0, false) on failure.
func parseIfMatchVersion(w http.ResponseWriter, r *http.Request) (int, bool) {
	s := r.Header.Get("If-Match-Version")
	if s == "" {
		writeError(w, 428, "missing If-Match-Version header", "precondition_required")
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid If-Match-Version", "invalid_request_error")
		return 0, false
	}
	return v, true
}

// extractActor reads actor_sub and actor_roles from the request context.
func extractActor(r *http.Request) (string, []string) {
	actorSub := auth.SubFromContext(r.Context())
	if actorSub == "" {
		actorSub = "admin-token"
	}
	actorRoles := auth.RolesFromContext(r.Context())
	if len(actorRoles) == 0 {
		actorRoles = []string{"admin"}
	}
	return actorSub, actorRoles
}

// readRequestBody reads the request body and returns it as json.RawMessage.
func readRequestBody(r *http.Request) (json.RawMessage, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return json.RawMessage(`{}`), nil
	}
	// Validate it's valid JSON (optional but helps surface errors early).
	var dummy interface{}
	if err := json.Unmarshal(body, &dummy); err != nil {
		return nil, err
	}
	return body, nil
}

// validateModelConfigOrError validates a single model for PATCH/POST: provider exists
// (in gc.Providers or fallbackProviders), pricing non-negative, mock bounds.
func validateModelConfigOrError(m *config.ModelConfig, gc *config.GlobalConfig, fallbackProviders map[string]config.ProviderConfig) error {
	if m.Provider == "" {
		return errors.New("provider is required")
	}
	providerOk := false
	if gc.Providers != nil {
		if _, ok := gc.Providers[m.Provider]; ok {
			providerOk = true
		}
	}
	if !providerOk && fallbackProviders != nil {
		if _, ok := fallbackProviders[m.Provider]; ok {
			providerOk = true
		}
	}
	if !providerOk {
		return fmt.Errorf("provider %q is not configured", m.Provider)
	}
	if m.Pricing.PromptPer1M < 0 || m.Pricing.CompletionPer1M < 0 {
		return errors.New("pricing values must be non-negative")
	}
	if m.InfrastructureMonthlyUSD < 0 {
		return errors.New("infrastructure_monthly_usd must be non-negative")
	}
	if m.MarkupPercentage < 0 {
		return errors.New("markup_percentage must be non-negative")
	}
	if m.Type == "ml" {
		if m.Execution == nil || m.Execution.Endpoint == "" {
			return errors.New("execution.endpoint is required for ml models")
		}
	}
	if m.Observable != nil {
		seen := make(map[string]struct{})
		for i, f := range m.Observable.Fields {
			if strings.TrimSpace(f.Path) == "" {
				return fmt.Errorf("observable.fields[%d].path must not be empty", i)
			}
			switch f.Type {
			case "text", "json", "number":
			default:
				return fmt.Errorf("observable.fields[%d].type must be one of: text, json, number", i)
			}
			switch f.Role {
			case "input", "output":
			default:
				return fmt.Errorf("observable.fields[%d].role must be one of: input, output", i)
			}
			key := f.Path + "|" + f.Type + "|" + f.Role
			if _, dup := seen[key]; dup {
				return fmt.Errorf("observable.fields contains duplicate entry: path=%q type=%q role=%q", f.Path, f.Type, f.Role)
			}
			seen[key] = struct{}{}
		}
	}
	if m.Mock.DelayMinMs < 0 || m.Mock.DelayMaxMs < 0 {
		return errors.New("mock delay_min_ms and delay_max_ms must be non-negative")
	}
	if m.Mock.DelayMaxMs < m.Mock.DelayMinMs {
		return errors.New("mock delay_max_ms must be >= delay_min_ms")
	}
	if m.Mock.ErrorRate < 0 || m.Mock.ErrorRate > 1 {
		return errors.New("mock error_rate must be between 0 and 1")
	}
	if m.Mock.ErrorStatus != 0 && (m.Mock.ErrorStatus < 100 || m.Mock.ErrorStatus > 599) {
		return errors.New("mock error_status must be a valid HTTP status (100-599)")
	}
	return nil
}

// validateGlobalConfigOrError returns an error for structural problems that should
// block activation (e.g. duplicate model names, invalid auth). It does NOT return
// an error for soft warnings (missing pricing, unknown provider) — those are logged separately.
func validateGlobalConfigOrError(gc *config.GlobalConfig) error {
	seen := make(map[string]bool)
	for _, m := range gc.Models {
		if m.Name == "" {
			return errors.New("all models must have a non-empty name")
		}
		if seen[m.Name] {
			return fmt.Errorf("duplicate model name: %q", m.Name)
		}
		seen[m.Name] = true
	}
	if err := validateGlobalConfigAuth(gc.Auth); err != nil {
		return err
	}
	return nil
}

// validateGlobalConfigAuth validates the auth block when present.
// auth.mode must be api_key, jwt, or both; when mode includes JWT, issuer/audience/jwks_url
// and required_claims must be valid; rbac arrays must be string slices.
func validateGlobalConfigAuth(auth *config.AuthConfig) error {
	if auth == nil {
		return nil
	}
	validModes := map[string]bool{"api_key": true, "jwt": true, "both": true}
	if auth.Mode == "" {
		return errors.New("auth.mode is required when auth is present")
	}
	if !validModes[auth.Mode] {
		return fmt.Errorf("auth.mode must be one of api_key, jwt, both; got %q", auth.Mode)
	}
	useJWT := auth.Mode == "jwt" || auth.Mode == "both"
	if useJWT {
		if auth.JWT.Issuer == "" {
			return errors.New("auth.jwt.issuer is required when auth.mode includes JWT")
		}
		if auth.JWT.Audience == "" {
			return errors.New("auth.jwt.audience is required when auth.mode includes JWT")
		}
		if auth.JWT.JWKSURL == "" {
			return errors.New("auth.jwt.jwks_url is required when auth.mode includes JWT")
		}
		if u, err := url.Parse(auth.JWT.JWKSURL); err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("auth.jwt.jwks_url must be a valid URL")
		}
		if auth.JWT.RequiredClaims != nil {
			if t := strings.TrimSpace(auth.JWT.RequiredClaims["tenant_id"]); t == "" {
				return errors.New("auth.jwt.required_claims.tenant_id must be a non-empty string when required_claims is set")
			}
			if r := strings.TrimSpace(auth.JWT.RequiredClaims["roles"]); r == "" {
				return errors.New("auth.jwt.required_claims.roles must be a non-empty string when required_claims is set")
			}
		}
	}
	// RBAC arrays (auth.jwt.rbac): must be string slices (no null coercion to invalid structures)
	if auth.JWT.RBAC.UserRoles == nil {
		auth.JWT.RBAC.UserRoles = []string{}
	}
	if auth.JWT.RBAC.AdminRoles == nil {
		auth.JWT.RBAC.AdminRoles = []string{}
	}
	if auth.JWT.RBAC.FinanceRoles == nil {
		auth.JWT.RBAC.FinanceRoles = []string{}
	}
	return nil
}

// isNotFoundErr reports whether the error message indicates a missing version.
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) > 10 && msg[:10] == "version " // "version N not found …"
}

// mergeMapsPublic applies an RFC 7396 JSON merge patch: patch values overwrite
// target values; nested maps are merged recursively; null patch values delete keys.
func mergeMapsPublic(target, patch map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(target))
	for k, v := range target {
		result[k] = v
	}
	for k, pv := range patch {
		if pv == nil {
			delete(result, k)
			continue
		}
		if pMap, ok := pv.(map[string]interface{}); ok {
			if tMap, ok2 := result[k].(map[string]interface{}); ok2 {
				result[k] = mergeMapsPublic(tMap, pMap)
				continue
			}
		}
		result[k] = pv
	}
	return result
}

// createModelBody is the request body for POST /admin/models (API uses "id" and snake_case).
type createModelBody struct {
	ID                       string                      `json:"id"`
	Provider                 string                      `json:"provider"`
	ProviderModelID          string                      `json:"provider_model_id,omitempty"`
	Type                     string                      `json:"type"`
	BaseURL                  string                      `json:"base_url,omitempty"`
	Pricing                  *createModelPricing         `json:"pricing"`
	Mock                     *createModelMock            `json:"mock"`
	InfrastructureMonthlyUSD float64                     `json:"infrastructure_monthly_usd"`
	MarkupPercentage         float64                     `json:"markup_percentage"`
	Execution                *config.MLExecutionConfig   `json:"execution"`
	Observable               *config.MLObservableConfig  `json:"observable"`
}

type createModelPricing struct {
	PromptPer1M     float64 `json:"prompt_per_1m"`
	CompletionPer1M float64 `json:"completion_per_1m"`
}

type createModelMock struct {
	Enabled       bool    `json:"enabled"`
	DelayMinMs    int     `json:"delay_min_ms"`
	DelayMaxMs    int     `json:"delay_max_ms"`
	ErrorRate     float64 `json:"error_rate"`
	ErrorStatus   int     `json:"error_status"`
	ErrorMessage  string  `json:"error_message"`
	FixedResponse string  `json:"fixed_response"`
}

// AdminCreateModel creates a new model in the global config. POST /admin/models
// Requires If-Match-Version. Body: id (required), provider (required), optional type, pricing, mock.
func (h *Handlers) AdminCreateModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	ifMatchVersion, ok := parseIfMatchVersion(w, r)
	if !ok {
		return
	}

	var body createModelBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}
	if body.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required", "invalid_request_error")
		return
	}
	if body.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required", "invalid_request_error")
		return
	}

	currentJSON, currentVersion, exists, err := h.store.GetGlobalConfig(ctx)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch global config for model create", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch global config", "internal_error")
		return
	}

	var gc config.GlobalConfig
	if exists {
		if err := json.Unmarshal(currentJSON, &gc); err != nil {
			h.log.ErrorContext(ctx, "failed to unmarshal global config", "error", err)
			writeError(w, http.StatusInternalServerError, "global config is corrupt", "internal_error")
			return
		}
	} else {
		gc = *config.GlobalConfigFromYAML(h.cfg)
		currentVersion = 0
	}

	if currentVersion != ifMatchVersion {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("version conflict: expected %d, current %d", ifMatchVersion, currentVersion),
			"version_conflict_error")
		return
	}

	for _, m := range gc.Models {
		if m.Name == body.ID {
			writeError(w, http.StatusConflict, "model already exists: "+body.ID, "duplicate_model")
			return
		}
	}

	m := config.ModelConfig{
		Name:                     body.ID,
		Provider:                 body.Provider,
		Type:                     body.Type,
		BaseURL:                  body.BaseURL,
		InfrastructureMonthlyUSD: body.InfrastructureMonthlyUSD,
		MarkupPercentage:         body.MarkupPercentage,
		Execution:                body.Execution,
		Observable:               body.Observable,
	}
	// Bedrock (catalog provider id "bedrock") requires provider_model_id at runtime; default to Model ID when omitted.
	pid := strings.TrimSpace(body.ProviderModelID)
	if pid == "" && body.Provider == "bedrock" {
		pid = strings.TrimSpace(body.ID)
	}
	if pid != "" {
		m.ProviderModelID = pid
	}
	if body.Pricing != nil {
		m.Pricing.PromptPer1M = body.Pricing.PromptPer1M
		m.Pricing.CompletionPer1M = body.Pricing.CompletionPer1M
	}
	if body.Mock != nil {
		m.Mock.Enabled = body.Mock.Enabled
		m.Mock.DelayMinMs = body.Mock.DelayMinMs
		m.Mock.DelayMaxMs = body.Mock.DelayMaxMs
		m.Mock.ErrorRate = body.Mock.ErrorRate
		m.Mock.ErrorStatus = body.Mock.ErrorStatus
		m.Mock.ErrorMessage = body.Mock.ErrorMessage
		m.Mock.FixedResponse = body.Mock.FixedResponse
	}

	if err := validateModelConfigOrError(&m, &gc, h.cfg.Providers); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "validation_error")
		return
	}

	gc.Models = append(gc.Models, m)
	newConfigJSON, _ := json.Marshal(stripProviderSecretsForStorage(gc))
	actorSub, actorRoles := extractActor(r)

	newVersion, err := h.store.PutGlobalConfig(ctx, ifMatchVersion, newConfigJSON, actorSub, actorRoles)
	if err != nil {
		if _, ok := err.(storage.ErrVersionConflict); ok {
			writeError(w, http.StatusConflict, err.Error(), "version_conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to create model", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create model", "internal_error")
		return
	}

	h.invalidateGlobalConfigCache()

	w.Header().Set("Location", "/admin/models/"+body.ID)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"version":    newVersion,
		"model_name": body.ID,
		"message":    fmt.Sprintf("Model %q created successfully", body.ID),
	})
}

// tenantsReferencingModel returns tenant IDs that reference the given model (allowed_models or route_groups).
func tenantsReferencingModel(tenants []config.TenantConfig, modelID string) []string {
	var out []string
	for _, t := range tenants {
		for _, name := range t.AllowedModels {
			if name == modelID {
				out = append(out, t.ID)
				break
			}
		}
		for _, models := range t.Selection.RouteGroups {
			for _, name := range models {
				if name == modelID {
					out = append(out, t.ID)
					break
				}
			}
		}
	}
	// Dedupe
	seen := make(map[string]struct{})
	var deduped []string
	for _, id := range out {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	return deduped
}

// AdminDeleteModel removes a model from the global config. DELETE /admin/models/{model_id}
// Rejects with 409 if the model is referenced by any tenant (allowed_models or route_groups).
func (h *Handlers) AdminDeleteModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelID := r.PathValue("model_id")
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "model_id is required", "invalid_request_error")
		return
	}

	ifMatchVersion, ok := parseIfMatchVersion(w, r)
	if !ok {
		return
	}

	currentJSON, currentVersion, exists, err := h.store.GetGlobalConfig(ctx)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to fetch global config for model delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch global config", "internal_error")
		return
	}

	var gc config.GlobalConfig
	if exists {
		if err := json.Unmarshal(currentJSON, &gc); err != nil {
			h.log.ErrorContext(ctx, "failed to unmarshal global config", "error", err)
			writeError(w, http.StatusInternalServerError, "global config is corrupt", "internal_error")
			return
		}
	} else {
		gc = *config.GlobalConfigFromYAML(h.cfg)
		currentVersion = 0
	}

	if currentVersion != ifMatchVersion {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("version conflict: expected %d, current %d", ifMatchVersion, currentVersion),
			"version_conflict_error")
		return
	}

	refs := tenantsReferencingModel(h.loadAllTenantConfigs(ctx), modelID)
	if len(refs) > 0 {
		writeError(w, http.StatusConflict,
			fmt.Sprintf("model %q is in use by tenant(s): %v", modelID, refs),
			"model_in_use")
		return
	}

	// Remove model from slice.
	newModels := make([]config.ModelConfig, 0, len(gc.Models)-1)
	for i := range gc.Models {
		if gc.Models[i].Name != modelID {
			newModels = append(newModels, gc.Models[i])
		}
	}
	if len(newModels) == len(gc.Models) {
		writeError(w, http.StatusNotFound, "model not found: "+modelID, "not_found")
		return
	}
	gc.Models = newModels
	newConfigJSON, _ := json.Marshal(stripProviderSecretsForStorage(gc))
	actorSub, actorRoles := extractActor(r)

	_, err = h.store.PutGlobalConfig(ctx, ifMatchVersion, newConfigJSON, actorSub, actorRoles)
	if err != nil {
		if _, ok := err.(storage.ErrVersionConflict); ok {
			writeError(w, http.StatusConflict, err.Error(), "version_conflict_error")
			return
		}
		h.log.ErrorContext(ctx, "failed to delete model", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete model", "internal_error")
		return
	}

	h.invalidateGlobalConfigCache()

	w.WriteHeader(http.StatusNoContent)
}
