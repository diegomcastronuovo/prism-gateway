import { NextResponse } from 'next/server'
import {
  getBenchmarkSummary,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/benchmarks/summary - Get benchmark summary stats
export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const result = await getBenchmarkSummary(auth.token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Benchmark summary API error:', error)

    if (error instanceof GatewayAdminError) {
      if (error.statusCode === 404) {
        return NextResponse.json({
          total_benchmarks: 0,
          avg_latency_ms: 0,
          success_rate: 0,
        })
      }
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch benchmark summary' },
      { status: 500 }
    )
  }
}
