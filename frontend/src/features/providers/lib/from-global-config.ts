import type { GlobalConfig } from '@/features/global-config/api/use-global-config'
import type { ProviderWithVersion } from '@/features/providers/api/use-providers'

function asRecord(raw: unknown): Record<string, unknown> {
  return raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {}
}

function getStr(pc: Record<string, unknown>, ...keys: string[]): string | undefined {
  for (const k of keys) {
    const v = pc[k]
    if (typeof v === 'string' && v.length > 0) return v
  }
  return undefined
}

function getNum(pc: Record<string, unknown>, ...keys: string[]): number | undefined {
  for (const k of keys) {
    const v = pc[k]
    if (typeof v === 'number' && !Number.isNaN(v)) return v
  }
  return undefined
}

/** Go: nil Enabled means enabled */
function getEnabled(pc: Record<string, unknown>): boolean {
  const e = pc.Enabled ?? pc.enabled
  if (e === null || e === undefined) return true
  return Boolean(e)
}

function normalizeApiKeySource(raw: unknown): 'env' | 'stored' | 'missing' {
  const s = typeof raw === 'string' ? raw.toLowerCase() : ''
  if (s === 'env' || s === 'environment') return 'env'
  if (s === 'stored' || s === 'global_config' || s === 'global config') return 'stored'
  if (s === 'missing' || s === 'not_configured' || s === '') return 'missing'
  return 'missing'
}

function deriveStatus(
  enabled: boolean,
  hasApiKey: boolean,
  source: 'env' | 'stored' | 'missing'
): ProviderWithVersion['status'] {
  if (!enabled) return 'disabled'
  if (!hasApiKey && source === 'missing') return 'missing_credentials'
  if (!hasApiKey) return 'missing_credentials'
  return 'ready'
}

/** AWS Bedrock: GET may omit secret; Ready when type is set and access key + region are non-empty (secret is required on save, not always visible in JSON). */
function isAwsBedrockReady(
  type: string,
  awsAccessKeyId: string | undefined,
  awsRegion: string | undefined
): boolean {
  return type === 'aws_bedrock' && Boolean(awsAccessKeyId?.trim()) && Boolean(awsRegion?.trim())
}

/**
 * Maps one entry under `globalConfig.config.providers[id]` into the Providers UI model.
 * Reads both PascalCase (Go JSON) and snake_case (normalized) keys.
 */
export function providerEntryToViewModel(
  id: string,
  entry: unknown,
  documentVersion: number
): ProviderWithVersion {
  const pc = asRecord(entry)

  const enabled = getEnabled(pc)
  const type = getStr(pc, 'Type', 'type') || id
  const baseUrl = getStr(pc, 'BaseURL', 'base_url')

  const awsAccessKeyId = getStr(pc, 'AwsAccessKeyID', 'aws_access_key_id')
  const awsRegion = getStr(pc, 'AwsRegion', 'aws_region')
  const awsSecretInMap = Boolean(getStr(pc, 'AwsSecretAccessKey', 'aws_secret_access_key')?.trim())

  if (type === 'aws_bedrock' || id === 'bedrock') {
    const normalizedBedrockType = 'aws_bedrock'
    const bedrockReady = isAwsBedrockReady(normalizedBedrockType, awsAccessKeyId, awsRegion)
    const apiKeySource: 'env' | 'stored' | 'missing' = bedrockReady ? 'stored' : 'missing'
    return {
      id,
      type: normalizedBedrockType,
      enabled,
      has_api_key: bedrockReady,
      api_key_source: apiKeySource,
      base_url: baseUrl,
      status: deriveStatus(enabled, bedrockReady, apiKeySource),
      api_version: getStr(pc, 'ApiVersion', 'api_version'),
      organization: getStr(pc, 'Organization', 'organization'),
      project: getStr(pc, 'Project', 'project'),
      timeout_ms: getNum(pc, 'TimeoutMs', 'timeout_ms'),
      max_retries: getNum(pc, 'MaxRetries', 'max_retries'),
      last_credential_update: getStr(pc, 'LastCredentialUpdate', 'last_credential_update'),
      version: documentVersion,
      aws_access_key_id: awsAccessKeyId,
      aws_region: awsRegion,
      aws_secret_configured: awsSecretInMap || bedrockReady,
    }
  }

  const hasMeta = pc.HasAPIKey ?? pc.has_api_key
  const hasApiKey =
    typeof hasMeta === 'boolean' ? hasMeta : Boolean(getStr(pc, 'APIKeyEnv', 'api_key_env'))

  let apiKeySource = normalizeApiKeySource(pc.APIKeySource ?? pc.api_key_source)
  const apiKeyEnv = getStr(pc, 'APIKeyEnv', 'api_key_env')
  if (apiKeySource === 'missing' && apiKeyEnv) apiKeySource = 'env'

  return {
    id,
    type,
    enabled,
    has_api_key: hasApiKey,
    api_key_source: apiKeySource,
    base_url: baseUrl,
    status: deriveStatus(enabled, hasApiKey, apiKeySource),
    api_version: getStr(pc, 'ApiVersion', 'api_version'),
    organization: getStr(pc, 'Organization', 'organization'),
    project: getStr(pc, 'Project', 'project'),
    timeout_ms: getNum(pc, 'TimeoutMs', 'timeout_ms'),
    max_retries: getNum(pc, 'MaxRetries', 'max_retries'),
    last_credential_update: getStr(pc, 'LastCredentialUpdate', 'last_credential_update'),
    version: documentVersion,
  }
}

/**
 * Builds the provider list from the active global config document (same source as PATCH).
 * AWS Bedrock is always listed so operators can configure it even when `providers.bedrock`
 * is not yet present in the stored global config (first save creates it via PATCH).
 */
export function providersFromGlobalConfig(globalConfig: GlobalConfig | undefined): ProviderWithVersion[] {
  if (!globalConfig?.config) return []
  const v = globalConfig.version
  const raw = globalConfig.config.providers
  const entries =
    raw && typeof raw === 'object' ? Object.entries(raw as Record<string, unknown>) : []

  const list = entries.map(([id, entry]) => providerEntryToViewModel(id, entry, v))

  const ids = new Set(list.map((p) => p.id))
  if (!ids.has('bedrock')) {
    list.push(providerEntryToViewModel('bedrock', {}, v))
  }

  return list.sort((a, b) => a.id.localeCompare(b.id))
}
