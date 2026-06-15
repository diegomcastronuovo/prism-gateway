import { NextResponse } from 'next/server'
import { testTenantPiiConnection, getAdminAuthToken, GatewayAdminError } from '@/lib/server/gateway-admin-client'

export async function POST(request: Request, { params }: { params: { tenantId: string } }) {
  try {
    const body = await request.json()
    const { request_url, response_url, timeout_ms, api_key } = body || {}

    if (!request_url || !response_url || typeof timeout_ms !== 'number') {
      return NextResponse.json({ error: 'Missing or invalid payload' }, { status: 400 })
    }

    const token = await getAdminAuthToken(request)
    const result = await testTenantPiiConnection(
      params.tenantId,
      { request_url, response_url, timeout_ms, ...(api_key && { api_key }) },
      token
    )

    return NextResponse.json(result)
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }
    return NextResponse.json({ error: 'Failed to test PII connection' }, { status: 500 })
  }
}
