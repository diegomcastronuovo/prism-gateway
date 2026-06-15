import type { TenantBudget } from '../api/use-budgets'

function escapeCSV(value: string | number | null | undefined): string {
  if (value === null || value === undefined) return ''
  const stringValue = String(value)
  if (stringValue.includes(',') || stringValue.includes('"') || stringValue.includes('\n')) {
    return `"${stringValue.replace(/"/g, '""')}"`
  }
  return stringValue
}

export function exportBudgetsToCSV(
  budgets: TenantBudget[],
  windowHours: number = 720,
  calculateProjectedSpend?: (currentSpend: number) => number
) {
  const now = new Date()
  const generatedAt = now.toISOString()

  const projectedFn = calculateProjectedSpend ?? ((currentSpend: number) => {
    const elapsedFractionMap: Record<number, number> = {
      24: 0.1,
      168: 0.3,
      720: 0.5,
    }
    const elapsedFraction = elapsedFractionMap[windowHours] ?? 0.5
    return elapsedFraction > 0 ? currentSpend / elapsedFraction : currentSpend
  })

  const headers = [
    'generated_at',
    'window',
    'tenant_id',
    'spend_usd',
    'limit_usd',
    'progress_pct',
    'projected_spend_usd',
    'status',
    'override',
  ]

  const rows = budgets.map((budget) => {
    const hasOverride = budget.override_limit_usd !== null && budget.override_limit_usd !== undefined
    const isPaused = budget.enforcement_paused === true
    const effectiveLimit = hasOverride ? (budget.override_limit_usd ?? budget.monthly_usd ?? 0) : (budget.monthly_usd ?? 0)
    const progressPct = effectiveLimit > 0 ? (budget.current_spend_usd / effectiveLimit) * 100 : 0
    const projectedSpend = projectedFn(budget.current_spend_usd)

    let overrideState = 'none'
    if (isPaused) {
      overrideState = 'paused'
    } else if (hasOverride) {
      overrideState = 'override'
    }

    return [
      escapeCSV(generatedAt),
      escapeCSV(`${windowHours}h`),
      escapeCSV(budget.tenant_id),
      escapeCSV(budget.current_spend_usd.toFixed(6)),
      escapeCSV(effectiveLimit.toFixed(6)),
      escapeCSV(progressPct.toFixed(3)),
      escapeCSV(projectedSpend.toFixed(6)),
      escapeCSV(budget.status),
      escapeCSV(overrideState),
    ].join(',')
  })

  const csv = [headers.join(','), ...rows].join('\n')

  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  
  const dateStr = now.toISOString().split('T')[0]
  link.download = `budgets_${dateStr}.csv`
  
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}
