import { NextResponse } from 'next/server'
import { GatewayAdminError, gatewayAdminFetch, getAdminAuthToken } from '@/lib/server/gateway-admin-client'

export async function PATCH(request: Request, { params }: { params: { name: string } }) {
  try {
    const token = await getAdminAuthToken(request)
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id')
    const body = await request.json()
    const path = tenantId
      ? `/v1/semantic/anchors/${encodeURIComponent(params.name)}?tenant_id=${encodeURIComponent(tenantId)}`
      : `/v1/semantic/anchors/${encodeURIComponent(params.name)}`
    const data = await gatewayAdminFetch(path, {
      method: 'PATCH',
      body: JSON.stringify(body),
      requestAuthToken: token,
    })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Semantic anchor update error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, {
        status: error.statusCode || 500,
      })
    }
    return NextResponse.json({ error: 'Failed to update semantic anchor' }, { status: 500 })
  }
}

export async function DELETE(request: Request, { params }: { params: { name: string } }) {
  try {
    const token = await getAdminAuthToken(request)
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id')
    const path = tenantId
      ? `/v1/semantic/anchors/${encodeURIComponent(params.name)}?tenant_id=${encodeURIComponent(tenantId)}`
      : `/v1/semantic/anchors/${encodeURIComponent(params.name)}`
    await gatewayAdminFetch(path, {
      method: 'DELETE',
      requestAuthToken: token,
    })
    return NextResponse.json({ status: 'deleted' })
  } catch (error) {
    console.error('Semantic anchor delete error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, {
        status: error.statusCode || 500,
      })
    }
    return NextResponse.json({ error: 'Failed to delete semantic anchor' }, { status: 500 })
  }
}
