import { NextResponse } from 'next/server'
import {
  GatewayAdminError,
  gatewayAdminFetch,
  getAdminAuthToken,
} from '@/lib/server/gateway-admin-client'

function extractThresholdFromConfig(config: Record<string, unknown> | undefined) {
  if (!config) return null
  const routing = config.routing as Record<string, unknown> | undefined
  const semantic = (routing?.semantic as Record<string, unknown> | undefined) ??
    (config.semantic as Record<string, unknown> | undefined)
  if (semantic && typeof semantic.threshold_default === 'number') {
    return semantic.threshold_default
  }
  if (typeof (config as Record<string, unknown>).threshold_default === 'number') {
    return (config as Record<string, unknown>).threshold_default as number
  }
  return null
}

export async function GET(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const { searchParams } = new URL(request.url)
    const tenantId = searchParams.get('tenant_id')
    if (!tenantId) {
      return NextResponse.json({ error: 'tenant_id is required' }, { status: 400 })
    }
    const data = await gatewayAdminFetch(`/admin/tenants/${tenantId}/config`, {
      requestAuthToken: token,
    })
    const threshold = extractThresholdFromConfig((data as { config?: Record<string, unknown> }).config)
    return NextResponse.json({ tenant_id: tenantId, threshold_default: threshold })
  } catch (error) {
    console.error('Semantic threshold fetch error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, { status: error.statusCode || 500 })
    }
    return NextResponse.json({ error: 'Failed to fetch semantic threshold' }, { status: 500 })
  }
}

export async function PATCH(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const body = await request.json()
    const { tenant_id: tenantId, threshold_default } = body as {
      tenant_id?: string
      threshold_default?: number
    }
    if (!tenantId || typeof threshold_default !== 'number') {
      return NextResponse.json({ error: 'tenant_id and threshold_default are required' }, { status: 400 })
    }
    const data = await gatewayAdminFetch(`/admin/tenants/${tenantId}/semantic-threshold`, {
      method: 'PATCH',
      body: JSON.stringify({ threshold_default }),
      requestAuthToken: token,
    })
    return NextResponse.json(data)
  } catch (error) {
    console.error('Semantic threshold update error:', error)
    if (error instanceof GatewayAdminError) {
      return NextResponse.json({ error: error.message, details: error.details }, { status: error.statusCode || 500 })
    }
    return NextResponse.json({ error: 'Failed to update semantic threshold' }, { status: 500 })
  }
}
