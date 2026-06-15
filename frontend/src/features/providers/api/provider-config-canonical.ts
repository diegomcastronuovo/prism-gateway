/**
 * Global config stores provider overrides using the same JSON field names as Go's
 * encoding/json for config.ProviderConfig (PascalCase: BaseURL, Enabled, Type, APIKeyEnv).
 *
 * Patches must not mix snake_case aliases (base_url, enabled) with those keys, or merge
 * patches accumulate duplicate fields in the stored JSON.
 *
 * @see bugs/bug_fe_providers_wrong_field_naming.md
 */

/** snake_case / runtime-style aliases → Go JSON field names for provider config */
const SNAKE_TO_PASCAL: Record<string, string> = {
  base_url: 'BaseURL',
  enabled: 'Enabled',
  type: 'Type',
  api_key_env: 'APIKeyEnv',
  api_version: 'ApiVersion',
  organization: 'Organization',
  project: 'Project',
  timeout_ms: 'TimeoutMs',
  max_retries: 'MaxRetries',
  has_api_key: 'HasAPIKey',
  api_key_source: 'APIKeySource',
  last_credential_update: 'LastCredentialUpdate',
}

/**
 * Build a provider merge-patch that uses only canonical keys: snake_case aliases are
 * mapped to PascalCase; explicit PascalCase wins over a conflicting snake alias.
 */
export function canonicalizeProviderPatchForGlobalConfig(
  patch: Record<string, unknown>
): Record<string, unknown> {
  const result: Record<string, unknown> = {}

  // Lower-precedence: snake_case aliases
  for (const [k, v] of Object.entries(patch)) {
    if (v === undefined) continue
    const pascal = SNAKE_TO_PASCAL[k]
    if (pascal) result[pascal] = v
  }

  // Higher-precedence: explicit PascalCase keys and unknown keys (e.g. nested objects)
  for (const [k, v] of Object.entries(patch)) {
    if (v === undefined) continue
    if (SNAKE_TO_PASCAL[k]) continue
    result[k] = v
  }

  return result
}
