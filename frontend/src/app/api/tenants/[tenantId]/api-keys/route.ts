import { NextResponse } from 'next/server'
import { getTenantApiKeys, getAdminAuthToken, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireTenantAuth } from '@/lib/server/require-tenant-auth'

export async function GET(
  request: Request,
  { params }: { params: { tenantId: string } }
) {
  try {
    const authResult = await requireTenantAuth(request)
    if ('response' in authResult) {
      return authResult.response
    }
    const result = await getTenantApiKeys(params.tenantId, authResult.token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Tenant API keys error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch tenant API keys' },
      { status: 500 }
    )
  }
}
