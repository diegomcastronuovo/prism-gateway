import { useQuery } from '@tanstack/react-query'

export interface BenchmarkModelAggregate {
  model: string
  provider: string
  avg_latency_ms: number
  p95_latency_ms: number
  success_rate: number
  avg_cost_usd: number
  samples: number
}

function parseErrorMessage(body: unknown): string {
  if (!body || typeof body !== 'object') return 'Failed to fetch benchmark model aggregates'
  const e = (body as { error?: unknown }).error
  if (typeof e === 'string') return e
  if (e && typeof e === 'object' && 'message' in e) {
    return String((e as { message?: string }).message ?? 'Failed to fetch benchmark model aggregates')
  }
  return 'Failed to fetch benchmark model aggregates'
}

async function fetchBenchmarkModels(windowHours: number): Promise<BenchmarkModelAggregate[]> {
  const response = await fetch(`/api/benchmarks/models?window_hours=${windowHours}`, {
    credentials: 'include',
    cache: 'no-store',
  })

  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    const err = new Error(parseErrorMessage(body)) as Error & { status?: number }
    err.status = response.status
    throw err
  }

  const data = await response.json()
  return data.data || []
}

export function useBenchmarkModels(windowHours: number = 24, enabled = true) {
  return useQuery({
    queryKey: ['benchmarkModels', windowHours],
    queryFn: () => fetchBenchmarkModels(windowHours),
    enabled,
    gcTime: 0,
    staleTime: 0,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 3
    },
  })
}
