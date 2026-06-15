import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'

export interface TenantBudget {
  tenant_id: string
  monthly_usd: number | null
  timezone: string | null
  enforcement_enabled: boolean
  enforcement_mode: 'report_only' | 'hard_limit' | null
  warn_pct: number | null
  hard_pct: number | null
  current_spend_usd: number
  reserved_usd: number
  effective_spend_usd: number
  remaining_usd: number
  pct: number
  pct_effective: number
  status: 'healthy' | 'warning' | 'exceeded' | 'not_configured'
  version: number
  // V2 fields (optional)
  enforcement_paused?: boolean
  enforcement_pause_until?: string | null
  override_limit_usd?: number | null
  override_reason?: string | null
}

export interface UpdateBudgetRequest {
  monthly_usd?: number
  timezone?: string
  enforcement_enabled?: boolean
  enforcement_mode?: 'report_only' | 'hard_limit'
  warn_pct?: number
  hard_pct?: number
  // V2 fields (optional)
  enforcement_paused?: boolean
  enforcement_pause_until?: string | null
  override_limit_usd?: number | null
  override_reason?: string | null
}

// Fetch all tenant budgets
async function fetchTenantBudgets(): Promise<TenantBudget[]> {
  const response = await fetch('/api/budgets/tenants', { credentials: 'include' })

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({}))
    const msg =
      typeof errorBody.error === 'string' ? errorBody.error : 'Failed to fetch tenant budgets'
    const err = new Error(msg) as Error & { status?: number }
    err.status = response.status
    throw err
  }

  const data = await response.json()
  return data.data || []
}

// Update tenant budget
async function updateTenantBudget(
  tenantId: string,
  request: UpdateBudgetRequest,
  version: number
): Promise<TenantBudget> {
  const response = await fetch(`/api/budgets/tenants/${tenantId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ ...request, version }),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update budget')
  }

  return response.json()
}

// Hook to fetch all tenant budgets
export function useTenantBudgets() {
  return useQuery({
    queryKey: ['tenantBudgets'],
    queryFn: fetchTenantBudgets,
    // Do not retry auth / RBAC denials — avoids flashing stale data while "retrying"
    retry: (failureCount, err) => {
      const s = (err as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
  })
}

// Hook to update tenant budget
export function useUpdateTenantBudget() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: ({
      tenantId,
      request,
      version,
    }: {
      tenantId: string
      request: UpdateBudgetRequest
      version: number
    }) => updateTenantBudget(tenantId, request, version),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenantBudgets'] })
      toast({
        title: 'Success',
        description: 'Budget updated successfully',
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
