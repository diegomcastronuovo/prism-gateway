import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'
import type { GlobalConfig } from '@/features/global-config/api/use-global-config'

// New backend schema for provider runtime metadata
export interface Provider {
  id: string
  type: string
  enabled: boolean
  has_api_key: boolean
  api_key_source: 'env' | 'stored' | 'missing'
  base_url?: string
  status: 'ready' | 'missing_credentials' | 'disabled'
  // Legacy fields kept for backward compatibility
  api_version?: string
  organization?: string
  project?: string
  timeout_ms?: number
  max_retries?: number
  last_credential_update?: string
  custom_headers?: Record<string, string>
  /** AWS Bedrock (global config) */
  aws_access_key_id?: string
  aws_region?: string
  /** True when secret is present in JSON or credentials are considered complete for read-only UI */
  aws_secret_configured?: boolean
  [key: string]: unknown
}

export interface ProviderWithVersion extends Provider {
  version: number
}

// Fetch all providers
async function fetchProviders(): Promise<Provider[]> {
  const response = await fetch('/api/providers', { cache: 'no-store', credentials: 'include' })

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({}))
    const msg =
      typeof errorBody.error === 'string' ? errorBody.error : 'Failed to fetch providers'
    const err = new Error(msg) as Error & { status?: number }
    err.status = response.status
    throw err
  }

  const data = await response.json()
  return data.data || []
}

// Fetch single provider with version
async function fetchProvider(providerId: string): Promise<ProviderWithVersion> {
  const response = await fetch(`/api/providers/${providerId}`, { cache: 'no-store', credentials: 'include' })

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({}))
    const msg = typeof errorBody.error === 'string' ? errorBody.error : 'Failed to fetch provider'
    const err = new Error(msg) as Error & { status?: number }
    err.status = response.status
    throw err
  }

  const data = await response.json()
  return { ...data.data, version: data.version }
}

// Update provider config
async function updateProvider(
  providerId: string,
  config: Partial<Provider>,
  version: number
): Promise<{ message: string; version: number }> {
  if (process.env.NODE_ENV !== 'production') {
    console.log('[providers] PATCH version:', version, 'providerId:', providerId)
  }
  const response = await fetch(`/api/providers/${providerId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ config, version }),
    cache: 'no-store',
    credentials: 'include',
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update provider')
  }

  return response.json()
}

// Update provider credentials
export type ProviderCredentialsPayload =
  | { api_key?: string; api_secret?: string; organization?: string }
  | {
      aws_access_key_id: string
      aws_secret_access_key: string
      aws_region: string
    }

async function updateProviderCredentials(
  providerId: string,
  credentials: ProviderCredentialsPayload,
  version: number
): Promise<{ message: string; version: number }> {
  if (process.env.NODE_ENV !== 'production') {
    console.log('[providers] POST credentials version:', version, 'providerId:', providerId)
  }
  const response = await fetch(`/api/providers/${providerId}/credentials`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ credentials, version }),
    cache: 'no-store',
    credentials: 'include',
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update credentials')
  }

  return response.json()
}

// Hook to get all providers
export function useProviders() {
  return useQuery({
    queryKey: ['providers'],
    queryFn: fetchProviders,
    /** Overrides global staleTime so invalidation always drives a fresh fetch after mutations */
    staleTime: 0,
    gcTime: 0,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
  })
}

// Hook to get single provider
export function useProvider(providerId: string | null) {
  return useQuery({
    queryKey: ['provider', providerId],
    queryFn: () => fetchProvider(providerId!),
    enabled: !!providerId,
    staleTime: 0,
    gcTime: 0,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
  })
}

// Hook to update provider config
export function useUpdateProvider() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: ({ providerId, config, version }: { providerId: string; config: Partial<Provider>; version: number }) =>
      updateProvider(providerId, config, version),
    onSuccess: async (data, variables) => {
      const newVersion = typeof (data as { version?: number })?.version === 'number' ? (data as { version: number }).version : undefined
      if (newVersion !== undefined) {
        queryClient.setQueryData<GlobalConfig>(['globalConfig'], (old) =>
          old ? { ...old, version: newVersion } : old
        )
      }
      await queryClient.invalidateQueries({
        queryKey: ['providers'],
        refetchType: 'all',
      })
      await queryClient.invalidateQueries({
        queryKey: ['provider', variables.providerId],
        refetchType: 'all',
      })
      // Providers page reads config via useGlobalConfig — must refetch after PATCH
      await queryClient.invalidateQueries({
        queryKey: ['globalConfig'],
        refetchType: 'all',
      })
      await queryClient.invalidateQueries({
        queryKey: ['globalConfigChanges'],
        refetchType: 'all',
      })
      toast({
        title: 'Success',
        description: 'Provider updated successfully',
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

// Hook to update provider credentials
export function useUpdateProviderCredentials() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: ({
      providerId,
      credentials,
      version
    }: {
      providerId: string;
      credentials: ProviderCredentialsPayload;
      version: number
    }) => updateProviderCredentials(providerId, credentials, version),
    onSuccess: async (data, variables) => {
      const newVersion = typeof (data as { version?: number })?.version === 'number' ? (data as { version: number }).version : undefined
      if (newVersion !== undefined) {
        queryClient.setQueryData<GlobalConfig>(['globalConfig'], (old) =>
          old ? { ...old, version: newVersion } : old
        )
      }
      await queryClient.invalidateQueries({
        queryKey: ['providers'],
        refetchType: 'all',
      })
      await queryClient.invalidateQueries({
        queryKey: ['provider', variables.providerId],
        refetchType: 'all',
      })
      // Providers page reads config via useGlobalConfig — must refetch after update
      await queryClient.invalidateQueries({
        queryKey: ['globalConfig'],
        refetchType: 'all',
      })
      await queryClient.invalidateQueries({
        queryKey: ['globalConfigChanges'],
        refetchType: 'all',
      })
      toast({
        title: 'Success',
        description: 'Credentials updated successfully',
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
