import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { searchParams } = new URL(request.url)
    const scope = searchParams.get('scope') || 'global'
    const tenantId = searchParams.get('tenant_id') || ''
    const fromVersion = searchParams.get('from_version') || ''
    const toVersion = searchParams.get('to_version') || ''

    if (!fromVersion || !toVersion) {
      return NextResponse.json(
        { error: 'from_version and to_version are required' },
        { status: 400 }
      )
    }

    const qs = new URLSearchParams()
    qs.set('scope', scope)
    if (tenantId) {
      qs.set('tenant_id', tenantId)
    }
    qs.set('from_version', fromVersion)
    qs.set('to_version', toVersion)

    const data = await gatewayAdminFetch(`/admin/config/diff?${qs.toString()}`, {
      requestAuthToken: auth.token,
    })

    return NextResponse.json(data)
  } catch (error) {
    console.error('Config diff error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch config diff' },
      { status: 500 }
    )
  }
}
