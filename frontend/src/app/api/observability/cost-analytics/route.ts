import { NextResponse } from 'next/server'
import { getAdminAuthToken, gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const data = await gatewayAdminFetch('/admin/observability/cost-analytics', {
      requestAuthToken: token,
    })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Cost analytics error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch cost analytics' },
      { status: 500 }
    )
  }
}
