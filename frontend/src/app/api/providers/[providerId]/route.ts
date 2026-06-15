import { NextResponse } from 'next/server'
import {
  getAdminAuthToken,
  getGlobalConfig,
  getProviders,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'

export const dynamic = 'force-dynamic'

// GET /api/providers/[providerId] - Get single provider runtime metadata
export async function GET(
  request: Request,
  { params }: { params: { providerId: string } }
) {
  try {
    const { providerId } = params
    const token = await getAdminAuthToken(request)

    // Fetch all providers from backend runtime endpoint
    const result = await getProviders(token)
    const providers = ((result as { data?: unknown[] }).data || []) as Array<Record<string, unknown>>
    
    // Find the requested provider
    const provider = providers.find((p) => p.id === providerId)
    
    if (!provider) {
      return NextResponse.json(
        { error: `Provider ${providerId} not found` },
        { status: 404 }
      )
    }

    // Version must match global config (PATCH provider updates global config, not runtime-only state)
    let version = 1
    try {
      const gc = await getGlobalConfig(token)
      if (typeof gc?.version === 'number') version = gc.version
    } catch {
      // Fallback if global config cannot be read (e.g. no session); client may still have cache
    }

    return NextResponse.json({
      data: provider,
      version,
    })
  } catch (error) {
    console.error(`Provider API error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch provider' },
      { status: 500 }
    )
  }
}

// PATCH /api/providers/[providerId] - Update provider config
// Note: This updates global config, not runtime state
export async function PATCH(
  request: Request,
  { params }: { params: { providerId: string } }
) {
  // Import here to avoid loading on GET requests
  const { getAdminAuthToken, patchProviderConfig } = await import('@/lib/server/gateway-admin-client')
  
  try {
    const { providerId } = params
    const body = await request.json()
    const { config, version } = body

    if (!config || typeof version !== 'number') {
      return NextResponse.json(
        { error: 'Missing required fields: config and version' },
        { status: 400 }
      )
    }

    // Remove any credential-related fields from config patch
    const safeConfig = { ...config } as Record<string, unknown>
    delete safeConfig.credentials
    delete safeConfig.api_key
    delete safeConfig.api_secret

    const token = await getAdminAuthToken(request)
    const result = await patchProviderConfig(providerId, safeConfig, version, token)
    return NextResponse.json(result)
  } catch (error) {
    console.error(`Provider patch error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update provider' },
      { status: 500 }
    )
  }
}
