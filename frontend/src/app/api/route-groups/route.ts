import { NextResponse } from 'next/server'
import {
  getRouteGroupsFromTenantConfig,
  createRouteGroupInTenantConfig,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/route-groups - List all route groups from tenant config
export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const tenantId = new URL(request.url).searchParams.get('tenantId')
    if (!tenantId) {
      return NextResponse.json({ error: 'tenantId is required' }, { status: 400 })
    }

    const routeGroups = await getRouteGroupsFromTenantConfig(tenantId, auth.token)
    return NextResponse.json({ data: routeGroups })
  } catch (error) {
    console.error('Route Groups API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch route groups' },
      { status: 500 }
    )
  }
}

// POST /api/route-groups - Create new route group in tenant config
export async function POST(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const body = await request.json()

    const tenantId = body.tenantId as string | undefined
    if (!tenantId) {
      return NextResponse.json({ error: 'tenantId is required' }, { status: 400 })
    }

    if (!body.id) {
      return NextResponse.json(
        { error: 'Missing required field: id' },
        { status: 400 }
      )
    }

    const result = await createRouteGroupInTenantConfig(
      tenantId,
      body.id,
      body.models || [],
      auth.token
    )
    return NextResponse.json(result, { status: 201 })
  } catch (error) {
    console.error('Route Group create error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to create route group' },
      { status: 500 }
    )
  }
}
