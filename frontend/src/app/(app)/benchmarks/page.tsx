'use client'

import { useState, useCallback } from 'react'
import { PageHeader } from '@/components/layout/page-header'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  BarChart3,
  CheckCircle,
  Clock,
  DollarSign,
  Download,
  ShieldAlert,
  Target,
  Trash2,
} from 'lucide-react'
import { useAuth } from '@/hooks/use-auth'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import {
  useBenchmarkModels,
  type BenchmarkModelAggregate,
} from '@/features/benchmarks/api/use-benchmarks'
import { BenchmarksTable } from '@/features/benchmarks/components/benchmarks-table'
import { exportBenchmarksToCSV } from '@/features/benchmarks/utils/export-csv'

const WINDOW_OPTIONS = [
  { value: '1', label: '1h' },
  { value: '6', label: '6h' },
  { value: '24', label: '24h' },
  { value: '72', label: '72h' },
  { value: '168', label: '168h' },
]

function formatLatency(value: number) {
  return `${Math.round(value).toLocaleString()} ms`
}

function formatSuccessRate(value: number) {
  return `${(value * 100).toFixed(1)}%`
}

function formatCost(value: number) {
  return `$${value.toFixed(6)}`
}

function getAverage(values: number[]) {
  if (values.length === 0) return 0
  return values.reduce((acc, curr) => acc + curr, 0) / values.length
}

function getWeightedAverage(values: { value: number; weight: number }[]) {
  if (values.length === 0) return 0
  const totalWeight = values.reduce((acc, curr) => acc + curr.weight, 0)
  if (totalWeight === 0) return 0
  const weightedSum = values.reduce((acc, curr) => acc + curr.value * curr.weight, 0)
  return weightedSum / totalWeight
}

function getHealthStatus(successRate: number) {
  if (successRate >= 0.99) return 'healthy'
  if (successRate >= 0.8) return 'degraded'
  return 'failing'
}

function BarChartSection({
  title,
  rows,
  valueAccessor,
  valueLabel,
  className,
}: {
  title: string
  rows: BenchmarkModelAggregate[]
  valueAccessor: (row: BenchmarkModelAggregate) => number
  valueLabel: (value: number) => string
  className?: string
}) {
  const maxValue = rows.reduce((max, row) => Math.max(max, valueAccessor(row)), 0)

  return (
    <SectionCard title={title} className={className}>
      <div className="space-y-3">
        {rows.map((row) => {
          const value = valueAccessor(row)
          const width = maxValue > 0 ? Math.max((value / maxValue) * 100, 2) : 0

          return (
            <div key={row.model} className="space-y-1">
              <div className="flex items-center justify-between text-sm">
                <span className="font-medium">{row.model}</span>
                <span className="text-muted-foreground">{valueLabel(value)}</span>
              </div>
              <div className="h-2 w-full rounded-full bg-muted">
                <div
                  className="h-2 rounded-full bg-primary"
                  style={{ width: `${width}%` }}
                />
              </div>
            </div>
          )
        })}
      </div>
    </SectionCard>
  )
}

