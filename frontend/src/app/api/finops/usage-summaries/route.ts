import { NextResponse } from 'next/server'
import {
  getTenants,
  getTenantUsageSummary,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export const dynamic = 'force-dynamic'

interface Tenant {
  tenant_id: string
}

export async function GET(request: Request) {
  const auth = await requireAdminBearer(request)
  if ('response' in auth) return auth.response

  try {
    const { searchParams } = new URL(request.url)
    const month = searchParams.get('month')

    if (!month) {
      return NextResponse.json(
        { error: 'Missing required query param: month (YYYY-MM)' },
        { status: 400 }
      )
    }

    const tenantsResp = await getTenants(auth.token)
    const tenants: Tenant[] = Array.isArray((tenantsResp as { data?: unknown })?.data)
      ? ((tenantsResp as { data: Tenant[] }).data)
      : []

    const summaries = await Promise.all(
      tenants.map(async (t) => {
        try {
          const data = await getTenantUsageSummary(t.tenant_id, month, auth.token)

          // Backend returns { total_cost_usd, total_requests, models: { [modelName]: { requests, cost_usd, ... } } }
          const d = data as Record<string, unknown>
          const total_cost_usd = Number(d?.total_cost_usd ?? d?.cost_usd ?? 0)
          const total_requests = Number(d?.total_requests ?? d?.requests ?? 0)
          const modelsPayload = d?.models
          let models: Array<{ model: string; cost_usd: number; requests: number }> = []

          if (Array.isArray(modelsPayload)) {
            models = modelsPayload.map((m: Record<string, unknown>) => ({
              model: String(m.model ?? ''),
              cost_usd: Number(m.cost_usd ?? 0),
              requests: Number(m.requests ?? 0),
            }))
          } else if (modelsPayload && typeof modelsPayload === 'object') {
            models = Object.entries(modelsPayload as Record<string, unknown>).map(([modelName, raw]) => {
              const entry = raw as Record<string, unknown>
              return {
                model: String(modelName),
                cost_usd: Number(entry?.cost_usd ?? 0),
                requests: Number(entry?.requests ?? 0),
              }
            })
          }

          return {
            tenant_id: t.tenant_id,
            total_cost_usd,
            total_requests,
            models,
          }
        } catch (err) {
          if (err instanceof GatewayAdminError && (err.statusCode === 401 || err.statusCode === 403)) {
            throw err
          }
          console.error('Usage summary error for tenant', t.tenant_id, err)
          return {
            tenant_id: t.tenant_id,
            total_cost_usd: 0,
            total_requests: 0,
            models: [] as Array<{ model: string; cost_usd: number; requests: number }>,
          }
        }
      })
    )

    return NextResponse.json({ data: summaries })
  } catch (error: unknown) {
    console.error('FinOps usage summaries API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch usage summaries' },
      { status: 500 }
    )
  }
}
