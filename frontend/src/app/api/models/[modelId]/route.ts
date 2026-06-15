import { NextResponse } from 'next/server'
import { getModels, getGlobalConfig, updateModel, deleteModel, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

// GET /api/models/[modelId] - Get single model with version from global config
export async function GET(
  request: Request,
  { params }: { params: { modelId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { modelId } = params
    
    // Fetch model from runtime endpoint
    const result = await getModels(auth.token)
    const models = ((result as { data?: unknown[] }).data || []) as Array<Record<string, unknown>>
    const model = models.find((m) => m.id === modelId)
    
    if (!model) {
      return NextResponse.json(
        { error: `Model ${modelId} not found` },
        { status: 404 }
      )
    }

    // Fetch version and Enabled from global config
    // (/admin/models catalog endpoint does not include Enabled)
    const globalConfig = await getGlobalConfig(auth.token)
    const globalVersion = globalConfig.version || 1
    const globalModels = Array.isArray(globalConfig.config?.models)
      ? (globalConfig.config.models as Array<Record<string, unknown>>)
      : []
    const globalEntry = globalModels.find(
      (m) => String(m.Name ?? m.name ?? '') === modelId
    )

    return NextResponse.json({
      data: {
        ...model,
        Enabled: globalEntry !== undefined ? globalEntry.Enabled : model.Enabled,
        version: globalVersion
      }
    })
  } catch (error) {
    console.error(`Model API error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to fetch model' },
      { status: 500 }
    )
  }
}

// PATCH /api/models/[modelId] - Update model
export async function PATCH(
  request: Request,
  { params }: { params: { modelId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { modelId } = params
    const body = await request.json()
    
    // Fetch current global config version (ignore client version - always use latest)
    const globalConfig = await getGlobalConfig(auth.token)
    const currentVersion = globalConfig.version || 1
    
    const result = await updateModel(modelId, body, currentVersion, auth.token)
    return NextResponse.json(result)
  } catch (error) {
    console.error(`Model patch error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update model' },
      { status: 500 }
    )
  }
}

// DELETE /api/models/[modelId] - Delete model
export async function DELETE(
  request: Request,
  { params }: { params: { modelId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response

    const { modelId } = params
    
    // Fetch current global config version
    const globalConfig = await getGlobalConfig(auth.token)
    const currentVersion = globalConfig.version || 1
    
    await deleteModel(modelId, currentVersion, auth.token)
    // Backend returns 204 No Content, return simple success
    return NextResponse.json({ message: 'Model deleted successfully' })
  } catch (error) {
    console.error(`Model delete error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to delete model' },
      { status: 500 }
    )
  }
}
