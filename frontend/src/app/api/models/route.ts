import { NextResponse } from 'next/server'
import { getModels, createModel, updateModel, getGlobalConfig, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/models - List all models
export async function GET(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const result = await getModels(auth.token)
    return NextResponse.json({ data: (result as { data?: unknown[] }).data || [] })
  } catch (error) {
    console.error('Models API error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch models' },
      { status: 500 }
    )
  }
}

// POST /api/models - Create new model
export async function POST(request: Request) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const body = await request.json()
    
    if (!body.id || !body.provider) {
      return NextResponse.json(
        { error: 'Missing required fields: id and provider' },
        { status: 400 }
      )
    }
    
    // Fetch current global config version
    const globalConfig = await getGlobalConfig(auth.token)
    const currentVersion = globalConfig.version || 1
    
    const result = await createModel(body, currentVersion, auth.token)

    // POST /admin/models does NOT persist provider_model_id — the Go createModelBody
    // struct doesn't have that field and json.Decode silently drops it.
    // For Bedrock, immediately PATCH to inject it: PATCH uses a free-form map-merge
    // that does handle provider_model_id correctly.
    if (body.provider === 'bedrock') {
      const pid: string =
        typeof body.provider_model_id === 'string' && body.provider_model_id.trim()
          ? body.provider_model_id.trim()
          : String(body.id).trim()
      const newVersion = (result as { version?: number }).version
      if (pid && newVersion != null) {
        try {
          await updateModel(
            body.id,
            { provider_model_id: pid } as Record<string, unknown>,
            newVersion,
            auth.token
          )
        } catch (patchErr) {
          // Non-fatal — model was created, only provider_model_id is missing.
          console.error('[createModel] bedrock provider_model_id patch failed', patchErr)
        }
      }
    }

    return NextResponse.json(result, { status: 201 })
  } catch (error) {
    console.error('Model create error:', error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to create model' },
      { status: 500 }
    )
  }
}
