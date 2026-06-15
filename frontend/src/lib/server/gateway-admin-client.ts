/**
 * Server-side Gateway Admin Client
 * 
 * CRITICAL: This file runs ONLY on the server.
 * It uses the admin API key which must NEVER be exposed to the browser.
 * 
 * Environment variables used:
 * - GATEWAY_BASE_URL (server-only)
 * - GATEWAY_ADMIN_API_KEY
 */

import 'server-only'
import { cookies } from 'next/headers'
import { canonicalizeProviderPatchForGlobalConfig } from '@/features/providers/api/provider-config-canonical'
import { buildAdminModelPatchPayload } from '@/lib/server/admin-model-patch'

const GATEWAY_BASE_URL = process.env.GATEWAY_BASE_URL
const GATEWAY_ADMIN_API_KEY = process.env.GATEWAY_ADMIN_API_KEY

if (!GATEWAY_ADMIN_API_KEY) {
  console.warn('⚠️  GATEWAY_ADMIN_API_KEY is not set. Backend calls will fail.')
}

export class GatewayAdminError extends Error {
  statusCode?: number
  details?: unknown

  constructor(message: string, statusCode?: number, details?: unknown) {
    super(message)
    this.name = 'GatewayAdminError'
    this.statusCode = statusCode
    this.details = details
  }
}

export type GatewayAdminFetchOptions = RequestInit & {
  /** When set, use Authorization: Bearer instead of X-API-Key (logged-in admin session). */
  requestAuthToken?: string | null
}

/**
 * Fetches from the gateway admin API. By default uses X-API-Key (GATEWAY_ADMIN_API_KEY).
 * Pass requestAuthToken to use the logged-in admin's Bearer token instead (e.g. from cookie).
 * Automatically handles 401s by refreshing the token and retrying.
 */
export async function gatewayAdminFetch(path: string, options: GatewayAdminFetchOptions = {}) {
  const { requestAuthToken, ...init } = options
  // Mock session sentinel → bypass Bearer, fall back to GATEWAY_ADMIN_API_KEY.
  const useBearer =
    typeof requestAuthToken === 'string' &&
    requestAuthToken.length > 0 &&
    requestAuthToken !== MOCK_SESSION_SENTINEL

  if (!GATEWAY_BASE_URL) {
    throw new GatewayAdminError('Missing GATEWAY_BASE_URL', 500)
  }
  if (!useBearer && !GATEWAY_ADMIN_API_KEY) {
    throw new GatewayAdminError('Missing GATEWAY_ADMIN_API_KEY', 500)
  }

  const authMode = useBearer ? 'bearer' : 'api_key'
  if (process.env.NODE_ENV !== 'production') {
    console.log('[gatewayAdminFetch] auth_mode=%s path=%s', authMode, path)
  }

  try {
    const res = await fetch(`${GATEWAY_BASE_URL}${path}`, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...(useBearer
          ? { Authorization: `Bearer ${requestAuthToken}` }
          : { 'X-API-Key': GATEWAY_ADMIN_API_KEY! }),
        ...init.headers,
      },
      cache: 'no-store',
    })

    // Handle 401 by refreshing token and retrying (only for Bearer auth)
    if (res.status === 401 && useBearer) {
      if (process.env.NODE_ENV !== 'production') {
        console.log('[gatewayAdminFetch] 🔄 Got 401, attempting token refresh')
      }
      try {
        // Call refresh-server endpoint (internal to Next.js server)
        const refreshUrl = process.env.NEXTAUTH_URL
          ? `${process.env.NEXTAUTH_URL}/api/auth/refresh-server`
          : '/api/auth/refresh-server'
        const refreshRes = await fetch(refreshUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
        })
        if (refreshRes.ok) {
          const refreshData = await refreshRes.json()
          if (refreshData.ok && refreshData.access_token) {
            if (process.env.NODE_ENV !== 'production') {
              console.log('[gatewayAdminFetch] ✅ Token refreshed, retrying request')
            }
            // Retry the original request with new token
            const retryRes = await fetch(`${GATEWAY_BASE_URL}${path}`, {
              ...init,
              headers: {
                'Content-Type': 'application/json',
                Authorization: `Bearer ${refreshData.access_token}`,
                ...init.headers,
              },
              cache: 'no-store',
            })
            if (!retryRes.ok) {
              const text = await retryRes.text()
              throw new GatewayAdminError(
                `Gateway error ${retryRes.status}: ${text}`,
                retryRes.status,
                { status: retryRes.status, response: text }
              )
            }
            // Handle 204 No Content
            if (retryRes.status === 204) {
              return null
            }
            return retryRes.json()
          }
        }
      } catch (refreshError) {
        if (process.env.NODE_ENV !== 'production') {
          console.log('[gatewayAdminFetch] ❌ Token refresh failed:', refreshError)
        }
        // Fall through to original error handling
      }
    }

    if (!res.ok) {
      const text = await res.text()
      throw new GatewayAdminError(
        `Gateway error ${res.status}: ${text}`,
        res.status,
        { status: res.status, response: text }
      )
    }

    // Handle 204 No Content response (no body to parse)
    if (res.status === 204) {
      return null
    }

    return res.json()
  } catch (error) {
    // Log connection errors for debugging (without exposing secrets)
    if (error instanceof Error && error.message.includes('ECONNREFUSED')) {
      console.error('gatewayAdminFetch failed')
      console.error(`baseUrl=${GATEWAY_BASE_URL}`)
      console.error(`path=${path}`)
      console.error('error=ECONNREFUSED')
      throw new GatewayAdminError(
        'Backend unreachable. Check GATEWAY_BASE_URL or Docker port mapping.',
        503,
        { baseUrl: GATEWAY_BASE_URL, path, error: 'ECONNREFUSED' }
      )
    }
    // Re-throw other errors
    throw error
  }
}

