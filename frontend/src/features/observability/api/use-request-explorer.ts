import { useMutation, useQuery } from '@tanstack/react-query'
import { parseObservabilityErrorResponse } from './parse-observability-response'
import type {
  RequestExplorerData,
  RequestExplorerFilters,
  RequestReplayResult,
  RequestRoutingSnapshot,
} from '../types/request-explorer'

export function useRequestExplorer(
  filters: RequestExplorerFilters,
  options?: { enabled?: boolean }
) {
  return useQuery<RequestExplorerData>({
    queryKey: ['request-explorer', filters],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (filters.tenant_id) params.append('tenant_id', filters.tenant_id)
      if (filters.model) params.append('model', filters.model)
      if (filters.provider) params.append('provider', filters.provider)
      if (filters.status) params.append('status', filters.status)
      if (filters.fallback_used !== undefined) params.append('fallback_used', String(filters.fallback_used))
      if (filters.time_range) params.append('time_range', String(filters.time_range))
      // SPEC_143: workflow/conversation drill-down
      if (filters.workflow_id)     params.append('workflow_id',     filters.workflow_id)
      if (filters.conversation_id) params.append('conversation_id', filters.conversation_id)
      params.append('limit', String(filters.limit ?? 50))
      params.append('offset', String(filters.offset ?? 0))

      const res = await fetch(`/api/observability/request-explorer?${params.toString()}`)
      if (!res.ok) throw new Error('Failed to fetch request explorer data')
      return res.json()
    },
    enabled: options?.enabled !== false,
    refetchInterval: 10000, // Refresh every 10 seconds
  })
}

export function useRequestRoutingSnapshot(requestId: string | null, enabled = true) {
  return useQuery<RequestRoutingSnapshot | null>({
    queryKey: ['request-routing-snapshot', requestId],
    enabled: Boolean(requestId) && enabled,
    queryFn: async () => {
      if (!requestId) return null
      const res = await fetch(`/api/observability/request-explorer/${encodeURIComponent(requestId)}/routing`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (res.status === 404) return null
      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Failed to fetch routing snapshot')
      }
      return res.json()
    },
    staleTime: 30_000,
  })
}

export function useReplayRequest() {
  return useMutation<RequestReplayResult, Error, string>({
    mutationFn: async (requestId: string) => {
      const res = await fetch(`/api/observability/request-explorer/${encodeURIComponent(requestId)}/replay`, {
        method: 'POST',
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Replay failed.')
      }
      return res.json()
    },
  })
}
