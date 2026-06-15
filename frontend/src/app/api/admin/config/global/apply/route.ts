import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function POST(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const body = await request.json().catch(() => ({})) as { version?: number }
    const version = body?.version
    if (typeof version !== 'number' || !Number.isFinite(version)) {
      return NextResponse.json({ error: 'version (number) is required' }, { status: 400 })
    }

    const data = await gatewayAdminFetch('/admin/config/global/apply', {
      method: 'POST',
      body: JSON.stringify({ version }),
      requestAuthToken: auth.token,
    })

    return NextResponse.json(data)
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, status: error.statusCode }, { status: error.statusCode ?? 500 })
    }
    return NextResponse.json({ error: 'Unexpected error' }, { status: 500 })
  }
}
