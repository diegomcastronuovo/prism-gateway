package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/diegomcastronuovo/prism-gateway/internal/config"
	"github.com/diegomcastronuovo/prism-gateway/internal/storage"
	"github.com/google/uuid"
)

// BootstrapGlobalConfig seeds the global config (models/providers/CB/RL/smart_routing) into
// the database on startup, controlled by DYNAMIC_CONFIG_SEED_MODE env or dynamic_config.seed_mode.
//
// Modes:
//   - "never"    → skip (default in prod)
//   - "if_empty" → seed only when no active global config exists in DB (default in dev)
//   - "always"   → always seed a new version (test/local only; never use in prod)
//
// Returns an error only for unexpected DB failures; "already exists" is not an error.
func BootstrapGlobalConfig(ctx context.Context, cfg *config.Config, store storage.Storage, log *slog.Logger) error {
	// Resolve seed mode: env var overrides YAML config
	seedMode := cfg.DynamicConfig.SeedMode
	if envMode := os.Getenv("DYNAMIC_CONFIG_SEED_MODE"); envMode != "" {
		seedMode = envMode
	}
	if seedMode == "" {
		seedMode = "if_empty" // safe default: seed only when DB is empty
	}

	switch seedMode {
	case "never":
		log.InfoContext(ctx, "global config seed skipped", "seed_mode", seedMode)
		return nil
	case "if_empty", "always":
		// fall through to seeding logic
	default:
		log.WarnContext(ctx, "unknown DYNAMIC_CONFIG_SEED_MODE, defaulting to if_empty", "seed_mode", seedMode)
		seedMode = "if_empty"
	}

	// Build GlobalConfig from YAML — strip inline credentials before persisting;
	// runtime keys are sourced from environment variables (APIKeyEnv).
	gc := config.GlobalConfigFromYAML(cfg)
	gcJSON, err := json.Marshal(stripProviderSecretsForStorage(*gc))
	if err != nil {
		return fmt.Errorf("marshal global config: %w", err)
	}

	if seedMode == "always" {
		// Force-overwrite: get current version and replace it unconditionally.
		_, currentVersion, exists, err := store.GetGlobalConfig(ctx)
		if err != nil {
			return fmt.Errorf("get global config for force-seed: %w", err)
		}
		if !exists {
			// Nothing in DB yet — fall through to normal seed below.
			goto normalSeed
		}
		if _, err := store.PutGlobalConfig(ctx, currentVersion, gcJSON, "bootstrap", []string{"system"}); err != nil {
			return fmt.Errorf("force-seed global config: %w", err)
		}
		log.InfoContext(ctx, "global config force-seeded from YAML", "seed_mode", seedMode, "replaced_version", currentVersion)
		return nil
	}

normalSeed:
	seeded, err := store.SeedGlobalConfig(ctx, gcJSON)
	if err != nil {
		return fmt.Errorf("seed global config: %w", err)
	}
	if seeded {
		log.InfoContext(ctx, "global config seeded from YAML", "seed_mode", seedMode)
	} else {
		log.InfoContext(ctx, "global config already in DB, skipping seed", "seed_mode", seedMode)
	}

	return nil
}

// BootstrapTenantFullSeeding seeds tenant_config_versions, tenant_active_config, and api_keys
// for all YAML-defined tenants. Called only when dynamic_config.enabled = true.
// Controlled by the same DYNAMIC_CONFIG_SEED_MODE env/config as BootstrapGlobalConfig.
func BootstrapTenantFullSeeding(ctx context.Context, cfg *config.Config, store storage.Storage, log *slog.Logger) error {
	// Resolve seed mode (same logic as BootstrapGlobalConfig)
	seedMode := cfg.DynamicConfig.SeedMode
	if envMode := os.Getenv("DYNAMIC_CONFIG_SEED_MODE"); envMode != "" {
		seedMode = envMode
	}
	if seedMode == "" || (seedMode != "if_empty" && seedMode != "always") {
		seedMode = "if_empty"
	}

	for _, tenant := range cfg.Tenants {
		configForDB := stripAPIKeys(tenant)
		configJSON, err := json.Marshal(configForDB)
		if err != nil {
			return fmt.Errorf("marshal tenant config %s: %w", tenant.ID, err)
		}

		// 1. Seed tenant_config_versions + tenant_active_config
		seeded, err := store.SeedTenantVersionedConfig(ctx, tenant.ID, configJSON, seedMode)
		if err != nil {
			return fmt.Errorf("seed tenant versioned config %s: %w", tenant.ID, err)
		}
		if seeded {
			log.InfoContext(ctx, "tenant versioned config seeded", "tenant_id", tenant.ID)
		}

		// 2. Seed API keys from YAML
		for _, apiKey := range tenant.APIKeys {
			seeded, err := store.SeedAPIKeyFromYAML(ctx, tenant.ID, apiKey)
			if err != nil {
				return fmt.Errorf("seed api key for tenant %s: %w", tenant.ID, err)
			}
			if seeded {
				log.InfoContext(ctx, "api key seeded from YAML", "tenant_id", tenant.ID)
			}
		}
	}
	return nil
}

