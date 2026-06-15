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
    const apiKeyName = searchParams.get('api_key_name') || ''
    const model = searchParams.get('model') || ''
    const provider = searchParams.get('provider') || ''
    const status = searchParams.get('status') || ''
    const limit = searchParams.get('limit') || '50'
    const offset = searchParams.get('offset') || '0'

    if (from) qs.set('from', from)
    if (to) qs.set('to', to)
    if (tenantId) qs.set('tenant_id', tenantId)
    if (apiKeyName) qs.set('api_key_name', apiKeyName)
    if (model) qs.set('model', model)
    if (provider) qs.set('provider', provider)
    if (status) qs.set('status', status)
    qs.set('limit', limit)
    qs.set('offset', offset)

    const data = await gatewayAdminFetch(`/admin/api-keys/requests?${qs.toString()}`, {
      requestAuthToken: auth.token,
    })

    return NextResponse.json({ data })
  } catch (error) {
    console.error('API keys requests error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch API keys raw usage requests' },
      { status: 500 }
    )
  }
}
