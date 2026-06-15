import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export const dynamic = 'force-dynamic'

export async function GET(request: Request) {
  const auth = await requireAdminBearer(request)
  if ('response' in auth) return auth.response

  try {
    const { searchParams } = new URL(request.url)

    const qs = new URLSearchParams()
    const from = searchParams.get('from') || ''
    const to = searchParams.get('to') || ''
    const tenantId = searchParams.get('tenant_id') || ''
    const provider = searchParams.get('provider') || ''
    const model = searchParams.get('model') || ''
    const status = searchParams.get('status') || ''
    const jwtSub = searchParams.get('jwt_sub') || ''
    const sortBy = searchParams.get('sort_by') || ''
    const sortOrder = searchParams.get('sort_order') || ''
    const limit = searchParams.get('limit') || '50'
    const offset = searchParams.get('offset') || '0'

    if (from) qs.set('from', from)
    if (to) qs.set('to', to)
    if (tenantId) qs.set('tenant_id', tenantId)
    if (provider) qs.set('provider', provider)
    if (model) qs.set('model', model)
    if (status) qs.set('status', status)
    if (jwtSub) qs.set('jwt_sub', jwtSub)
    if (sortBy) qs.set('sort_by', sortBy)
    if (sortOrder) qs.set('sort_order', sortOrder)
    qs.set('limit', limit)
    qs.set('offset', offset)

    const data = await gatewayAdminFetch(`/admin/jwt-subs/usage?${qs.toString()}`, {
      requestAuthToken: auth.token,
    })

    return NextResponse.json({ data })
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      if (error.statusCode === 404) {
        return NextResponse.json(
          {
            error: 'Upstream endpoint not found: /admin/jwt-subs/usage',
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
    return NextResponse.json({ error: 'Failed to fetch JWT subs usage' }, { status: 500 })
  }
}
