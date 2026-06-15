import { NextResponse } from 'next/server'
import {
  getAdminAuthToken,
  getGlobalConfigChanges,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const limit = parseInt(searchParams.get('limit') || '50', 10)
    const offset = parseInt(searchParams.get('offset') || '0', 10)

    const token = await getAdminAuthToken(request)
    const result = await getGlobalConfigChanges(limit, offset, token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Global config changes API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch global config changes' },
      { status: 500 }
    )
  }
}
