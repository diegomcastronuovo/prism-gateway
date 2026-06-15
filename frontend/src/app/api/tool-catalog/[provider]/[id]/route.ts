import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

type Params = { params: { provider: string; id: string } }

// PUT /api/tool-catalog/[provider]/[id] - Update tool catalog entry
export async function PUT(request: Request, { params }: Params) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { provider, id } = params
    const body = await request.json()

    const result = await gatewayAdminFetch(
      `/admin/tool-catalog/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`,
      {
        method: 'PUT',
        body: JSON.stringify(body),
        requestAuthToken: auth.token,
      }
    )

    return NextResponse.json(result)
  } catch (error) {
    console.error('Tool catalog PUT error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update tool catalog entry' },
      { status: 500 }
    )
  }
}

// DELETE /api/tool-catalog/[provider]/[id] - Delete tool catalog entry
export async function DELETE(request: Request, { params }: Params) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { provider, id } = params

    await gatewayAdminFetch(
      `/admin/tool-catalog/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`,
      {
        method: 'DELETE',
        requestAuthToken: auth.token,
      }
    )

    return new NextResponse(null, { status: 204 })
  } catch (error) {
    console.error('Tool catalog DELETE error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to delete tool catalog entry' },
      { status: 500 }
    )
  }
}
