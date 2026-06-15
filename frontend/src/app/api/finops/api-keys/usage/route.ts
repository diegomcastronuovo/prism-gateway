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
    const windowHours = searchParams.get('window_hours')
    const windowDays = searchParams.get('window_days')
    const tenantId = searchParams.get('tenant_id') || ''
    const status = searchParams.get('status') || ''
    const provider = searchParams.get('provider') || ''
    const model = searchParams.get('model') || ''
    const apiKeyName = searchParams.get('api_key_name') || ''
    const limit = searchParams.get('limit') || '50'
    const offset = searchParams.get('offset') || '0'

    const resolvedWindowDays =
      windowDays ?? (windowHours ? String(Math.max(1, Math.ceil(Number(windowHours) / 24))) : '30')

    qs.set('window_days', resolvedWindowDays)
    if (windowHours) qs.set('window_hours', windowHours)
    if (tenantId) qs.set('tenant_id', tenantId)
    if (status) qs.set('status', status)
    if (provider) qs.set('provider', provider)
    if (model) qs.set('model', model)
    if (apiKeyName) qs.set('api_key_name', apiKeyName)
    qs.set('limit', limit)
    qs.set('offset', offset)

    const data = await gatewayAdminFetch(`/admin/api-keys/usage?${qs.toString()}`, {
      requestAuthToken: auth.token,
    })

    return NextResponse.json({ data })
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      if (error.statusCode === 404) {
        return NextResponse.json(
          {
            error: 'Upstream endpoint not found: /admin/api-keys/usage',
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
    return NextResponse.json({ error: 'Failed to fetch API keys usage' }, { status: 500 })
  }
}
