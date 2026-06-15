import { NextResponse } from 'next/server'
import { GatewayAdminError, gatewayAdminFetch, getAdminAuthToken } from '@/lib/server/gateway-admin-client'

// GET /api/semantic/routes/[name] — fetches full route detail including utterances.
// The backend has no dedicated GET-by-name endpoint; we call PATCH with an empty body,
// which is a safe no-op (all patch fields are nil pointers → preserves current state)
// and returns the full patchSemanticRouteResponse including utterances[].
export async function GET(request: Request, { params }: { params: { name: string } }) {
  try {
    const token = await getAdminAuthToken(request)
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id')
    if (!tenantId) {
      return NextResponse.json({ error: 'tenant_id is required' }, { status: 400 })
    }
    const data = await gatewayAdminFetch(
      `/admin/semantic/routes/${encodeURIComponent(params.name)}?tenant_id=${encodeURIComponent(tenantId)}`,
      {
        method: 'PATCH',
        body: JSON.stringify({}),
        requestAuthToken: token,
      }
    )
    return NextResponse.json(data)
  } catch (error) {
    console.error('Semantic route get error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, {
        status: error.statusCode || 500,
      })
    }
    return NextResponse.json({ error: 'Failed to fetch semantic route' }, { status: 500 })
  }
}

export async function PATCH(request: Request, { params }: { params: { name: string } }) {
  try {
    const token = await getAdminAuthToken(request)
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id')
    if (!tenantId) {
      return NextResponse.json({ error: 'tenant_id is required' }, { status: 400 })
    }
    const body = await request.json().catch(() => ({}))
    const data = await gatewayAdminFetch(
      `/admin/semantic/routes/${encodeURIComponent(params.name)}?tenant_id=${encodeURIComponent(tenantId)}`,
      {
        method: 'PATCH',
        body: JSON.stringify(body),
        requestAuthToken: token,
      }
    )
    return NextResponse.json(data)
  } catch (error) {
    console.error('Semantic route patch error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, {
        status: error.statusCode || 500,
      })
    }
    return NextResponse.json({ error: 'Failed to update semantic route' }, { status: 500 })
  }
}

export async function DELETE(request: Request, { params }: { params: { name: string } }) {
  try {
    const token = await getAdminAuthToken(request)
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id')
    if (!tenantId) {
      return NextResponse.json({ error: 'tenant_id is required' }, { status: 400 })
    }
    await gatewayAdminFetch(
      `/admin/semantic/routes/${encodeURIComponent(params.name)}?tenant_id=${encodeURIComponent(tenantId)}`,
      {
        method: 'DELETE',
        requestAuthToken: token,
      }
    )
    return NextResponse.json({ status: 'deleted' })
  } catch (error) {
    console.error('Semantic route delete error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, {
        status: error.statusCode || 500,
      })
    }
    return NextResponse.json({ error: 'Failed to delete semantic route' }, { status: 500 })
  }
}
