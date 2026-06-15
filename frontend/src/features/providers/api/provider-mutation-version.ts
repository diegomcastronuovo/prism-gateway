import type { QueryClient } from '@tanstack/react-query'
import type { GlobalConfig } from '@/features/global-config/api/use-global-config'

/**
 * PATCH provider mutates global config; version must match the latest global config document.
 * Prefer React Query cache from GET /api/global-config, fall back to provider detail version
 * (from GET /api/providers/:id, which resolves version from global config on the server).
 */
export function getVersionForProviderMutation(queryClient: QueryClient, fallback: number): number {
  const cached = queryClient.getQueryData<GlobalConfig>(['globalConfig'])
  if (typeof cached?.version === 'number') return cached.version
  return fallback
}
