import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export const dynamic = 'force-dynamic'

/** Proxies GET /admin/budgets/overview — effective tenant spend for FinOps dashboard. */
export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const result = await gatewayAdminFetch('/admin/budgets/overview', { requestAuthToken: auth.token })
    return NextResponse.json(result)
  } catch (error) {
    console.error('Budget overview API error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, { status: error.statusCode || 500 })
    }
    return NextResponse.json({ error: 'Failed to fetch budget overview' }, { status: 500 })
  }
}
