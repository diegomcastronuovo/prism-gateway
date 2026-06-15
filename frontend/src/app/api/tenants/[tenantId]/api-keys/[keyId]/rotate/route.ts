import { NextResponse } from 'next/server'
import { rotateTenantApiKey, getAdminAuthToken, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function POST(
  request: Request,
  { params }: { params: { tenantId: string; keyId: string } }
) {
  try {
    const token = await getAdminAuthToken(request)
    const result = await rotateTenantApiKey(params.tenantId, params.keyId, token)
    console.log('Rotate API key backend response:', JSON.stringify(result, null, 2))
    return NextResponse.json(result)
  } catch (error) {
    console.error('Rotate API key error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to rotate API key' },
      { status: 500 }
    )
  }
}
