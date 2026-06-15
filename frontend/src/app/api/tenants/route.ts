import { NextResponse } from 'next/server'
import {
  getTenants,
  createTenant,
  getAdminAuthToken,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'

export async function GET(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const result = await getTenants(token)
    return NextResponse.json((result as { data?: unknown[] }).data || [])
  } catch (error) {
    console.error('Tenants list API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch tenants' },
      { status: 500 }
    )
  }
}

export async function POST(request: Request) {
  try {
    const body = await request.json()
    const { tenant_id, environment } = body

    if (!tenant_id) {
      return NextResponse.json(
        { error: 'tenant_id is required' },
        { status: 400 }
      )
    }
    if (!environment || !['DEV', 'STAGING', 'PROD'].includes(String(environment))) {
      return NextResponse.json(
        { error: 'environment is required' },
        { status: 400 }
      )
    }

    const token = await getAdminAuthToken(request)
    const result = await createTenant(tenant_id, String(environment) as 'DEV' | 'STAGING' | 'PROD', token)
    return NextResponse.json(result, { status: 201 })
  } catch (error) {
    console.error('Create tenant API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to create tenant' },
      { status: 500 }
    )
  }
}
