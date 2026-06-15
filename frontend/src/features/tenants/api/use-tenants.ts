import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'

export type TenantEnvironment = 'DEV' | 'STAGING' | 'PROD'

export interface Tenant {
  tenant_id: string
  environment?: TenantEnvironment
}

/** Tenant-level RPM limit scope (SPEC_114). Distinct from global rate_limit backend config. */
export interface TenantRateLimit {
  rpm: number
  burst: number
  scope: 'tenant' | 'api_key' | 'jwt_sub'
}

export interface TenantConfig {
  tenant_id: string
  version: number
  environment?: TenantEnvironment
  config: {
    allowed_models?: string[]
    budgets?: {
      monthly_usd?: number
      timezone?: string
    }
    hooks?: {
      global?: {
        external_pii?: ExternalPiiConfig
      }
    }
    routing?: {
      strategy?: string
      route_group?: string | null
      [key: string]: unknown
    }
    routing_strategy?: string
    route_groups?: string[]
    selection?: {
      route_groups?: Record<string, string[]>
    }
    /** Per-tenant request rate limit (not global Redis limiter config). */
    rate_limit?: TenantRateLimit | null
    budget_enforcement?: {
      enabled?: boolean
      mode?: 'report_only' | 'block' | 'degrade'
      thresholds?: {
        warn_pct?: number
        hard_pct?: number
      }
      block_status?: number
      degrade_route_group?: string
      include_cost_in_error?: boolean
      tag_budgets?: {
        enabled?: boolean
        keys?: string[]
        monthly_usd_by_tag?: Record<string, number>
      }
    }
    [key: string]: unknown
  }
}

export interface ExternalPiiConfig {
  enabled?: boolean
  request_url?: string
  response_url?: string
  timeout_ms?: number
  failure_policy?: 'accept' | 'deny'
  api_key?: string
}

async function fetchTenants(): Promise<Tenant[]> {
  const response = await fetch('/api/tenants')
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch tenants')
  }
  
  return response.json()
}

async function fetchTenantConfig(tenantId: string): Promise<TenantConfig> {
  const response = await fetch(`/api/tenants/${tenantId}`)
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch tenant config')
  }
  
  return response.json()
}

async function createTenant(params: {
  tenant_id: string
  environment: TenantEnvironment
}): Promise<{ message: string; tenant_id: string; version: number }> {
  const response = await fetch('/api/tenants', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tenant_id: params.tenant_id, environment: params.environment }),
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to create tenant')
  }
  
  return response.json()
}

async function updateTenantBudget(params: {
  tenantId: string
  monthly_usd?: number
  timezone?: string
  version: number
}): Promise<{ message: string; tenant_id: string; version: number }> {
  const response = await fetch(`/api/tenants/${params.tenantId}/budget`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      monthly_usd: params.monthly_usd,
      timezone: params.timezone,
      version: params.version,
    }),
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update tenant budget')
  }
  
  return response.json()
}

