import { NextResponse } from 'next/server'
import {
  gatewayAdminFetch,
  getTenantConfig,
  getTenantBudgetStatus,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export const dynamic = 'force-dynamic'

interface Tenant {
  tenant_id: string
}

interface TenantConfig {
  config?: {
    budgets?: {
      monthly_usd?: number
      timezone?: string
    }
    budget_enforcement?: {
      enabled?: boolean
      mode?: 'report_only' | 'hard_limit'
      thresholds?: {
        warn_pct?: number
        hard_pct?: number
      }
      paused?: boolean
      pause_until?: string
      override_limit_usd?: number
      override_reason?: string
    }
  }
  version: number
}

interface BudgetStatus {
  spend_usd?: number
  reserved_usd?: number
  effective_spend_usd?: number
  budget_usd?: number
  pct?: number
  pct_effective?: number
  enforcement_mode?: string
  enforcement_enabled?: boolean
  warn_pct?: number
  hard_pct?: number
}

type TenantBudgetRow = {
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
  enforcement_paused?: boolean
  enforcement_pause_until?: string | null
  override_limit_usd?: number | null
  override_reason?: string | null
}

function buildBudgetRow(
  tenant: Tenant,
  config: TenantConfig,
  status: BudgetStatus
): TenantBudgetRow {
  const budgetConfig = config.config?.budgets
  const enforcementConfig = config.config?.budget_enforcement

  const monthlyUsd = budgetConfig?.monthly_usd ?? null
  const currentSpend = status.spend_usd ?? 0
  const reservedUsd = status.reserved_usd ?? 0
  const effectiveSpendUsd = status.effective_spend_usd ?? currentSpend + reservedUsd
  const budgetUsd = status.budget_usd ?? monthlyUsd ?? 0
  const pct = status.pct ?? 0
  const pctEffective = status.pct_effective ?? (budgetUsd > 0 ? effectiveSpendUsd / budgetUsd : 0)
  const warnPct = enforcementConfig?.thresholds?.warn_pct ?? status.warn_pct ?? 0.8
  const hardPct = enforcementConfig?.thresholds?.hard_pct ?? status.hard_pct ?? 1.0

  let budgetStatus: 'healthy' | 'warning' | 'exceeded' | 'not_configured' = 'not_configured'

  if (monthlyUsd !== null && monthlyUsd > 0) {
    // Use pct_effective (confirmed + reserved) to drive health state
    if (pctEffective >= hardPct) {
      budgetStatus = 'exceeded'
    } else if (pctEffective >= warnPct) {
      budgetStatus = 'warning'
    } else {
      budgetStatus = 'healthy'
    }
  }

  return {
    tenant_id: tenant.tenant_id,
    monthly_usd: monthlyUsd,
    timezone: budgetConfig?.timezone ?? null,
    enforcement_enabled: enforcementConfig?.enabled ?? status.enforcement_enabled ?? false,
    enforcement_mode:
      enforcementConfig?.mode ?? (status.enforcement_mode as 'report_only' | 'hard_limit') ?? null,
    warn_pct: warnPct,
    hard_pct: hardPct,
    current_spend_usd: currentSpend,
    reserved_usd: reservedUsd,
    effective_spend_usd: effectiveSpendUsd,
    remaining_usd: budgetUsd - effectiveSpendUsd,
    pct,
    pct_effective: pctEffective,
    status: budgetStatus,
    version: config.version,
    enforcement_paused: enforcementConfig?.paused ?? false,
    enforcement_pause_until: enforcementConfig?.pause_until ?? null,
    override_limit_usd: enforcementConfig?.override_limit_usd ?? null,
    override_reason: enforcementConfig?.override_reason ?? null,
  }
}

// GET /api/budgets/tenants — only tenants the caller may read; never invent rows on 403/401
export async function GET(request: Request) {
  const auth = await requireAdminBearer(request)
  if ('response' in auth) return auth.response

  try {
    const tenantsResponse = await gatewayAdminFetch('/admin/tenants', {
      requestAuthToken: auth.token,
    })
    const tenants: Tenant[] = (tenantsResponse as { data?: Tenant[] }).data || []

    const rows = await Promise.all(
      tenants.map(async (tenant): Promise<TenantBudgetRow | null> => {
        try {
          const [configResponse, statusResponse] = await Promise.all([
            getTenantConfig(tenant.tenant_id, auth.token),
            getTenantBudgetStatus(tenant.tenant_id, auth.token),
          ])

          const config = configResponse as TenantConfig
          const status = statusResponse as BudgetStatus
          return buildBudgetRow(tenant, config, status)
        } catch (error) {
          console.error(`Failed to fetch budget for tenant ${tenant.tenant_id}:`, error)
          // Critical: do NOT return placeholder "not_configured" rows on 403 — that leaks fake state in the UI.
          if (error instanceof GatewayAdminError) {
            return null
          }
          return null
        }
      })
    )

    const budgets = rows.filter((row): row is TenantBudgetRow => row !== null)

    return NextResponse.json({ data: budgets })
  } catch (error) {
    console.error('Tenant budgets API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json({ error: 'Failed to fetch tenant budgets' }, { status: 500 })
  }
}
