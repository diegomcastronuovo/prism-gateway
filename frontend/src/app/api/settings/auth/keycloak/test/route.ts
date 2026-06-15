import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function POST(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const body = (await request.json().catch(() => ({}))) as {
      jwks_url?: string
      issuer?: string
      audience?: string
    }

    const { jwks_url } = body || {}
    if (!jwks_url) {
      return NextResponse.json(
        { ok: false, status: 400, message: 'jwks_url is required' },
        { status: 400 }
      )
    }

    // SPEC_80: Always go through backend endpoint
    const payload = { url: jwks_url, type: 'jwks' as const }
    const data = await gatewayAdminFetch('/admin/test-connection', {
      method: 'POST',
      body: JSON.stringify(payload),
      requestAuthToken: auth.token,
    })

    // Expect shape { ok: boolean, status?: number, error?: string }
    if (data?.ok) {
      return NextResponse.json({ ok: true, status: data.status ?? 200, message: 'Keycloak JWKS reachable' })
    }
    return NextResponse.json(
      { ok: false, status: data?.status ?? 500, message: data?.error || 'Failed to connect to JWKS' },
      { status: typeof data?.status === 'number' ? data.status : 500 }
    )
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { ok: false, status: error.statusCode ?? 500, message: error.message },
        { status: error.statusCode ?? 500 }
      )
    }
    const message = error instanceof Error ? error.message : 'Unexpected error'
    return NextResponse.json({ ok: false, status: 500, message }, { status: 500 })
  }
}