/**
 * Sentinel returned by getAdminAuthToken for mock sessions.
 * gatewayAdminFetch detects this and uses GATEWAY_ADMIN_API_KEY instead of Bearer.
 * Remove together with mock session support when deploying to production.
 */
export const MOCK_SESSION_SENTINEL = '__mock_session__'

/**
 * Returns the admin auth token from the current request (cookie or Authorization header).
 * Returns MOCK_SESSION_SENTINEL when is_mock_session cookie is set (dev/mock mode only).
 */
export async function getAdminAuthToken(request: Request): Promise<string | null> {
  const cookieStore = await cookies()
  if (cookieStore.get('is_mock_session')?.value === '1') {
    return MOCK_SESSION_SENTINEL
  }
  const fromCookie = cookieStore.get('admin_access_token')?.value ?? null
  if (fromCookie) return fromCookie
  const authHeader = request.headers.get('authorization')
  if (authHeader?.startsWith('Bearer ')) return authHeader.slice(7).trim()
  return null
}

// ============================================================================
// Dashboard API
// ============================================================================

export interface VersionInfo {
  service: string
  backend_version: string
  git_commit: string
  build_time: string
  release_notes: string
}

export interface Tenant {
  tenant_id: string
}

export interface Model {
  id: string
  provider: string
  route_groups: string[]
}

export interface Provider {
  id: string
}

export interface RouteGroup {
  id: string
  models?: string[]
}

export interface Features {
  budget_enforcement: boolean
  dynamic_routes: boolean
  semantic_cache: boolean
  semantic_routing: boolean
}

export interface ModelBenchmark {
  model: string
  provider: string
  avg_latency_ms: number
  p95_latency_ms: number
  success_rate: number
  avg_cost_usd: number
  samples: number
}

export async function getVersion() {
  return gatewayAdminFetch('/admin/version')
}

export async function getTenants(requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/tenants', { requestAuthToken })
}

export async function getModels(requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/models', { requestAuthToken })
}

export async function getProviders(requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/providers', { requestAuthToken })
}

export async function getSystemHealth(requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/health/system', { requestAuthToken })
}

export async function getRouteGroups() {
  return gatewayAdminFetch('/admin/route-groups')
}

export async function getRouteGroupsFromTenantConfig(tenantId: string, requestAuthToken?: string | null) {
  const config = await getTenantConfig(tenantId, requestAuthToken)
  const routeGroups = config.config?.selection?.route_groups || {}
  
  // Convert map to array of RouteGroup objects
  return Object.entries(routeGroups).map(([id, models]) => ({
    id,
    models: models as string[],
  }))
}

