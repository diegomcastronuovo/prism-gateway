import { NextResponse } from 'next/server'
import {
  patchTenantConfigGeneric,
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
    const { version, patch } = body

    if (version === undefined) {
      return NextResponse.json(
        { error: 'version is required for optimistic concurrency control' },
        { status: 400 }
      )
    }

    if (!patch || typeof patch !== 'object') {
      return NextResponse.json(
        { error: 'patch is required and must be an object' },
        { status: 400 }
      )
    }

    const token = await getAdminAuthToken(request)
    const result = await patchTenantConfigGeneric(params.tenantId, patch, version, token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Update tenant config API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update tenant config' },
      { status: 500 }
    )
  }
}
