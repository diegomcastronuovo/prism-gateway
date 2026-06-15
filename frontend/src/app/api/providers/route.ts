import { NextResponse } from 'next/server'
import { getAdminAuthToken, getProviders, GatewayAdminError } from '@/lib/server/gateway-admin-client'

/** Avoid static caching of admin/runtime data */
export const dynamic = 'force-dynamic'

export async function GET(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const result = await getProviders(token)
    return NextResponse.json({ data: (result as { data?: unknown[] }).data || [] })
  } catch (error) {
    console.error('Providers API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch providers' },
      { status: 500 }
    )
  }
}
