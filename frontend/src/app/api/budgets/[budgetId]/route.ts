import { NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/budgets/[budgetId] - Get single budget
export async function GET(
  request: Request,
  { params }: { params: { budgetId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const { budgetId } = params
    const result = await gatewayAdminFetch(`/admin/budgets/${budgetId}`, { requestAuthToken: auth.token })
    return NextResponse.json(result)
  } catch (error) {
    console.error(`Budget API error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch budget' },
      { status: 500 }
    )
  }
}

// PATCH /api/budgets/[budgetId] - Update budget
export async function PATCH(
  request: Request,
  { params }: { params: { budgetId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const { budgetId } = params
    const body = await request.json()

    const result = await gatewayAdminFetch(`/admin/budgets/${budgetId}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
      requestAuthToken: auth.token,
    })
    return NextResponse.json(result)
  } catch (error) {
    console.error(`Budget update error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update budget' },
      { status: 500 }
    )
  }
}

// DELETE /api/budgets/[budgetId] - Delete budget
export async function DELETE(
  request: Request,
  { params }: { params: { budgetId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const { budgetId } = params
    await gatewayAdminFetch(`/admin/budgets/${budgetId}`, {
      method: 'DELETE',
      requestAuthToken: auth.token,
    })
    return new NextResponse(null, { status: 204 })
  } catch (error) {
    console.error(`Budget delete error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to delete budget' },
      { status: 500 }
    )
  }
}
