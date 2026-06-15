import { NextResponse } from 'next/server'
import { revokeTenantApiKey, getAdminAuthToken, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function POST(
  request: Request,
  { params }: { params: { tenantId: string; keyId: string } }
) {
  try {
    const token = await getAdminAuthToken(request)
    const result = await revokeTenantApiKey(params.tenantId, params.keyId, token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Revoke API key error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to revoke API key' },
      { status: 500 }
    )
  }
}
