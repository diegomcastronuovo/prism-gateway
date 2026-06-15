import { NextResponse } from 'next/server'
import {
  patchTenantConfig,
  getAdminAuthToken,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireTenantAuth } from '@/lib/server/require-tenant-auth'

export async function PATCH(
  request: Request,
  { params }: { params: { tenantId: string } }
) {
  try {
    const body = await request.json()
    const { monthly_usd, timezone, version } = body

    if (version === undefined) {
      return NextResponse.json(
        { error: 'version is required for optimistic concurrency control' },
        { status: 400 }
      )
    }

    const config: { monthly_usd?: number; timezone?: string } = {}
    if (monthly_usd !== undefined) config.monthly_usd = monthly_usd
    if (timezone !== undefined) config.timezone = timezone

    const token = await getAdminAuthToken(request)
    const result = await patchTenantConfig(params.tenantId, config, version, token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Update tenant budget API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update tenant budget' },
      { status: 500 }
    )
  }
}
