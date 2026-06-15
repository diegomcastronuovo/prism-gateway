import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/budgets - List all budgets
export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const result = await gatewayAdminFetch('/admin/budgets', { requestAuthToken: auth.token })
    return NextResponse.json({ data: (result as { data?: unknown[] }).data || [] })
  } catch (error) {
    console.error('Budgets API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch budgets' },
      { status: 500 }
    )
  }
}

// POST /api/budgets - Create new budget
export async function POST(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const body = await request.json()

    if (!body.tenant_id || !body.name || !body.limit_usd || !body.window_hours || body.alert_threshold === undefined) {
      return NextResponse.json(
        { error: 'Missing required fields: tenant_id, name, limit_usd, window_hours, alert_threshold' },
        { status: 400 }
      )
    }

    const result = await gatewayAdminFetch('/admin/budgets', {
      method: 'POST',
      body: JSON.stringify(body),
      requestAuthToken: auth.token,
    })
    return NextResponse.json(result, { status: 201 })
  } catch (error) {
    console.error('Budget create error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to create budget' },
      { status: 500 }
    )
  }
}
