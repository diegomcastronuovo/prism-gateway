import { NextResponse } from 'next/server'
import { GatewayAdminError, gatewayAdminFetch, getAdminAuthToken } from '@/lib/server/gateway-admin-client'

function tenantIdFromRequest(request: Request, body: Record<string, unknown>): string | null {
  const { searchParams } = new URL(request.url)
  const fromQuery = searchParams.get('tenant_id')
  if (fromQuery) return fromQuery
  const fromBody = body.tenant_id
  if (typeof fromBody === 'string' && fromBody.trim()) return fromBody.trim()
  return null
}

export async function POST(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const body = (await request.json().catch(() => ({}))) as Record<string, unknown>
    const tenantId = tenantIdFromRequest(request, body)
    if (!tenantId) {
      return NextResponse.json({ error: 'tenant_id is required' }, { status: 400 })
    }
    const rest = { ...body }
    delete rest.tenant_id
    const data = await gatewayAdminFetch(`/admin/semantic/anchors/suggest?tenant_id=${encodeURIComponent(tenantId)}`, {
      method: 'POST',
      body: JSON.stringify(rest),
      requestAuthToken: token,
    })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Semantic suggestion error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, {
        status: error.statusCode || 500,
      })
    }
    return NextResponse.json({ error: 'Failed to suggest anchors' }, { status: 500 })
  }
}
