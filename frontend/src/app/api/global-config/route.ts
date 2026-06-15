import { NextResponse } from 'next/server'
import {
  getAdminAuthToken,
  getGlobalConfig,
  patchGlobalConfig,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'

export const dynamic = 'force-dynamic'

export async function GET(request: Request) {
  try {
    const token = await getAdminAuthToken(request)
    const result = await getGlobalConfig(token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Global config API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch global config' },
      { status: 500 }
    )
  }
}

export async function PATCH(request: Request) {
  try {
    const body = await request.json()
    const { config, version } = body

    if (!config || typeof version !== 'number') {
      return NextResponse.json(
        { error: 'Missing required fields: config and version' },
        { status: 400 }
      )
    }

    const token = await getAdminAuthToken(request)
    const result = await patchGlobalConfig(config, version, token)
    return NextResponse.json(result)
  } catch (error) {
    console.error('Global config patch error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update global config' },
      { status: 500 }
    )
  }
}
