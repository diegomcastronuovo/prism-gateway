import { NextResponse } from 'next/server'
import {
  getRouteGroupsFromTenantConfig,
  updateRouteGroupInTenantConfig,
  deleteRouteGroupFromTenantConfig,
  getTenantConfig,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

function requireTenantId(request: Request): string | NextResponse {
  const tenantId = new URL(request.url).searchParams.get('tenantId')
  if (!tenantId) {
    return NextResponse.json({ error: 'tenantId is required' }, { status: 400 })
  }
  return tenantId
}

// GET /api/route-groups/[routeGroupId] - Get single route group from tenant config
export async function GET(
  request: Request,
  { params }: { params: { routeGroupId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const tenantResolved = requireTenantId(request)
    if (tenantResolved instanceof NextResponse) return tenantResolved
    const tenantId = tenantResolved

    const { routeGroupId } = params

    // Fetch route groups from tenant config
    const routeGroups = await getRouteGroupsFromTenantConfig(tenantId, auth.token)
    const routeGroup = routeGroups.find((rg) => rg.id === routeGroupId)

    if (!routeGroup) {
      return NextResponse.json(
        { error: `Route group ${routeGroupId} not found` },
        { status: 404 }
      )
    }

    // Fetch version from tenant config
    const tenantConfig = await getTenantConfig(tenantId, auth.token)
    const version = tenantConfig.version || 1

    return NextResponse.json({
      data: {
        ...routeGroup,
        version,
      },
    })
  } catch (error) {
    console.error(`Route Group API error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch route group' },
      { status: 500 }
    )
  }
}

// PATCH /api/route-groups/[routeGroupId] - Update route group in tenant config
export async function PATCH(
  request: Request,
  { params }: { params: { routeGroupId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const tenantResolved = requireTenantId(request)
    if (tenantResolved instanceof NextResponse) return tenantResolved
    const tenantId = tenantResolved

    const { routeGroupId } = params
    const body = await request.json()

    const result = await updateRouteGroupInTenantConfig(
      tenantId,
      routeGroupId,
      body.models || [],
      auth.token
    )
    return NextResponse.json(result)
  } catch (error) {
    console.error(`Route Group patch error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update route group' },
      { status: 500 }
    )
  }
}

// DELETE /api/route-groups/[routeGroupId] - Delete route group from tenant config
export async function DELETE(
  request: Request,
  { params }: { params: { routeGroupId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const tenantResolved = requireTenantId(request)
    if (tenantResolved instanceof NextResponse) return tenantResolved
    const tenantId = tenantResolved

    const { routeGroupId } = params

    await deleteRouteGroupFromTenantConfig(
      tenantId,
      routeGroupId,
      auth.token
    )
    return NextResponse.json({ message: 'Route group deleted successfully' })
  } catch (error) {
    console.error(`Route Group delete error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to delete route group' },
      { status: 500 }
    )
  }
}
