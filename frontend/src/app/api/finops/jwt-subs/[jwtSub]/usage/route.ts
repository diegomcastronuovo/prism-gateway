import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export const dynamic = 'force-dynamic'

export async function GET(request: Request, { params }: { params: { jwtSub: string } }) {
  const auth = await requireAdminBearer(request)
  if ('response' in auth) return auth.response

  try {
    const { searchParams } = new URL(request.url)
    const jwtSub = params.jwtSub

    const qs = new URLSearchParams()
    const from = searchParams.get('from') || ''
    const to = searchParams.get('to') || ''
    const tenantId = searchParams.get('tenant_id') || ''
    const groupBy = searchParams.get('group_by') || ''

    if (from) qs.set('from', from)
    if (to) qs.set('to', to)
    if (tenantId) qs.set('tenant_id', tenantId)
    if (groupBy) qs.set('group_by', groupBy)

    const data = await gatewayAdminFetch(`/admin/jwt-subs/${encodeURIComponent(jwtSub)}/usage?${qs.toString()}`, {
      requestAuthToken: auth.token,
    })

    return NextResponse.json({ data })
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      if (error.statusCode === 404) {
        return NextResponse.json(
          {
            error: 'Upstream endpoint not found: /admin/jwt-subs/{jwt_sub}/usage',
            details: error.details,
          },
          { status: 502 }
        )
      }
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }
    return NextResponse.json({ error: 'Failed to fetch JWT sub usage detail' }, { status: 500 })
  }
}