// BootstrapTenantConfigs seeds all YAML-defined tenants into the database on startup.
// It is idempotent (ON CONFLICT DO NOTHING) and HA-safe: multiple pods can run this
// concurrently without creating duplicates or config_change_log entries.
// Returns an error if any seed operation fails; the caller should treat this as fatal.
func BootstrapTenantConfigs(ctx context.Context, cfg *config.Config, store storage.Storage, log *slog.Logger) error {
	for _, tenant := range cfg.Tenants {
		configForDB := stripAPIKeys(tenant)
		configJSON, err := json.Marshal(configForDB)
		if err != nil {
			return fmt.Errorf("marshal tenant config %s: %w", tenant.ID, err)
		}
		seeded, err := store.SeedTenantConfig(ctx, tenant.ID, configJSON)
		if err != nil {
			return fmt.Errorf("seed tenant config %s: %w", tenant.ID, err)
		}
		if seeded {
			log.InfoContext(ctx, "tenant config seeded", "tenant_id", tenant.ID, "version", 0)
		}
	}
	return nil
}

// BootstrapAdminAPIKey ensures the GATEWAY_ADMIN_API_KEY from env exists in the database
// and ALWAYS matches the active DB key. This guarantees deterministic behavior:
// after restart, DB active key MUST match env key.
//
// Behavior:
// - If GATEWAY_ADMIN_API_KEY is not set, does nothing (skip).
// - If set, compares the env key with the active DB key:
//   * Match: log confirmation, done
//   * Mismatch: automatically rotate DB key to match env
//   * Not found: create new key
// - Plaintext key is NEVER printed (only hash and prefix).
// - Prefix derived from plaintext (first part before hash)
//
// Returns an error only for unexpected DB failures.
func BootstrapAdminAPIKey(ctx context.Context, store storage.Storage, log *slog.Logger) error {
	adminKey := os.Getenv("GATEWAY_ADMIN_API_KEY")
	if adminKey == "" {
		log.DebugContext(ctx, "GATEWAY_ADMIN_API_KEY not set, skipping admin api key bootstrap")
		return nil
	}

	// Extract prefix from env key for logging (first part before the hash suffix)
	envKeyPrefix := extractKeyPrefix(adminKey)
	log.InfoContext(ctx, "bootstrap admin api key started", "env_key_prefix", envKeyPrefix)

	// Hash the env key
	keyHash := hashAdminAPIKey(adminKey)

	// Cast store to required interfaces
	type fullAPIKeyer interface {
		ListAPIKeys(context.Context, string) ([]storage.APIKeyMeta, error)
		RotateAPIKey(context.Context, string, uuid.UUID, string, []string) (uuid.UUID, storage.APIKeyCreateResult, error)
		CreateAPIKey(context.Context, string, string, []string, *time.Time, string, []string) (storage.APIKeyCreateResult, error)
		CreateAPIKeyFromPlaintext(context.Context, string, string, string, []string, *time.Time, string, []string) (storage.APIKeyCreateResult, error)
		LookupAPIKeyByHash(context.Context, string) (storage.APIKeyRecord, bool, error)
		RevokeAPIKey(context.Context, string, uuid.UUID, string, []string) (*time.Time, error)
	}

	fullStore, ok := store.(fullAPIKeyer)
	if !ok {
		log.DebugContext(ctx, "store does not support full API key operations, skipping admin api key bootstrap")
		return nil
	}

	tenantID := "system:admin"
	const bootstrapKeyName = "bootstrap-admin"
	scopes := []string{"admin_read", "admin_write"}

	// 1. Check if the EXACT env key (by hash) already exists in DB
	existingByHash, found, err := fullStore.LookupAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return fmt.Errorf("lookup admin api key by hash: %w", err)
	}

	log.DebugContext(ctx, "bootstrap admin api key lookup by hash",
		"env_key_prefix", envKeyPrefix,
		"hash_found", found,
		"hash_first_12chars", keyHash[:12],
	)

	if found && existingByHash.RevokedAt == nil {
		// Perfect match: env key is already in DB and active
		log.InfoContext(ctx, "bootstrap admin api key already active (no action needed)",
			"tenant", tenantID,
			"name", bootstrapKeyName,
			"env_key_prefix", envKeyPrefix,
			"db_key_prefix", existingByHash.Prefix,
			"action", "skip",
		)
		return nil
	}

	// 2. Check if there's an existing active bootstrap-admin key in DB (by name)
	existingKeys, err := fullStore.ListAPIKeys(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list admin api keys: %w", err)
	}

	var existingBootstrapKey *storage.APIKeyMeta
	for i := range existingKeys {
		if existingKeys[i].Name == bootstrapKeyName && existingKeys[i].RevokedAt == nil {
			existingBootstrapKey = &existingKeys[i]
			break
		}
	}

	if existingBootstrapKey != nil {
		// There's an existing active key with the same name but different hash
		// This means env was changed or DB is out of sync with env
		// ALWAYS revoke old key and install env key (deterministic behavior)
		log.WarnContext(ctx, "admin api key mismatch: DB key does not match env key, replacing",
			"env_key_prefix", envKeyPrefix,
			"db_key_prefix", existingBootstrapKey.Prefix,
			"db_key_id", existingBootstrapKey.ID.String(),
		)

		// Revoke the old key
		type revoker interface {
			RevokeAPIKey(context.Context, string, uuid.UUID, string, []string) (*time.Time, error)
		}

		revokeStore, ok := store.(revoker)
		if ok {
			_, err := revokeStore.RevokeAPIKey(ctx, tenantID, existingBootstrapKey.ID, "bootstrap", []string{"system"})
			if err != nil {
				return fmt.Errorf("revoke old admin api key: %w", err)
			}
			log.DebugContext(ctx, "revoked mismatched admin api key", "old_id", existingBootstrapKey.ID.String())
		}

		// Create new key with env plaintext
		result, err := fullStore.CreateAPIKeyFromPlaintext(ctx, tenantID, bootstrapKeyName, adminKey, scopes, nil, "bootstrap", []string{"system"})
		if err != nil {
			return fmt.Errorf("create new admin api key from plaintext: %w", err)
		}

		log.InfoContext(ctx, "bootstrap admin api key replaced (env key installed)",
			"tenant", tenantID,
			"name", bootstrapKeyName,
			"old_key_id", existingBootstrapKey.ID.String(),
			"new_key_id", result.ID,
			"old_prefix", existingBootstrapKey.Prefix,
			"new_prefix", result.Prefix,
			"env_key_prefix", envKeyPrefix,
			"action", "replace",
		)
		return nil
	}

	// 3. No existing key found by name; create new one from env plaintext
	log.InfoContext(ctx, "bootstrap admin api key creating",
		"tenant", tenantID,
		"name", bootstrapKeyName,
		"env_key_prefix", envKeyPrefix,
		"action", "create",
	)

	result, err := fullStore.CreateAPIKeyFromPlaintext(ctx, tenantID, bootstrapKeyName, adminKey, scopes, nil, "bootstrap", []string{"system"})
	if err != nil {
		return fmt.Errorf("create admin api key from plaintext: %w", err)
	}

	log.InfoContext(ctx, "bootstrap admin api key created",
		"tenant", tenantID,
		"name", bootstrapKeyName,
		"key_id", result.ID,
		"prefix", result.Prefix,
		"env_key_prefix", envKeyPrefix,
		"action", "create",
	)

	return nil
}

// extractKeyPrefix extracts the visible prefix from an API key (first 12 chars after rk_live_)
// for safe logging without exposing the full key
func extractKeyPrefix(key string) string {
	// API keys are typically: rk_live_<prefix><hash>
	// We extract first 12 chars after the prefix marker for logging
	if len(key) < 20 {
		return "invalid_key" // too short to be valid
	}
	// Return first 12 chars (safe to log)
	if len(key) > 20 {
		return key[:20]
	}
	return key
}

// hashAdminAPIKey computes SHA256 hash of plaintext key
func hashAdminAPIKey(plaintext string) string {
	hash := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(hash[:])
}
