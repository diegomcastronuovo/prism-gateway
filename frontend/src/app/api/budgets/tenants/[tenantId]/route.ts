import { NextResponse } from 'next/server'
import {
  patchTenantConfigGeneric,
  GatewayAdminError,
  getAdminAuthToken,
} from '@/lib/server/gateway-admin-client'

export const dynamic = 'force-dynamic'

// PATCH /api/budgets/tenants/[tenantId] - Update tenant budget
export async function PATCH(
  request: Request,
  { params }: { params: { tenantId: string } }
) {
  try {
    const token = await getAdminAuthToken(request)
    const { tenantId } = params
    const body = await request.json()

    const {
      monthly_usd,
      timezone,
      enforcement_enabled,
      enforcement_mode,
      warn_pct,
      hard_pct,
      enforcement_paused,
      enforcement_pause_until,
      override_limit_usd,
      override_reason,
      version,
    } = body

    if (version === undefined) {
      return NextResponse.json(
        { error: 'Missing required field: version' },
        { status: 400 }
      )
    }

    // Build patch payload
    const patch: Record<string, unknown> = {}

    if (monthly_usd !== undefined || timezone !== undefined) {
      patch.budgets = {}
      if (monthly_usd !== undefined) {
        (patch.budgets as Record<string, unknown>).monthly_usd = monthly_usd
      }
      if (timezone !== undefined) {
        (patch.budgets as Record<string, unknown>).timezone = timezone
      }
    }

    if (
      enforcement_enabled !== undefined ||
      enforcement_mode !== undefined ||
      warn_pct !== undefined ||
      hard_pct !== undefined ||
      enforcement_paused !== undefined ||
      enforcement_pause_until !== undefined ||
      override_limit_usd !== undefined ||
      override_reason !== undefined
    ) {
      patch.budget_enforcement = {}
      if (enforcement_enabled !== undefined) {
        (patch.budget_enforcement as Record<string, unknown>).enabled = enforcement_enabled
      }
      if (enforcement_mode !== undefined) {
        (patch.budget_enforcement as Record<string, unknown>).mode = enforcement_mode
      }
      if (warn_pct !== undefined || hard_pct !== undefined) {
        (patch.budget_enforcement as Record<string, unknown>).thresholds = {}
        if (warn_pct !== undefined) {
          ((patch.budget_enforcement as Record<string, unknown>).thresholds as Record<string, unknown>).warn_pct = warn_pct
        }
        if (hard_pct !== undefined) {
          ((patch.budget_enforcement as Record<string, unknown>).thresholds as Record<string, unknown>).hard_pct = hard_pct
        }
      }
      // V2 fields
      if (enforcement_paused !== undefined) {
        (patch.budget_enforcement as Record<string, unknown>).paused = enforcement_paused
      }
      if (enforcement_pause_until !== undefined) {
        (patch.budget_enforcement as Record<string, unknown>).pause_until = enforcement_pause_until
      }
      if (override_limit_usd !== undefined) {
        (patch.budget_enforcement as Record<string, unknown>).override_limit_usd = override_limit_usd
      }
      if (override_reason !== undefined) {
        (patch.budget_enforcement as Record<string, unknown>).override_reason = override_reason
      }
    }

    const result = await patchTenantConfigGeneric(tenantId, patch, version, token)

    return NextResponse.json(result)
  } catch (error) {
    console.error(`Tenant budget update error:`, error)

    if (error instanceof GatewayAdminError) {
      return NextResponse.json(
        { error: error.message, details: error.details },
        { status: error.statusCode || 500 }
      )
    }

    return NextResponse.json(
      { error: 'Failed to update tenant budget' },
      { status: 500 }
    )
  }
}
