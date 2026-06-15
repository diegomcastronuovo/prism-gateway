import { NextResponse } from 'next/server'
import {
  getTenantConfig,
  deleteTenant,
  getAdminAuthToken,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireTenantAuth } from '@/lib/server/require-tenant-auth'

export async function GET(
  request: Request,
  { params }: { params: { tenantId: string } }
) {
  try {
    const authResult = await requireTenantAuth(request)
    if ('response' in authResult) {
      return authResult.response
    }
    const config = await getTenantConfig(params.tenantId, authResult.token)
    return NextResponse.json(config)
  } catch (error) {
    console.error('Tenant config API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch tenant config' },
      { status: 500 }
    )
  }
}

export async function DELETE(
  request: Request,
  { params }: { params: { tenantId: string } }
) {
  try {
    const authResult = await requireTenantAuth(request)
    if ('response' in authResult) {
      return authResult.response
    }
    await deleteTenant(params.tenantId, authResult.token)
    return new NextResponse(null, { status: 204 })
  } catch (error) {
    console.error('Delete tenant API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to delete tenant' },
      { status: 500 }
    )
  }
}
