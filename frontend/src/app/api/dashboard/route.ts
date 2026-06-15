import { NextResponse } from 'next/server'
import {
  getVersion,
  getTenants,
  getModels,
  getProviders,
  getRouteGroups,
  getFeatures,
  getModelBenchmarks,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'

export async function GET() {
  try {
    // Do NOT silently fall back to empty data: if any call fails (e.g., auth), surface the error
    const [version, tenants, models, providers, routeGroups, features, benchmarks] =
      await Promise.all([
        getVersion(),
        getTenants(),
        getModels(),
        getProviders(),
        getRouteGroups(),
        getFeatures(),
        getModelBenchmarks(24),
      ])

    return NextResponse.json({
      version,
      tenants: (tenants as { data?: unknown[] }).data || [],
      models: (models as { data?: unknown[] }).data || [],
      providers: (providers as { data?: unknown[] }).data || [],
      routeGroups: (routeGroups as { data?: unknown[] }).data || [],
      features,
      benchmarks: (benchmarks as { data?: unknown[] }).data || [],
    })
  } catch (error) {
    if (process.env.NODE_ENV !== 'production') {
      console.error('Dashboard API error:', error)
    }

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch dashboard data' },
      { status: 500 }
    )
  }
}
