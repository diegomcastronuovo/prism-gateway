import { NextResponse } from 'next/server'
import {
  getAdminAuthToken,
  getModelBenchmarks,
  deleteModelBenchmarks,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'

// GET /api/benchmarks/models - Get model performance benchmarks
export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const windowHours = parseInt(searchParams.get('window_hours') || '24', 10)
    const token = await getAdminAuthToken(request)

    const result = await getModelBenchmarks(windowHours, token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Benchmarks API error:', error)

    if (error instanceof GatewayAdminError) {
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

// DELETE /api/benchmarks/models - Purge all benchmark history
export async function DELETE(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const result = await deleteModelBenchmarks(token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Benchmarks delete error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to clear benchmark history' },
      { status: 500 }
    )
  }
}
