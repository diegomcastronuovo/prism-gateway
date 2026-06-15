import { NextResponse } from 'next/server'
import { createTenantApiKey, getAdminAuthToken, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function POST(
  request: Request,
  { params }: { params: { tenantId: string } }
) {
  try {
    const body = await request.json()
    console.log('Creating API key for tenant:', params.tenantId, 'with body:', body)
    const token = await getAdminAuthToken(request)
    const result = await createTenantApiKey(params.tenantId, body, token)
    console.log('Backend response:', JSON.stringify(result, null, 2))
    console.log('API key in response:', result.api_key)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Create API key error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to create API key' },
      { status: 500 }
    )
  }
}
