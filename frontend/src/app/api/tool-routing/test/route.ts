import { NextResponse } from 'next/server'
import { GatewayAdminError, gatewayAdminFetch, getAdminAuthToken } from '@/lib/server/gateway-admin-client'

const GATEWAY_BASE_URL = process.env.GATEWAY_BASE_URL

async function revokeEphemeralKey(tenantId: string, keyId: string, token: string | null): Promise<void> {
  await gatewayAdminFetch(
    `/admin/tenants/${encodeURIComponent(tenantId)}/api-keys/${encodeURIComponent(keyId)}/revoke`,
    { method: 'POST', requestAuthToken: token }
  )
}

export async function POST(request: Request) {
  const { searchParams } = new URL(request.url)
  const tenantId = searchParams.get('tenant_id')
  if (!tenantId) {
    return NextResponse.json({ error: 'tenant_id is required' }, { status: 400 })
  }

  if (!GATEWAY_BASE_URL) {
    return NextResponse.json({ error: 'Missing GATEWAY_BASE_URL' }, { status: 500 })
  }

  let body: unknown
  try {
    body = await request.json()
  } catch {
    return NextResponse.json({ error: 'Invalid request body' }, { status: 400 })
  }

  const token = await getAdminAuthToken(request)

  // Create an ephemeral inference API key scoped to the selected tenant.
  // The plaintext key is only returned on creation — we use it once and revoke it immediately.
  let ephemeralKey: string
  let ephemeralKeyId: string
  try {
    const createResult = await gatewayAdminFetch(
      `/admin/tenants/${encodeURIComponent(tenantId)}/api-keys`,
      {
        method: 'POST',
        body: JSON.stringify({ name: '__tool_routing_test__', scopes: ['inference'] }),
        requestAuthToken: token,
      }
    )
    ephemeralKey = createResult?.key
    ephemeralKeyId = createResult?.id
    if (!ephemeralKey || !ephemeralKeyId) {
      return NextResponse.json({ error: 'Failed to obtain test credentials for tenant' }, { status: 500 })
    }
  } catch (error) {
    const msg = error instanceof GatewayAdminError ? error.message : 'Failed to create test credentials'
    const status = error instanceof GatewayAdminError ? (error.statusCode ?? 500) : 500
    return NextResponse.json({ error: msg }, { status })
  }

  // Call the real runtime endpoint with the ephemeral tenant key.
  let inferenceRes: Response
  try {
    inferenceRes = await fetch(`${GATEWAY_BASE_URL}/v1/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-API-Key': ephemeralKey,
      },
      body: JSON.stringify(body),
    })
  } catch {
    await revokeEphemeralKey(tenantId, ephemeralKeyId, token).catch(() => {})
    return NextResponse.json({ error: 'Backend unreachable' }, { status: 503 })
  }

  // Revoke the ephemeral key regardless of inference outcome.
  await revokeEphemeralKey(tenantId, ephemeralKeyId, token).catch(() => {})

  // Forward the inference response body and tool routing headers.
  const responseText = await inferenceRes.text()
  const responseHeaders = new Headers({ 'Content-Type': 'application/json' })

  const toolRoute = inferenceRes.headers.get('x-tool-route')
  const toolAction = inferenceRes.headers.get('x-tool-action')
  const toolSimilarity = inferenceRes.headers.get('x-tool-route-similarity')
  const selectedModel = inferenceRes.headers.get('x-selected-model')

  if (toolRoute) responseHeaders.set('x-tool-route', toolRoute)
  if (toolAction) responseHeaders.set('x-tool-action', toolAction)
  if (toolSimilarity) responseHeaders.set('x-tool-route-similarity', toolSimilarity)
  if (selectedModel) responseHeaders.set('x-selected-model', selectedModel)

  return new Response(responseText, {
    status: inferenceRes.status,
    headers: responseHeaders,
  })
}