export async function createRouteGroupInTenantConfig(
  tenantId: string,
  routeGroupId: string,
  models: string[],
  requestAuthToken?: string | null
) {
  // Single fetch: get current config and version atomically
  const config = await getTenantConfig(tenantId, requestAuthToken)
  const currentRouteGroups = config.config?.selection?.route_groups || {}
  const version = config.version || 1

  const updatedRouteGroups = {
    ...currentRouteGroups,
    [routeGroupId]: models,
  }

  return patchTenantConfigGeneric(
    tenantId,
    {
      selection: {
        route_groups: updatedRouteGroups,
      },
    },
    version,
    requestAuthToken
  )
}

export async function updateRouteGroupInTenantConfig(
  tenantId: string,
  routeGroupId: string,
  models: string[],
  requestAuthToken?: string | null
) {
  // Single fetch: get current config and version atomically
  const config = await getTenantConfig(tenantId, requestAuthToken)
  const currentRouteGroups = config.config?.selection?.route_groups || {}
  const version = config.version || 1

  const updatedRouteGroups = {
    ...currentRouteGroups,
    [routeGroupId]: models,
  }

  return patchTenantConfigGeneric(
    tenantId,
    {
      selection: {
        route_groups: updatedRouteGroups,
      },
    },
    version,
    requestAuthToken
  )
}

export async function deleteRouteGroupFromTenantConfig(
  tenantId: string,
  routeGroupId: string,
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch(
    `/admin/tenants/${encodeURIComponent(tenantId)}/route-groups/${encodeURIComponent(routeGroupId)}`,
    {
      method: 'DELETE',
      requestAuthToken,
    }
  )
}

export async function getFeatures() {
  return gatewayAdminFetch('/admin/features')
}

export async function getModelBenchmarks(
  windowHours: number = 24,
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch(`/admin/benchmarks/models?window_hours=${windowHours}`, {
    requestAuthToken,
  })
}

export async function deleteModelBenchmarks(requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/benchmarks/models', {
    method: 'DELETE',
    requestAuthToken,
  })
}

// ============================================================================
// Benchmarks API
// ============================================================================

export interface BenchmarkResult {
  model: string
  requests: number
  success_count: number
  error_count: number
  success_rate: number
  avg_latency_ms: number
  p95_latency_ms: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  estimated_cost_usd: number
}

export interface Benchmark {
  benchmark_id: string
  tenant_id: string
  created_at: string
  completed_at?: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  models: string[]
  prompts: string[]
  prompts_count: number
  avg_latency_ms?: number
  best_latency_model?: string
  best_success_model?: string
  lowest_cost_model?: string
  results?: BenchmarkResult[]
}

export interface BenchmarkSummary {
  total_benchmarks: number
  avg_latency_ms: number
  success_rate: number
}

export interface CreateBenchmarkRequest {
  tenant_id: string
  models: string[]
  prompts: string[]
}

export async function getBenchmarks(requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/benchmarks', { requestAuthToken })
}

export async function getBenchmark(benchmarkId: string, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/benchmarks/${benchmarkId}`, { requestAuthToken })
}

export async function getBenchmarkSummary(requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/benchmarks/summary', { requestAuthToken })
}

export async function createBenchmark(request: CreateBenchmarkRequest, requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/benchmarks', {
    method: 'POST',
    body: JSON.stringify(request),
    requestAuthToken,
  })
}

// ============================================================================
// Models API
// ============================================================================

export interface Model {
  id: string
  provider: string
  route_groups: string[]
  enabled?: boolean
  type?: string
  category?: string
  pricing?: {
    prompt_per_1m?: number
    completion_per_1m?: number
  }
  mock?: {
    enabled?: boolean
    delay_min_ms?: number
    delay_max_ms?: number
    error_rate?: number
    error_status?: number
    error_message?: string
    fixed_response?: string
  }
  capabilities?: {
    chat?: boolean
    embeddings?: boolean
    streaming?: boolean
    tool_calling?: boolean
  }
  [key: string]: unknown
}

export async function createModel(
  model: Omit<Model, 'id'> & { id: string },
  version: number,
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch('/admin/models', {
    method: 'POST',
    headers: {
      'If-Match-Version': version.toString(),
    },
    body: JSON.stringify(model),
    requestAuthToken,
  })
}

export async function updateModel(
  modelId: string,
  model: Partial<Model>,
  version: number,
  requestAuthToken?: string | null
) {
  const patch = buildAdminModelPatchPayload(model)
  if (process.env.NODE_ENV !== 'production') {
    console.log('[updateModel] gateway PATCH body', modelId, JSON.stringify(patch))
  }
  return gatewayAdminFetch(`/admin/models/${modelId}`, {
    method: 'PATCH',
    headers: {
      'If-Match-Version': version.toString(),
    },
    body: JSON.stringify(patch),
    requestAuthToken,
  })
}

export async function deleteModel(modelId: string, version: number, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/models/${modelId}`, {
    method: 'DELETE',
    headers: {
      'If-Match-Version': version.toString(),
    },
    requestAuthToken,
  })
}

