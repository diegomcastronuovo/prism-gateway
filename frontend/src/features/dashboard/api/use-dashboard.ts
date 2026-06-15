import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@/hooks/use-auth'

export interface DashboardData {
  version: {
    service: string
    backend_version: string
    git_commit: string
    build_time: string
    release_notes: string
  }
  tenants: Array<{ tenant_id: string }>
  models: Array<{
    id: string
    provider: string
    route_groups: string[]
  }>
  providers: Array<{ id: string }>
  routeGroups: Array<{ id: string }>
  features: {
    budget_enforcement: boolean
    dynamic_routes: boolean
    semantic_cache: boolean
    semantic_routing: boolean
  }
  benchmarks: Array<{
    model: string
    provider: string
    avg_latency_ms: number
    p95_latency_ms: number
    success_rate: number
    avg_cost_usd: number
    samples: number
  }>
}

async function fetchDashboard(ensureValid: () => Promise<boolean>): Promise<DashboardData> {
  // SPEC_84: Ensure session is valid before protected request
  const isValid = await ensureValid()
  if (!isValid) {
    if (process.env.NODE_ENV !== 'production') {
      console.log('[Dashboard] ❌ Session invalid, cannot fetch')
    }
    throw new Error('Session expired')
  }
  
  if (process.env.NODE_ENV !== 'production') {
    console.log('[Dashboard] 📊 Fetching dashboard data')
  }
  
  const response = await fetch('/api/dashboard')
  
  if (!response.ok) {
    if (process.env.NODE_ENV !== 'production') {
      console.log('[Dashboard] ❌ Protected request failed:', response.status)
    }
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch dashboard data')
  }
  
  if (process.env.NODE_ENV !== 'production') {
    console.log('[Dashboard] ✅ Dashboard data fetched successfully')
  }
  
  return response.json()
}

export function useDashboard() {
  const { ensureValidSession } = useAuth()
  
  return useQuery({
    queryKey: ['dashboard'],
    queryFn: () => fetchDashboard(ensureValidSession),
    refetchInterval: 30000, // Refetch every 30 seconds
  })
}
