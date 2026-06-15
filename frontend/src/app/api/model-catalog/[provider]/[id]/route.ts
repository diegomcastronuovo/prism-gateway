import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

type Params = { params: { provider: string; id: string } }

// PUT /api/model-catalog/[provider]/[id] - Update model catalog entry
export async function PUT(request: Request, { params }: Params) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { provider, id } = params
    const body = await request.json()

    const result = await gatewayAdminFetch(
      `/admin/model-catalog/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`,
      {
        method: 'PUT',
        body: JSON.stringify(body),
        requestAuthToken: auth.token,
      }
    )

    return NextResponse.json(result)
  } catch (error) {
    console.error('Model catalog PUT error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update model catalog entry' },
      { status: 500 }
    )
  }
}

// DELETE /api/model-catalog/[provider]/[id] - Delete model catalog entry
export async function DELETE(request: Request, { params }: Params) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { provider, id } = params

    await gatewayAdminFetch(
      `/admin/model-catalog/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`,
      {
        method: 'DELETE',
        requestAuthToken: auth.token,
      }
    )

    return new NextResponse(null, { status: 204 })
  } catch (error) {
    console.error('Model catalog DELETE error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to delete model catalog entry' },
      { status: 500 }
    )
  }
}
