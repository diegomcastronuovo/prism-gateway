import { NextResponse } from 'next/server'
import { GatewayAdminError, gatewayAdminFetch, getAdminAuthToken } from '@/lib/server/gateway-admin-client'

function serializeParams(params: Record<string, string | number | boolean | undefined>) {
  const search = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined) return
    search.set(key, String(value))
  })
  return search.toString()
}

export async function GET(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const { searchParams } = new URL(request.url)
    const limit = searchParams.get('limit') ?? '50'
    const offset = searchParams.get('offset') ?? '0'
    const includeAnchorText = searchParams.get('include_anchor_text') ?? 'false'
    const tenantId = searchParams.get('tenant_id') ?? undefined
    const query = serializeParams({ limit, offset, include_anchor_text: includeAnchorText, tenant_id: tenantId })
    const data = await gatewayAdminFetch(`/v1/semantic/anchors?${query}`, {
      requestAuthToken: token,
    })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Semantic anchors list error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, {
        status: error.statusCode || 500,
      })
    }
    return NextResponse.json({ error: 'Failed to fetch semantic anchors' }, { status: 500 })
  }
}

export async function POST(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id')
    const body = await request.json()
    const path = tenantId ? `/v1/semantic/anchors?tenant_id=${encodeURIComponent(tenantId)}` : '/v1/semantic/anchors'
    const data = await gatewayAdminFetch(path, {
      method: 'POST',
      body: JSON.stringify(body),
      requestAuthToken: token,
    })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Semantic anchor create error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, {
        status: error.statusCode || 500,
      })
    }
    return NextResponse.json({ error: 'Failed to create semantic anchor' }, { status: 500 })
  }
}