// ============================================================================
// Tenants API
// ============================================================================

export interface TenantConfig {
  tenant_id: string
  version: number
  config: {
    allowed_models?: string[]
    budgets?: {
      monthly_usd?: number
      timezone?: string
    }
    routing_strategy?: string
    route_groups?: string[]
    [key: string]: unknown
  }
}

export interface CreateTenantRequest {
  tenant_id: string
}

export interface CreateTenantResponse {
  message: string
  tenant_id: string
  version: number
}

export interface PatchTenantConfigRequest {
  budgets?: {
    monthly_usd?: number
    timezone?: string
  }
  [key: string]: unknown
}

export interface PatchTenantConfigResponse {
  message: string
  tenant_id: string
  version: number
}

/**
 * Fetches tenant config. When requestAuthToken is provided (e.g. from logged-in admin cookie),
 * the gateway is called with Authorization: Bearer so the backend uses the admin session
 * instead of the fallback API key.
 */
export async function getTenantConfig(tenantId: string, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/config`, { requestAuthToken })
}

export async function createTenant(
  tenantId: string,
  environment: 'DEV' | 'STAGING' | 'PROD',
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch('/admin/tenants', {
    method: 'POST',
    body: JSON.stringify({ tenant_id: tenantId, environment }),
    requestAuthToken,
  })
}

export async function patchTenantConfig(
  tenantId: string,
  config: {
    monthly_usd?: number
    timezone?: string
  },
  version: number,
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/config`, {
    method: 'PATCH',
    headers: {
      'If-Match-Version': version.toString(),
    },
    body: JSON.stringify({ budgets: config }),
    requestAuthToken,
  })
}

export async function patchTenantConfigGeneric(
  tenantId: string,
  patch: Record<string, unknown>,
  version: number,
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/config`, {
    method: 'PATCH',
    headers: {
      'If-Match-Version': version.toString(),
    },
    body: JSON.stringify(patch),
    requestAuthToken,
  })
}

export async function deleteTenant(tenantId: string, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}`, {
    method: 'DELETE',
    requestAuthToken,
  })
}

export async function getTenantApiKeys(tenantId: string, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/api-keys`, { requestAuthToken })
}

export async function getTenantUsage(tenantId: string, month?: string, requestAuthToken?: string | null) {
  const queryParam = month ? `?month=${month}` : ''
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/usage${queryParam}`, { requestAuthToken })
}

export async function getTenantUsageSummary(tenantId: string, month?: string, requestAuthToken?: string | null) {
  const queryParam = month ? `?month=${month}` : ''
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/usage/summary${queryParam}`, { requestAuthToken })
}

export async function getTenantBudgetStatus(tenantId: string, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/budgets/status`, { requestAuthToken })
}

export async function getGlobalConfig(requestAuthToken?: string | null) {
  return gatewayAdminFetch('/admin/config/global', { requestAuthToken })
}

export async function patchGlobalConfig(
  config: Record<string, unknown>,
  version: number,
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch('/admin/config/global', {
    method: 'PATCH',
    headers: {
      'If-Match-Version': version.toString(),
    },
    body: JSON.stringify(config),
    requestAuthToken,
  })
}

export async function getGlobalConfigChanges(limit = 50, offset = 0, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/config/global/changes?limit=${limit}&offset=${offset}`, { requestAuthToken })
}

export async function createTenantApiKey(
  tenantId: string,
  data: { name: string; scopes?: string[]; expires_at?: string | null },
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/api-keys`, {
    method: 'POST',
    body: JSON.stringify(data),
    requestAuthToken,
  })
}