function BenchmarksContent() {
  const [windowHours, setWindowHours] = useState(24)
  const [isClearing, setIsClearing] = useState(false)
  const { user, isRefreshingSession } = useAuth()
  const benchmarkQuery = useBenchmarkModels(windowHours, !!user && !isRefreshingSession)

  const handleClearHistory = useCallback(async () => {
    if (!confirm('Clear all benchmark history? This cannot be undone.')) return
    setIsClearing(true)
    try {
      await fetch('/api/benchmarks/models', { method: 'DELETE', credentials: 'include' })
      await benchmarkQuery.refetch()
    } finally {
      setIsClearing(false)
    }
  }, [benchmarkQuery])

  if (benchmarkQuery.isError) {
    const err = benchmarkQuery.error as Error & { status?: number }
    const isAccessDenied = err.status === 403 || err.status === 401
    return (
      <div>
        <PageHeader
          title="Benchmarks"
          description="Automatic benchmark analytics from gateway aggregates"
        />
        <SectionCard title={isAccessDenied ? 'Access limited' : 'Error'} className="border-t-4 border-t-rose-500">
          {isAccessDenied ? (
            <EmptyState
              icon={ShieldAlert}
              title="Insufficient permissions"
              description="Your role cannot view this page. This page only shows data when the gateway allows access to this global area."
            />
          ) : (
            <div className="text-center py-8">
              <p className="text-destructive mb-2">Failed to load benchmark data</p>
              <p className="text-sm text-muted-foreground">{benchmarkQuery.error.message}</p>
            </div>
          )}
        </SectionCard>
      </div>
    )
  }

  const rows = benchmarkQuery.data ?? []
  const isLoading = !!user && benchmarkQuery.isLoading

  const totalModels = rows.length

  // Filter healthy/degraded models for Best Of; fall back to all rows if every model is failing
  const nonFailingRows = rows.filter((row) => getHealthStatus(row.success_rate) !== 'failing')
  const bestOfRows = nonFailingRows.length > 0 ? nonFailingRows : rows
  const bestOfFallback = nonFailingRows.length === 0 && rows.length > 0

  // Avg Latency: weighted by samples across ALL models (including failing — latency is still measured)
  const avgLatency = getWeightedAverage(
    rows.map((row) => ({ value: row.avg_latency_ms, weight: row.samples }))
  )

  // Success Rate: weighted average by samples
  const avgSuccessRate = getWeightedAverage(
    rows.map((row) => ({ value: row.success_rate, weight: row.samples }))
  )

  // Best Of: prefer healthy/degraded; fall back to all when every model is failing
  const cheapestModel = bestOfRows.reduce<BenchmarkModelAggregate | null>(
    (best, row) => (best === null || row.avg_cost_usd < best.avg_cost_usd ? row : best),
    null
  )
  const fastestModel = bestOfRows.reduce<BenchmarkModelAggregate | null>(
    (best, row) => (best === null || row.avg_latency_ms < best.avg_latency_ms ? row : best),
    null
  )
  const mostReliableModel = bestOfRows.reduce<BenchmarkModelAggregate | null>(
    (best, row) => (best === null || row.success_rate > best.success_rate ? row : best),
    null
  )

  return (
    <div>
      <PageHeader
        title="Benchmarks"
        description="Read-only analytics from automatic benchmark cycles"
      />

      <div className="mb-6 flex justify-end gap-2">
        <Select
          value={windowHours.toString()}
          onValueChange={(value) => setWindowHours(parseInt(value, 10))}
        >
          <SelectTrigger className="w-[120px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {WINDOW_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          size="sm"
          onClick={() => exportBenchmarksToCSV(rows, windowHours)}
          disabled={rows.length === 0}
          title="Exports current benchmark view as CSV"
        >
          <Download className="h-4 w-4 mr-1" />
          Export CSV
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={handleClearHistory}
          disabled={isClearing || rows.length === 0}
          title="Delete all benchmark history and start fresh"
        >
          <Trash2 className="h-4 w-4 mr-1" />
          {isClearing ? 'Clearing...' : 'Clear History'}
        </Button>
      </div>

      <div className="grid gap-6 md:grid-cols-4 mb-6">
        {isLoading ? (
          <>
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
          </>
        ) : (
          <>
            <StatCard
              title="Total Models"
              value={totalModels.toString()}
              icon={BarChart3}
              description="Models benchmarked"
              className="border-t-4 border-t-pink-500"
            />
            <StatCard
              title="Avg Latency"
              value={totalModels === 0 ? '—' : formatLatency(avgLatency)}
              icon={Clock}
              description="Across all models"
              className="border-t-4 border-t-cyan-400"
            />
            <StatCard
              title="Success Rate"
              value={totalModels === 0 ? '—' : formatSuccessRate(avgSuccessRate)}
              icon={CheckCircle}
              description="Average reliability"
              className="border-t-4 border-t-emerald-500"
            />
            <StatCard
              title="Lowest Cost Model"
              value={cheapestModel?.model || '—'}
              icon={DollarSign}
              description={cheapestModel ? formatCost(cheapestModel.avg_cost_usd) : '—'}
              className="border-t-4 border-t-amber-400"
            />
          </>
        )}
      </div>

      {isLoading ? (
        <div className="grid gap-6 mb-6 md:grid-cols-3">
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
        </div>
      ) : rows.length > 0 ? (
        <>
          <SectionCard title="Best Of" className="border-t-4 border-t-purple-500">
            <div className="grid gap-4 md:grid-cols-3">
              <div className="rounded-md border p-4">
                <p className="text-xs text-muted-foreground mb-1">Fastest Model</p>
                <p className="font-medium">{fastestModel?.model || '—'}</p>
              </div>
              <div className="rounded-md border p-4">
                <p className="text-xs text-muted-foreground mb-1">Most Reliable Model</p>
                <p className="font-medium">{mostReliableModel?.model || '—'}</p>
              </div>
              <div className="rounded-md border p-4">
                <p className="text-xs text-muted-foreground mb-1">Cheapest Model</p>
                <p className="font-medium">{cheapestModel?.model || '—'}</p>
              </div>
            </div>
            <p className="text-xs text-muted-foreground mt-3 text-center">
              {bestOfFallback
                ? 'All models are currently failing — showing relative best across all models'
                : 'Best Of metrics prefer healthy/degraded models'}
            </p>
          </SectionCard>

          <div className="grid gap-6 mb-6 md:grid-cols-3">
            <BarChartSection
              title="Avg Latency by Model"
              rows={[...rows].sort((a, b) => a.avg_latency_ms - b.avg_latency_ms)}
              valueAccessor={(row) => row.avg_latency_ms}
              valueLabel={formatLatency}
              className="border-t-4 border-t-blue-500"
            />
            <BarChartSection
              title="Success Rate by Model"
              rows={[...rows].sort((a, b) => b.success_rate - a.success_rate)}
              valueAccessor={(row) => row.success_rate}
              valueLabel={formatSuccessRate}
              className="border-t-4 border-t-emerald-500"
            />
            <BarChartSection
              title="Average Cost by Model"
              rows={[...rows].sort((a, b) => a.avg_cost_usd - b.avg_cost_usd)}
              valueAccessor={(row) => row.avg_cost_usd}
              valueLabel={formatCost}
              className="border-t-4 border-t-amber-400"
            />
          </div>
        </>
      ) : null}

      <SectionCard title="Benchmark Results" className="border-t-4 border-t-cyan-400">
        {isLoading ? (
          <div className="space-y-2">
            {[...Array(3)].map((_, i) => (
              <Skeleton key={i} className="h-16 w-full" />
            ))}
          </div>
        ) : rows.length === 0 ? (
          <EmptyState
            icon={Target}
            title="No benchmark data available yet"
            description="Benchmarking data is collected automatically. Please wait for the next benchmark cycle."
          />
        ) : (
          <BenchmarksTable rows={rows} />
        )}
      </SectionCard>
    </div>
  )
}

export default function BenchmarksPage() {
  return (
    <RequireAdminRole>
      <BenchmarksContent />
    </RequireAdminRole>
  )
}
