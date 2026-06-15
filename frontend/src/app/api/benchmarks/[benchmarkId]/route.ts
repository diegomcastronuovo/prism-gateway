import { NextResponse } from 'next/server'
import {
  getBenchmark,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/benchmarks/[benchmarkId] - Get single benchmark with details
export async function GET(
  request: Request,
  { params }: { params: { benchmarkId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const { benchmarkId } = params
    const result = await getBenchmark(benchmarkId, auth.token)
    return NextResponse.json(result)
  } catch (error) {
    console.error(`Benchmark API error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch benchmark' },
      { status: 500 }
    )
  }
}
