import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'

export interface GlobalConfig {
  version: number
  config: Record<string, unknown>
}

export interface GlobalConfigChange {
  version: number
  changed_at: string
  changed_by?: string
  summary?: string
}

// SPEC_63: Helper to get runtime endpoint from localStorage
function getRuntimeEndpoint(): string | null {
  if (typeof window === 'undefined') return null
  try {
    const raw =
      window.localStorage.getItem('router_api_url') ||
      window.localStorage.getItem('gateway_api_endpoint')
    if (!raw) return null
    const value = raw.trim().replace(/\/+$/, '')
    if (!value) return null
    if (/^\d+$/.test(value)) return `http://localhost:${value}`
    if (value.startsWith('localhost:') || value.startsWith('127.0.0.1:')) {
      return `http://${value}`
    }
    if (!/^https?:\/\//i.test(value)) return `http://${value}`
    return value
  } catch {
    return null
  }
}

// SPEC_63: Helper to add runtime endpoint header
function getHeadersWithEndpoint(): HeadersInit {
  const headers: HeadersInit = { 'Content-Type': 'application/json' }
  const endpoint = getRuntimeEndpoint()
  if (endpoint) {
    headers['x-runtime-endpoint'] = endpoint
  }
  return headers
}

async function fetchGlobalConfig(): Promise<GlobalConfig> {
  const response = await fetch('/api/global-config', {
    headers: getHeadersWithEndpoint(),
    cache: 'no-store',
    credentials: 'include',
  })

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({}))
    const msg =
      typeof errorBody.error === 'string' ? errorBody.error : 'Failed to fetch global config'
    const err = new Error(msg) as Error & { status?: number }
    err.status = response.status
    throw err
  }

  return response.json()
}

async function updateGlobalConfig(params: {
  config: Record<string, unknown>
  version: number
}): Promise<{ message: string; version: number }> {
  const response = await fetch('/api/global-config', {
    method: 'PATCH',
    headers: getHeadersWithEndpoint(),
    body: JSON.stringify(params),
    cache: 'no-store',
    credentials: 'include',
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update global config')
  }
  
  return response.json()
}

async function fetchGlobalConfigChanges(limit = 50, offset = 0): Promise<{
  object: string
  data: GlobalConfigChange[]
  has_more: boolean
}> {
  const response = await fetch(`/api/global-config/changes?limit=${limit}&offset=${offset}`, {
    headers: getHeadersWithEndpoint(),
    cache: 'no-store',
    credentials: 'include',
  })

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({}))
    const msg =
      typeof errorBody.error === 'string' ? errorBody.error : 'Failed to fetch global config changes'
    const err = new Error(msg) as Error & { status?: number }
    err.status = response.status
    throw err
  }
  
  return response.json()
}

export function useGlobalConfig(enabled = true) {
  return useQuery({
    queryKey: ['globalConfig'],
    queryFn: fetchGlobalConfig,
    enabled,
    /** Match FinOps RBAC: no retry storm on 401/403; avoid long-lived stale cache on deny */
    gcTime: 0,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
  })
}

export function useUpdateGlobalConfig() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: updateGlobalConfig,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['globalConfig'] })
      queryClient.invalidateQueries({ queryKey: ['globalConfigChanges'] })
      toast({
        title: 'Global config updated',
        description: `Configuration updated to version ${data.version}.`,
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

export function useGlobalConfigChanges(limit = 50, offset = 0, enabled = true) {
  return useQuery({
    queryKey: ['globalConfigChanges', limit, offset],
    queryFn: () => fetchGlobalConfigChanges(limit, offset),
    enabled,
    gcTime: 0,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      if (error instanceof Error && error.message.includes('404')) {
        return false
      }
      return failureCount < 3
    },
  })
}
