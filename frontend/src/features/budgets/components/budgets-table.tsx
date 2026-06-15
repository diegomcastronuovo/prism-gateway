'use client'

import { useState } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatCurrency } from '@/lib/utils/format'
import { Pencil, ArrowUpDown, Download, HelpCircle, Globe } from 'lucide-react'
import type { TenantBudget } from '../api/use-budgets'
import { cn } from '@/lib/utils/cn'
import { exportBudgetsToCSV } from '../utils/export-csv'

interface BudgetsTableProps {
  budgets: TenantBudget[]
  onEdit: (budget: TenantBudget) => void
  calculateProjectedSpend?: (currentSpend: number) => number
  windowHours?: number
}

type SortField = 'spend' | 'limit' | 'usage_pct' | 'status' | 'projected'
type SortDirection = 'asc' | 'desc'

function getStatusVariant(status: string): 'default' | 'secondary' | 'destructive' {
  if (status === 'healthy') return 'default'
  if (status === 'warning') return 'secondary'
  return 'destructive'
}

// use shared formatCurrency from utils

function formatProgressPct(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0.0%'
  if (value < 0.01) return `${value.toFixed(3)}%`
  return `${value.toFixed(1)}%`
}

export function BudgetsTable({
  budgets,
  onEdit,
  calculateProjectedSpend: calculateProjectedSpendProp,
  windowHours = 720,
}: BudgetsTableProps) {
  const [sortField, setSortField] = useState<SortField>('usage_pct')
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc')

  const calculateProjectedSpend = calculateProjectedSpendProp ?? ((currentSpend: number): number => {
    const elapsedFraction = 0.5 // Placeholder - would come from backend
    return elapsedFraction > 0 ? currentSpend / elapsedFraction : currentSpend
  })

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDirection('desc')
    }
  }

  const sortedBudgets = budgets
    .map((budget, index) => ({ budget, index }))
    .sort((left, right) => {
    const a = left.budget
    const b = right.budget
    const effectiveLimitA = a.monthly_usd ?? 0
    const effectiveLimitB = b.monthly_usd ?? 0
    const effectiveSpendA = a.effective_spend_usd ?? a.current_spend_usd
    const effectiveSpendB = b.effective_spend_usd ?? b.current_spend_usd
    const usagePctA = effectiveLimitA > 0 ? (effectiveSpendA / effectiveLimitA) * 100 : 0
    const usagePctB = effectiveLimitB > 0 ? (effectiveSpendB / effectiveLimitB) * 100 : 0

    let comparison = 0
    if (sortField === 'spend') {
      comparison = effectiveSpendA - effectiveSpendB
    } else if (sortField === 'limit') {
      comparison = effectiveLimitA - effectiveLimitB
    } else if (sortField === 'usage_pct') {
      comparison = usagePctA - usagePctB
    } else if (sortField === 'status') {
      const statusOrder = { exceeded: 3, warning: 2, healthy: 1, not_configured: 0 }
      comparison = statusOrder[a.status] - statusOrder[b.status]
    } else if (sortField === 'projected') {
      const projectedA = calculateProjectedSpend(a.current_spend_usd)
      const projectedB = calculateProjectedSpend(b.current_spend_usd)
      comparison = projectedA - projectedB
    }

    if (comparison === 0) {
      return left.index - right.index
    }
    return sortDirection === 'asc' ? comparison : -comparison
  })
  .map((item) => item.budget)

  return (
    <div className="border rounded-md">
      <div className="flex justify-end p-2 border-b">
        <Button
          variant="outline"
          size="sm"
          onClick={() => exportBudgetsToCSV(sortedBudgets, windowHours, calculateProjectedSpend)}
          className="gap-2"
          disabled={sortedBudgets.length === 0}
        >
          <Download className="h-4 w-4" />
          Export CSV
        </Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Tenant</TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleSort('spend')}
                className="h-8 px-2"
              >
                Spend
                <ArrowUpDown className="ml-1 h-3 w-3" />
                <span title="Effective spend = confirmed spend + in-flight reservations. Reserved amounts are released after successful request completion.">
                  <HelpCircle className="ml-1 h-3 w-3 text-muted-foreground" />
                </span>
              </Button>
            </TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleSort('limit')}
                className="h-8 px-2"
              >
                Limit
                <ArrowUpDown className="ml-1 h-3 w-3" />
              </Button>
            </TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleSort('usage_pct')}
                className="h-8 px-2"
              >
                Progress
                <ArrowUpDown className="ml-1 h-3 w-3" />
              </Button>
            </TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleSort('projected')}
                className="h-8 px-2"
              >
                Projected
                <ArrowUpDown className="ml-1 h-3 w-3" />
                <span title="Projected spend is estimated by extrapolating current spend rate over the selected window.">
                  <HelpCircle className="ml-1 h-3 w-3 text-muted-foreground" />
                </span>
              </Button>
            </TableHead>
            <TableHead>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleSort('status')}
                className="h-8 px-2"
              >
                Status
                <ArrowUpDown className="ml-1 h-3 w-3" />
              </Button>
            </TableHead>
            <TableHead>Timezone</TableHead>
            <TableHead className="w-8"></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sortedBudgets.map((budget) => {
            const effectiveLimit = budget.monthly_usd ?? 0
            const effectiveSpend = budget.effective_spend_usd ?? budget.current_spend_usd
            const reservedUsd = budget.reserved_usd ?? 0
            const usagePct = effectiveLimit > 0 ? (effectiveSpend / effectiveLimit) * 100 : 0
            const projectedSpend = calculateProjectedSpend(budget.current_spend_usd)
            const isProjectedOverrun = projectedSpend >= effectiveLimit

            return (
              <TableRow key={budget.tenant_id}>
                <TableCell className="font-medium">{budget.tenant_id}</TableCell>
                <TableCell className="tabular-nums">
                  <div>
                    <span>{formatCurrency(effectiveSpend)} / {formatCurrency(effectiveLimit)}</span>
                    {reservedUsd > 0 && (
                      <div
                        className="text-xs text-muted-foreground mt-0.5"
                        title="Reserved amounts are temporary allocations for in-flight requests and are released after successful completion."
                      >
                        {formatCurrency(budget.current_spend_usd)} + {formatCurrency(reservedUsd)} reserved
                      </div>
                    )}
                  </div>
                </TableCell>
                <TableCell>
                  {budget.monthly_usd == null ? '—' : formatCurrency(budget.monthly_usd)}
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-3">
                    <span className="font-medium tabular-nums text-sm min-w-[3ch]">{formatProgressPct(usagePct)}</span>
                    <span
                      className="text-xs text-muted-foreground tabular-nums whitespace-nowrap"
                      title={reservedUsd > 0 ? `Confirmed: ${formatCurrency(budget.current_spend_usd)} + Reserved: ${formatCurrency(reservedUsd)}` : undefined}
                    >
                      ({formatCurrency(effectiveSpend)} / {formatCurrency(effectiveLimit)})
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <span className={cn(
                    'tabular-nums',
                    isProjectedOverrun && 'text-yellow-600 font-medium'
                  )}>
                    {formatCurrency(projectedSpend)}
                  </span>
                </TableCell>
                <TableCell>
                  <Badge variant={getStatusVariant(budget.status)}>{budget.status}</Badge>
                </TableCell>
                <TableCell>
                  {budget.timezone ? (
                    <span className="flex items-center gap-1 text-sm text-muted-foreground">
                      <Globe className="h-3 w-3 shrink-0" />
                      {budget.timezone}
                    </span>
                  ) : (
                    <span className="text-sm text-muted-foreground">UTC</span>
                  )}
                </TableCell>
                <TableCell>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => onEdit(budget)}
                    title={`Edit limit / timezone${budget.timezone ? ` (${budget.timezone})` : ''}`}
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
