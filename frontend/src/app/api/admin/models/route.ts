import { NextResponse } from 'next/server'
import { GatewayAdminError, gatewayAdminFetch, getAdminAuthToken } from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const data = await gatewayAdminFetch('/admin/models', {
      requestAuthToken: token,
    })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Admin models fetch error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }
    return NextResponse.json({ error: 'Failed to fetch models' }, { status: 500 })
  }
}
