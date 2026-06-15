import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const qs = new URLSearchParams()
    const from = searchParams.get('from')
    const to = searchParams.get('to')
    const tenantId = searchParams.get('tenant_id')
    const model = searchParams.get('model')
    const provider = searchParams.get('provider')
    const status = searchParams.get('status')
    const bucket = searchParams.get('bucket')

    if (from) qs.set('from', from)
    if (to) qs.set('to', to)
    if (tenantId) qs.set('tenant_id', tenantId)
    if (model) qs.set('model', model)
    if (provider) qs.set('provider', provider)
    if (status) qs.set('status', status)
    if (bucket) qs.set('bucket', bucket)

    const queryString = qs.toString()
    const endpoint = queryString
      ? `/admin/router/performance?${queryString}`
      : '/admin/router/performance'

    const data = await gatewayAdminFetch(endpoint, { requestAuthToken: auth.token })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Router performance error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch router performance' },
      { status: 500 }
    )
  }
}
