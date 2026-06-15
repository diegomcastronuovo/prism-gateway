import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/model-catalog - List model catalog entries
export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { searchParams } = new URL(request.url)
    const query = searchParams.toString()
    const path = query ? `/admin/model-catalog?${query}` : '/admin/model-catalog'

    const result = await gatewayAdminFetch(path, { requestAuthToken: auth.token })
    return NextResponse.json(result)
  } catch (error) {
    console.error('Model catalog GET error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch model catalog' },
      { status: 500 }
    )
  }
}

// POST /api/model-catalog - Create model catalog entry
export async function POST(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const body = await request.json()

    const result = await gatewayAdminFetch('/admin/model-catalog', {
      method: 'POST',
      body: JSON.stringify(body),
      requestAuthToken: auth.token,
    })

    return NextResponse.json(result, { status: 201 })
  } catch (error) {
    console.error('Model catalog POST error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to create model catalog entry' },
      { status: 500 }
    )
  }
}
