import { NextResponse } from 'next/server'
import {
  getBenchmarks,
  createBenchmark,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/benchmarks - List all benchmarks
export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const result = await getBenchmarks(auth.token)
    return NextResponse.json({ data: (result as { data?: unknown[] }).data || [] })
  } catch (error) {
    console.error('Benchmarks API error:', error)

    if (error instanceof GatewayAdminError) {
      if (error.statusCode === 404) {
        return NextResponse.json({ data: [] })
      }
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch benchmarks' },
      { status: 500 }
    )
  }
}

// POST /api/benchmarks - Create new benchmark
export async function POST(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const body = await request.json()

    if (!body.tenant_id || !body.models || !body.prompts) {
      return NextResponse.json(
        { error: 'Missing required fields: tenant_id, models, prompts' },
        { status: 400 }
      )
    }

    const result = await createBenchmark({
      tenant_id: body.tenant_id,
      models: body.models,
      prompts: body.prompts,
    }, auth.token)
    return NextResponse.json(result, { status: 201 })
  } catch (error) {
    console.error('Benchmark create error:', error)

    if (error instanceof GatewayAdminError) {
      if (error.statusCode === 404) {
        return NextResponse.json(
          { error: 'Benchmark creation is not available in the current backend version' },
          { status: 501 }
        )
      }
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to create benchmark' },
      { status: 500 }
    )
  }
}
