import { NextRequest, NextResponse } from 'next/server'
import { gatewayAdminFetch, GatewayAdminError } from '@/lib/server/gateway-admin-client'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export async function POST(
  request: NextRequest,
  { params }: { params: { requestId: string } }
) {
  try {
    const auth = await requireAdminBearer(request)
    if ('response' in auth) return auth.response
    const requestId = params.requestId
    const url = `/admin/replay/${encodeURIComponent(requestId)}?mode=deterministic`

    const response = await gatewayAdminFetch(url, {
      method: 'POST',
      requestAuthToken: auth.token,
    })

    const root = (response && typeof response === 'object')
      ? (response as Record<string, unknown>)
      : {}
    const replayResult = (
      root.replay_result && typeof root.replay_result === 'object'
        ? (root.replay_result as Record<string, unknown>)
        : root
    )
    const decisionSnapshot = (
      root.decision_snapshot && typeof root.decision_snapshot === 'object'
        ? (root.decision_snapshot as Record<string, unknown>)
        : (
          replayResult.decision_snapshot && typeof replayResult.decision_snapshot === 'object'
            ? (replayResult.decision_snapshot as Record<string, unknown>)
            : undefined
        )
    )
    const routingSnapshotFull = (
      root.routing_snapshot && typeof root.routing_snapshot === 'object'
        ? (root.routing_snapshot as Record<string, unknown>)
        : (
          root.routing_snapshot_full && typeof root.routing_snapshot_full === 'object'
            ? (root.routing_snapshot_full as Record<string, unknown>)
            : (
              replayResult.routing_snapshot && typeof replayResult.routing_snapshot === 'object'
                ? (replayResult.routing_snapshot as Record<string, unknown>)
                : undefined
            )
        )
    )
    const decisionReason = (
      typeof root.decision_reason === 'string'
        ? root.decision_reason
        : (
          typeof replayResult.decision_reason === 'string'
            ? replayResult.decision_reason
            : (
              decisionSnapshot && typeof decisionSnapshot.reason === 'string'
                ? decisionSnapshot.reason
                : undefined
            )
        )
    )

    return NextResponse.json({
      ...replayResult,
      decision_reason: decisionReason,
      decision_snapshot: decisionSnapshot,
      routing_snapshot_full: routingSnapshotFull,
    })
  } catch (error) {
    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: { message: error.message, type: 'gateway_error' } },
        { status: error.statusCode || 500 }
      )
    }
    console.error('Error running replay:', error)
    return NextResponse.json(
      { error: { message: 'Failed to run replay', type: 'internal_error' } },
      { status: 500 }
    )
  }
}
