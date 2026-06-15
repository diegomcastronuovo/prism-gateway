import { NextResponse } from 'next/server'
import { getAdminAuthToken, gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const windowHours = searchParams.get('window_hours') || '24'
    const token = await getAdminAuthToken(request)

    const data = await gatewayAdminFetch(`/admin/benchmarks/models?window_hours=${windowHours}`, {
      requestAuthToken: token,
    })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Model performance error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch model performance' },
      { status: 500 }
    )
  }
}