async function deleteTenant(tenantId: string): Promise<void> {
  const response = await fetch(`/api/tenants/${tenantId}`, {
    method: 'DELETE',
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to delete tenant')
  }
}

export function useTenants() {
  return useQuery({
    queryKey: ['tenants'],
    queryFn: fetchTenants,
  })
}

export function useTenantConfig(tenantId: string | null) {
  return useQuery({
    queryKey: ['tenant', tenantId],
    queryFn: () => fetchTenantConfig(tenantId!),
    enabled: !!tenantId,
  })
}

export function useCreateTenant() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: createTenant,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      toast({
        title: 'Tenant created',
        description: `Tenant "${data.tenant_id}" was created successfully.`,
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

export function useUpdateTenantBudget() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: updateTenantBudget,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['tenant', data.tenant_id] })
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      toast({
        title: 'Budget updated',
        description: `Budget for "${data.tenant_id}" was updated successfully.`,
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

export function useDeleteTenant() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: deleteTenant,
    onSuccess: (_, tenantId) => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      queryClient.removeQueries({ queryKey: ['tenant', tenantId] })
      toast({
        title: 'Tenant deleted',
        description: `Tenant "${tenantId}" was deleted successfully.`,
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

export interface TenantApiKey {
  id?: string
  name: string
  prefix: string
  scopes?: string[]
  created_at?: string
  last_used_at?: string
  expires_at?: string | null
  revoked_at?: string
}

export interface TenantUsage {
  tenant_id: string
  month: string
  requests: number
  tokens: number
  cost_usd: number
}

export interface TenantBudgetStatus {
  spend_usd: number
  budget_usd: number
  pct: number
  enforcement_mode?: string
  warn_pct?: number
  hard_pct?: number
}

async function fetchTenantApiKeys(tenantId: string): Promise<TenantApiKey[]> {
  const response = await fetch(`/api/tenants/${tenantId}/api-keys`)
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch tenant API keys')
  }
  
  const data = await response.json()
  return (data as { data?: TenantApiKey[] }).data || []
}

async function fetchTenantUsage(tenantId: string, month?: string): Promise<TenantUsage> {
  const queryParam = month ? `?month=${month}` : ''
  const response = await fetch(`/api/tenants/${tenantId}/usage${queryParam}`)
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch tenant usage')
  }
  
  return response.json()
}

async function fetchTenantBudgetStatus(tenantId: string): Promise<TenantBudgetStatus> {
  const response = await fetch(`/api/tenants/${tenantId}/budgets/status`)
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch tenant budget status')
  }
  const raw = (await response.json()) as Record<string, unknown>
  return {
    spend_usd: Number(raw.spend_usd ?? 0),
    budget_usd: Number(raw.budget_usd ?? 0),
    pct: Number(raw.pct ?? 0),
    enforcement_mode: raw.enforcement_mode == null ? undefined : String(raw.enforcement_mode),
    warn_pct: raw.warn_pct == null ? undefined : Number(raw.warn_pct),
    hard_pct: raw.hard_pct == null ? undefined : Number(raw.hard_pct),
  }
}

export function useTenantApiKeys(tenantId: string | null) {
  return useQuery({
    queryKey: ['tenantApiKeys', tenantId],
    queryFn: () => fetchTenantApiKeys(tenantId!),
    enabled: !!tenantId,
  })
}

export function useTenantUsage(tenantId: string | null, month?: string) {
  return useQuery({
    queryKey: ['tenantUsage', tenantId, month],
    queryFn: () => fetchTenantUsage(tenantId!, month),
    enabled: !!tenantId,
  })
}

export function useTenantBudgetStatus(tenantId: string | null) {
  return useQuery({
    queryKey: ['tenantBudgetStatus', tenantId],
    queryFn: () => fetchTenantBudgetStatus(tenantId!),
    enabled: !!tenantId,
  })
}

async function createApiKey(params: {
  tenantId: string
  name: string
  scopes?: string[]
  expires_at?: string | null
}): Promise<{ id: string; key: string; prefix: string; scopes?: string[] }> {
  const response = await fetch(`/api/tenants/${params.tenantId}/api-keys/create`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: params.name,
      scopes: params.scopes,
      expires_at: params.expires_at,
    }),
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to create API key')
  }
  
  return response.json()
}

async function rotateApiKey(params: {
  tenantId: string
  keyId: string
}): Promise<{ key: string; prefix: string }> {
  const response = await fetch(`/api/tenants/${params.tenantId}/api-keys/${params.keyId}/rotate`, {
    method: 'POST',
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to rotate API key')
  }
  
  return response.json()
}

async function revokeApiKey(params: {
  tenantId: string
  keyId: string
}): Promise<{ message: string }> {
  const response = await fetch(`/api/tenants/${params.tenantId}/api-keys/${params.keyId}/revoke`, {
    method: 'POST',
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to revoke API key')
  }
  
  return response.json()
}

export function useCreateTenantApiKey() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: createApiKey,
    onSuccess: (data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['tenantApiKeys', variables.tenantId] })
      toast({
        title: 'API key created',
        description: 'Your new API key has been created successfully.',
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

export function useRotateTenantApiKey() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: rotateApiKey,
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['tenantApiKeys', variables.tenantId] })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

export function useRevokeTenantApiKey() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: revokeApiKey,
    onSuccess: (data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['tenantApiKeys', variables.tenantId] })
      toast({
        title: 'API key revoked',
        description: 'The API key has been revoked successfully.',
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

async function updateTenantConfig(params: {
  tenantId: string
  version: number
  patch: Record<string, unknown>
}): Promise<{ message: string; tenant_id: string; version: number }> {
  const response = await fetch(`/api/tenants/${params.tenantId}/config`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      version: params.version,
      patch: params.patch,
    }),
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update tenant config')
  }
  
  return response.json()
}

export function useUpdateTenantConfig() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: updateTenantConfig,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['tenant', data.tenant_id] })
      toast({
        title: 'Configuration updated',
        description: `Tenant configuration was updated successfully.`,
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

export function useUpdateTenantPiiConfig() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: updateTenantConfig,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['tenant', data.tenant_id] })
      toast({
        title: 'PII configuration updated',
      })
    },
    onError: () => {
      toast({
        title: 'Failed to update PII configuration',
        variant: 'destructive',
      })
    },
  })
}