export async function rotateTenantApiKey(tenantId: string, keyId: string, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/api-keys/${keyId}/rotate`, {
    method: 'POST',
    requestAuthToken,
  })
}

export async function revokeTenantApiKey(tenantId: string, keyId: string, requestAuthToken?: string | null) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/api-keys/${keyId}/revoke`, {
    method: 'POST',
    requestAuthToken,
  })
}

export async function testTenantPiiConnection(
  tenantId: string,
  payload: { request_url: string; response_url: string; timeout_ms: number; api_key?: string },
  requestAuthToken?: string | null
) {
  return gatewayAdminFetch(`/admin/tenants/${tenantId}/pii/test-connection`, {
    method: 'POST',
    body: JSON.stringify(payload),
    requestAuthToken,
  })
}

// ============================================================================
// Providers API (via Global Config)
// ============================================================================

export interface ProviderConfig {
  type?: string
  enabled?: boolean
  base_url?: string
  api_version?: string
  organization?: string
  project?: string
  timeout_ms?: number
  max_retries?: number
  has_api_key?: boolean
  api_key_source?: 'global_config' | 'env_fallback' | 'not_configured'
  last_credential_update?: string
  custom_headers?: Record<string, string>
  aws_access_key_id?: string
  aws_secret_access_key?: string
  aws_region?: string
  [key: string]: unknown
}

export interface ProvidersConfig {
  openai?: ProviderConfig
  anthropic?: ProviderConfig
  gemini?: ProviderConfig
  xai?: ProviderConfig
  local?: ProviderConfig
  [key: string]: ProviderConfig | undefined
}

export async function getProvidersConfig() {
  const globalConfig = await getGlobalConfig()
  return (globalConfig.config?.providers || {}) as ProvidersConfig
}

export async function getConversationDialog(
  conversationId: string,
  workflowId: string,
  tenantId: string,
  token?: string | null
) {
  return gatewayAdminFetch(
    `/admin/conversations/${encodeURIComponent(conversationId)}/dialog?workflow_id=${encodeURIComponent(workflowId)}&tenant_id=${encodeURIComponent(tenantId)}`,
    { requestAuthToken: token }
  )
}

export async function patchProviderConfig(
  providerId: string,
  patch: Partial<ProviderConfig>,
  version: number,
  requestAuthToken?: string | null
) {
  const canonical = canonicalizeProviderPatchForGlobalConfig(patch as Record<string, unknown>)
  return gatewayAdminFetch('/admin/config/global', {
    method: 'PATCH',
    headers: {
      'If-Match-Version': version.toString(),
    },
    body: JSON.stringify({
      providers: {
        [providerId]: canonical
      }
    }),
    requestAuthToken,
  })
}

export async function updateProviderCredentials(
  providerId: string,
  credentials:
    | { api_key?: string; api_secret?: string; organization?: string }
    | { aws_access_key_id: string; aws_secret_access_key: string; aws_region: string },
  version: number,
  requestAuthToken?: string | null
) {
  if ('aws_access_key_id' in credentials) {
    const providerPatch = canonicalizeProviderPatchForGlobalConfig({
      type: 'aws_bedrock',
      aws_access_key_id: credentials.aws_access_key_id,
      aws_secret_access_key: credentials.aws_secret_access_key,
      aws_region: credentials.aws_region,
    })
    return gatewayAdminFetch('/admin/config/global', {
      method: 'PATCH',
      headers: {
        'If-Match-Version': version.toString(),
      },
      body: JSON.stringify({
        providers: {
          [providerId]: providerPatch,
        },
      }),
      requestAuthToken,
    })
  }

  const providerPatch = canonicalizeProviderPatchForGlobalConfig({
    credentials,
    has_api_key: !!credentials.api_key,
    api_key_source: 'global_config',
    last_credential_update: new Date().toISOString(),
  })
  return gatewayAdminFetch('/admin/config/global', {
    method: 'PATCH',
    headers: {
      'If-Match-Version': version.toString(),
    },
    body: JSON.stringify({
      providers: {
        [providerId]: providerPatch,
      },
    }),
    requestAuthToken,
  })
}
