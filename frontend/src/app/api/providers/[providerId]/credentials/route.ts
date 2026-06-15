import { NextResponse } from 'next/server'
import {
  getAdminAuthToken,
  updateProviderCredentials,
  GatewayAdminError,
} from '@/lib/server/gateway-admin-client'

export const dynamic = 'force-dynamic'

// POST /api/providers/[providerId]/credentials - Update provider credentials
export async function POST(
  request: Request,
  { params }: { params: { providerId: string } }
) {
  try {
    const { providerId } = params
    const body = await request.json()
    const { credentials, version } = body

    if (!credentials || typeof version !== 'number') {
      return NextResponse.json(
        { error: 'Missing required fields: credentials and version' },
        { status: 400 }
      )
    }

    if (providerId === 'bedrock') {
      const ak = typeof credentials.aws_access_key_id === 'string' ? credentials.aws_access_key_id.trim() : ''
      const sk = typeof credentials.aws_secret_access_key === 'string' ? credentials.aws_secret_access_key.trim() : ''
      const region = typeof credentials.aws_region === 'string' ? credentials.aws_region.trim() : ''
      if (!ak) {
        return NextResponse.json({ error: 'AWS Access Key ID is required' }, { status: 400 })
      }
      if (!sk) {
        return NextResponse.json({ error: 'AWS Secret Access Key is required' }, { status: 400 })
      }
      if (!region) {
        return NextResponse.json({ error: 'AWS Region is required' }, { status: 400 })
      }
      const token = await getAdminAuthToken(request)
      const result = await updateProviderCredentials(
        providerId,
        { aws_access_key_id: ak, aws_secret_access_key: sk, aws_region: region },
        version,
        token
      )
      return NextResponse.json({
        message: 'Credentials updated successfully',
        has_api_key: true,
        api_key_source: 'global_config',
        version: result.version,
      })
    }

    // Validate credentials
    if (!credentials.api_key || credentials.api_key.trim() === '') {
      return NextResponse.json(
        { error: 'API key is required' },
        { status: 400 }
      )
    }

    const token = await getAdminAuthToken(request)
    const result = await updateProviderCredentials(providerId, credentials, version, token)
    
    // Return success without exposing the credentials
    return NextResponse.json({
      message: 'Credentials updated successfully',
      has_api_key: true,
      api_key_source: 'global_config',
      version: result.version,
    })
  } catch (error) {
    console.error(`Provider credentials error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update credentials' },
      { status: 500 }
    )
  }
}
