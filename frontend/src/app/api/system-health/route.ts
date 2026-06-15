import { NextResponse } from 'next/server'
import { getSystemHealth, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const result = await getSystemHealth(auth.token)
    return NextResponse.json(result)
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: { message: error.message } },
        { status: error.statusCode || 500 }
      )
    }
    return NextResponse.json(
      { error: { message: 'Health check failed' } },
      { status: 500 }
    )
  }
}
