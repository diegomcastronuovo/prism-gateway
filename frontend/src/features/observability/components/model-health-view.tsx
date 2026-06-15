'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, ArrowDown, ArrowLeft, ArrowUp, ArrowUpDown, Download, HeartPulse, ShieldCheck, Timer, Wallet } from 'lucide-react'
import { useAuth } from '@/hooks/use-auth'
import { useObservabilityGlobalAccess } from '@/features/observability/api/use-observability-access'
import { useModelHealthUsage, type ModelHealthUsageRow } from '@/features/observability/api/use-model-health-usage'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard } from '@/components/shared/section-card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { formatCurrency } from '@/lib/utils/format'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Scatter,
  ScatterChart,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

interface ModelHealthViewProps {
  onBack: () => void
}

type HealthStatus = 'healthy' | 'degraded' | 'failing'
type RecommendationState = 'recommended' | 'eligible' | 'excluded'
type ModelHealthSortDirection = 'asc' | 'desc'
type ModelHealthSortField =
  | 'model'
  | 'provider'
  | 'model_type'
  | 'avg_latency_ms'
  | 'p95_latency_ms'
  | 'success_rate'
  | 'avg_cost_usd'
  | 'requests'
  | 'status'

type ModelHealthRow = ModelHealthUsageRow & {
  status: HealthStatus
  cost_per_latency: number
}

const WINDOW_OPTIONS = [
  { label: '24h', value: 24 },
  { label: '72h', value: 72 },
  { label: '7d', value: 168 },
] as const

const STATUS_SORT_WEIGHT: Record<HealthStatus, number> = {
  healthy: 3,
  degraded: 2,
  failing: 1,
}

function getStatus(successRate: number): HealthStatus {
  if (successRate >= 0.98) return 'healthy'
  if (successRate >= 0.9) return 'degraded'
  return 'failing'
}

function getStatusBadgeClassName(status: HealthStatus): string {
  if (status === 'healthy') {
    return 'bg-green-100 text-green-800 border-green-200 hover:bg-green-100'
  }
  if (status === 'degraded') {
    return 'bg-yellow-100 text-yellow-800 border-yellow-200 hover:bg-yellow-100'
  }
  return 'bg-red-100 text-red-800 border-red-200 hover:bg-red-100'
}

// use shared formatCurrency from utils

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return '0.0%'
  return `${(value * 100).toFixed(1)}%`
}

function modelRowKey(row: Pick<ModelHealthRow, 'model' | 'provider'>): string {
  return `${row.model}::${row.provider}`
}

function getRecommendationBadgeClassName(state: RecommendationState): string {
  if (state === 'recommended') {
    return 'bg-blue-100 text-blue-800 border-blue-200 hover:bg-blue-100'
  }
  if (state === 'eligible') {
    return 'bg-emerald-100 text-emerald-800 border-emerald-200 hover:bg-emerald-100'
  }
  return 'bg-slate-100 text-slate-700 border-slate-200 hover:bg-slate-100'
}

export function ModelHealthView({ onBack }: ModelHealthViewProps) {
  const { user, isRefreshingSession } = useAuth()
  const accessQuery = useObservabilityGlobalAccess()
  // Do not wait for access probe success: while the probe is pending, isSuccess is false and the
  // query would never run (deadlock with slow probes). Run whenever the user is signed in and the
  // access probe has not failed; FinOps/benchmark calls surface 401/403 via TanStack Query.
  const canFetch = Boolean(user && !isRefreshingSession && !accessQuery.isError)

  const [windowHours, setWindowHours] = useState<number>(24)
  const [providerFilter, setProviderFilter] = useState<string>('all')
  const [statusFilter, setStatusFilter] = useState<'all' | HealthStatus>('all')
  const [sortField, setSortField] = useState<ModelHealthSortField>('success_rate')
  const [sortDirection, setSortDirection] = useState<ModelHealthSortDirection>('desc')
  const [explainRow, setExplainRow] = useState<ModelHealthRow | null>(null)
  const [explainOpen, setExplainOpen] = useState(false)
  const { data, isLoading } = useModelHealthUsage(windowHours, canFetch)
  const isPageLoading = accessQuery.isLoading || isLoading

  const previousRowsRef = useRef<ModelHealthRow[] | null>(null)
  const previousWindowRef = useRef<number>(windowHours)
  const [latencyRegressionModels, setLatencyRegressionModels] = useState<string[]>([])

  const rows = useMemo<ModelHealthRow[]>(() => {
    return (data ?? []).map((item) => ({
      ...item,
      status: getStatus(item.success_rate),
      cost_per_latency:
        Number(item.avg_latency_ms) > 0 ? Number(item.avg_cost_usd) / Number(item.avg_latency_ms) : Number.POSITIVE_INFINITY,
    }))
  }, [data])

  const providerOptions = useMemo(
    () => Array.from(new Set(rows.map((row) => row.provider))).sort((a, b) => a.localeCompare(b)),
    [rows]
  )

  const filteredRows = useMemo(() => {
    return rows.filter((row) => {
      if (providerFilter !== 'all' && row.provider !== providerFilter) return false
      if (statusFilter !== 'all' && row.status !== statusFilter) return false
      return true
    })
  }, [rows, providerFilter, statusFilter])

  const sortedRows = useMemo(() => {
    const compareValues = (a: ModelHealthRow, b: ModelHealthRow): number => {
      if (sortField === 'model' || sortField === 'provider' || sortField === 'model_type') {
        const left = a[sortField]
        const right = b[sortField]
        return left.localeCompare(right)
      }

      if (sortField === 'requests') {
        return a.samples - b.samples
      }

      if (sortField === 'status') {
        return STATUS_SORT_WEIGHT[a.status] - STATUS_SORT_WEIGHT[b.status]
      }

      const left = Number(a[sortField])
      const right = Number(b[sortField])
      return left - right
    }

    const sorted = [...filteredRows].sort((a, b) => {
      const primary = compareValues(a, b)
      if (primary !== 0) {
        return sortDirection === 'asc' ? primary : -primary
      }

      if (a.success_rate !== b.success_rate) {
        return b.success_rate - a.success_rate
      }

      if (a.avg_latency_ms !== b.avg_latency_ms) {
        return a.avg_latency_ms - b.avg_latency_ms
      }

      return a.model.localeCompare(b.model)
    })

    return sorted
  }, [filteredRows, sortDirection, sortField])

  useEffect(() => {
    if (rows.length === 0) return

    if (previousRowsRef.current && previousWindowRef.current !== windowHours) {
      const previousByModel = new Map(previousRowsRef.current.map((row) => [row.model, row]))
      const regressions = rows
        .filter((row) => {
          const prev = previousByModel.get(row.model)
          if (!prev || prev.avg_latency_ms <= 0) return false
          return row.avg_latency_ms > prev.avg_latency_ms * 1.3
        })
        .map((row) => row.model)

      setLatencyRegressionModels(regressions)
    }

    previousRowsRef.current = rows
    previousWindowRef.current = windowHours
  }, [rows, windowHours])

  const fastestModel = useMemo(
    () => sortedRows.reduce<ModelHealthRow | null>((best, row) => (!best || row.avg_latency_ms < best.avg_latency_ms ? row : best), null),
    [sortedRows]
  )

  const cheapestModel = useMemo(
    () =>
      sortedRows
        .filter((row) => row.samples >= 1)
        .reduce<ModelHealthRow | null>((best, row) => (!best || row.avg_cost_usd < best.avg_cost_usd ? row : best), null),
    [sortedRows]
  )

  const mostReliableModel = useMemo(
    () => sortedRows.reduce<ModelHealthRow | null>((best, row) => (!best || row.success_rate > best.success_rate ? row : best), null),
    [sortedRows]
  )

  const failingModelsCount = useMemo(() => sortedRows.filter((row) => row.success_rate < 0.9).length, [sortedRows])

  const hasDegradedProviderAlert = useMemo(() => sortedRows.some((row) => row.success_rate < 0.9), [sortedRows])

  const recommendationCandidates = useMemo(
    () => sortedRows.filter((row) => row.status !== 'failing' && row.success_rate >= 0.9 && row.success_rate > 0),
    [sortedRows]
  )

  const cheapestEligibleModel = useMemo(
    () =>
      recommendationCandidates.reduce<ModelHealthRow | null>(
        (best, row) => (!best || row.avg_cost_usd < best.avg_cost_usd ? row : best),
        null
      ),
    [recommendationCandidates]
  )

  const recommendedModel = useMemo(() =>
    recommendationCandidates.reduce<ModelHealthRow | null>((best, row) => {
        if (!Number.isFinite(row.cost_per_latency)) return best
        if (row.cost_per_latency <= 0) return best
        if (!best) return row
        return row.cost_per_latency < best.cost_per_latency ? row : best
      }, null), [recommendationCandidates])

  const noRecommendationAvailable = sortedRows.length > 0 && recommendationCandidates.length === 0

  const isRecommendationEligible = (row: ModelHealthRow): boolean =>
    row.status !== 'failing' && row.success_rate >= 0.9 && row.success_rate > 0

  const getRecommendationState = (row: ModelHealthRow): RecommendationState => {
    if (!isRecommendationEligible(row)) return 'excluded'
    if (recommendedModel && modelRowKey(row) === modelRowKey(recommendedModel)) return 'recommended'
    return 'eligible'
  }

  const reliabilityRanking = useMemo(
    () =>
      [...sortedRows].sort(
        (a, b) => b.success_rate - a.success_rate || a.avg_latency_ms - b.avg_latency_ms || a.model.localeCompare(b.model)
      ),
    [sortedRows]
  )
  const latencyRanking = useMemo(
    () =>
      [...sortedRows].sort(
        (a, b) => a.avg_latency_ms - b.avg_latency_ms || b.success_rate - a.success_rate || a.model.localeCompare(b.model)
      ),
    [sortedRows]
  )
  const costRanking = useMemo(
    () =>
      [...sortedRows].sort(
        (a, b) => a.avg_cost_usd - b.avg_cost_usd || b.success_rate - a.success_rate || a.model.localeCompare(b.model)
      ),
    [sortedRows]
  )

  const getRankLabel = (ranking: ModelHealthRow[], row: ModelHealthRow | null): string => {
    if (!row || ranking.length === 0) return '—'
    const index = ranking.findIndex((item) => modelRowKey(item) === modelRowKey(row))
    if (index < 0) return '—'
    return `${index + 1} of ${ranking.length}`
  }

  const statusExplanation = useMemo(() => {
    if (!explainRow) return ''
    if (explainRow.success_rate >= 0.98) {
      return 'This model is marked healthy because its success rate is at least 98%.'
    }
    if (explainRow.success_rate >= 0.9) {
      return 'This model is marked degraded because its success rate is below 98% but still above 90%.'
    }
    return 'This model is marked failing because its success rate is below 90%.'
  }, [explainRow])

  const recommendationExplanation = useMemo(() => {
    if (!explainRow) return ''
    if (!isRecommendationEligible(explainRow)) {
      if (explainRow.status === 'failing') return 'This model is not recommended because it is failing.'
      if (explainRow.success_rate === 0) return 'This model is not recommended because its success rate is 0%.'
      return 'This model is not recommended because its success rate is below the minimum threshold.'
    }
    if (recommendedModel && modelRowKey(explainRow) === modelRowKey(recommendedModel)) {
      return 'This model is recommended because, among eligible models, it currently offers the best balance of reliability, latency, and cost.'
    }
    return 'This model is eligible, but another model currently offers a better balance of reliability, latency, and cost.'
  }, [explainRow, recommendedModel])

  const recommendationDetails = useMemo(() => {
    if (!explainRow) return ''
    if (!isRecommendationEligible(explainRow)) {
      return 'Excluded from recommendation because failing models cannot be recommended.'
    }
    if (recommendedModel && modelRowKey(explainRow) === modelRowKey(recommendedModel)) {
      return 'Recommended because it is eligible and currently offers the best overall tradeoff between reliability, latency, and cost.'
    }
    return 'Not recommended because a better eligible model exists in the current usage window.'
  }, [explainRow, recommendedModel])

  const exportCsv = () => {
    const headers = [
      'model',
      'provider',
      'model_type',
      'requests',
      'effective_spend',
      'avg_cost_per_request_effective',
      'avg_latency_ms',
      'p95_latency_ms',
      'success_rate',
      'status',
    ]
    const escapeCell = (value: string) => `"${value.replace(/"/g, '""')}"`
    const body = sortedRows.map((row) => [
      row.model,
      row.provider,
      row.model_type,
      String(row.samples),
      String(row.effective_spend),
      String(row.avg_cost_usd),
      String(row.avg_latency_ms),
      String(row.p95_latency_ms),
      String(row.success_rate),
      row.status,
    ])

    const csv = [
      headers.map(escapeCell).join(','),
      ...body.map((line) => line.map((cell) => escapeCell(cell)).join(',')),
    ].join('\n')

    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const now = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    const fileName = `model_health_${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}.csv`

    const link = document.createElement('a')
    link.href = url
    link.download = fileName
    link.click()
    URL.revokeObjectURL(url)
  }

  const handleSort = (field: ModelHealthSortField) => {
    if (field === sortField) {
      setSortDirection((current) => (current === 'asc' ? 'desc' : 'asc'))
      return
    }

    setSortField(field)
    setSortDirection(
      field === 'model' || field === 'provider' || field === 'model_type' ? 'asc' : 'desc'
    )
  }

  const formatModelTick = (value: string) => {
    if (value.length <= 18) return value
    return `${value.slice(0, 16)}…`
  }

  const SortHeader = ({ field, label }: { field: ModelHealthSortField; label: string }) => (
    <button
      type="button"
      className="inline-flex items-center gap-1 text-left hover:text-foreground"
      onClick={() => handleSort(field)}
    >
      <span>{label}</span>
      {sortField !== field ? (
        <ArrowUpDown className="h-3.5 w-3.5 text-muted-foreground" />
      ) : sortDirection === 'asc' ? (
        <ArrowUp className="h-3.5 w-3.5" />
      ) : (
        <ArrowDown className="h-3.5 w-3.5" />
      )}
    </button>
  )

  const openExplainDrawer = (row: ModelHealthRow) => {
    setExplainRow(row)
    setExplainOpen(true)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="outline" onClick={onBack}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Observability
        </Button>
        <h2 className="text-2xl font-bold">Model Health</h2>
      </div>

      <SectionCard
        title="Window"
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Select value={String(windowHours)} onValueChange={(v) => setWindowHours(Number(v))}>
              <SelectTrigger className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WINDOW_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={String(option.value)}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={providerFilter} onValueChange={setProviderFilter}>
              <SelectTrigger className="w-44">
                <SelectValue placeholder="All providers" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All providers</SelectItem>
                {providerOptions.map((provider) => (
                  <SelectItem key={provider} value={provider}>
                    {provider}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={statusFilter} onValueChange={(value) => setStatusFilter(value as 'all' | HealthStatus)}>
              <SelectTrigger className="w-36">
                <SelectValue placeholder="All status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All status</SelectItem>
                <SelectItem value="healthy">Healthy</SelectItem>
                <SelectItem value="degraded">Degraded</SelectItem>
                <SelectItem value="failing">Failing</SelectItem>
              </SelectContent>
            </Select>
            <Button variant="outline" size="sm" onClick={exportCsv} disabled={sortedRows.length === 0}>
              <Download className="mr-2 h-4 w-4" />
              Export CSV
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted-foreground">
          Using real API key usage and request drill-down data for the selected window (aggregated across all keys).
        </p>
      </SectionCard>

      {hasDegradedProviderAlert && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>Degraded Provider Alert</AlertTitle>
          <AlertDescription>
            One or more models show success rate below 90% in this window.
          </AlertDescription>
        </Alert>
      )}

      {latencyRegressionModels.length > 0 && (
        <Alert>
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>Latency Regression Detection</AlertTitle>
          <AlertDescription>
            Higher than 30% latency increase vs previous selected window: {latencyRegressionModels.join(', ')}.
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {isPageLoading ? (
          <>
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
          </>
        ) : (
          <>
            <StatCard
              title="Fastest Model"
              value={fastestModel?.model || '—'}
              icon={Timer}
              description={fastestModel ? `${fastestModel.avg_latency_ms.toFixed(1)} ms` : 'No data'}
            />
            <StatCard
              title="Cheapest Model"
              value={cheapestModel?.model || '—'}
              icon={Wallet}
              description={cheapestModel ? formatCurrency(cheapestModel.avg_cost_usd) : 'No data'}
            />
            <StatCard
              title="Most Reliable Model"
              value={mostReliableModel?.model || '—'}
              icon={ShieldCheck}
              description={mostReliableModel ? formatPercent(mostReliableModel.success_rate) : 'No data'}
            />
            <StatCard
              title="Failing Models"
              value={String(failingModelsCount)}
              icon={AlertTriangle}
              description="success_rate < 90%"
            />
          </>
        )}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <SectionCard title="Latency by Model">
          {isPageLoading ? (
            <Skeleton className="h-64" />
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={sortedRows}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="model" tickFormatter={formatModelTick} interval={0} angle={-30} textAnchor="end" height={72} />
                <YAxis />
                <Tooltip
                  labelFormatter={(label) => `Model: ${String(label)}`}
                  formatter={(value) => `${Number(value).toFixed(1)} ms`}
                />
                <Bar dataKey="avg_latency_ms" fill="#0ea5e9" />
              </BarChart>
            </ResponsiveContainer>
          )}
        </SectionCard>

        <SectionCard title="Success Rate by Model">
          {isPageLoading ? (
            <Skeleton className="h-64" />
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={sortedRows}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="model" tickFormatter={formatModelTick} interval={0} angle={-30} textAnchor="end" height={72} />
                <YAxis domain={[0, 1]} tickFormatter={(value) => `${Math.round(Number(value) * 100)}%`} />
                <Tooltip
                  labelFormatter={(label) => `Model: ${String(label)}`}
                  formatter={(value) => formatPercent(Number(value))}
                />
                <Bar dataKey="success_rate" fill="#22c55e" />
              </BarChart>
            </ResponsiveContainer>
          )}
        </SectionCard>

        <SectionCard title="Cost by Model">
          {isPageLoading ? (
            <Skeleton className="h-64" />
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={sortedRows}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="model" tickFormatter={formatModelTick} interval={0} angle={-30} textAnchor="end" height={72} />
                <YAxis tickFormatter={(value) => formatCurrency(Number(value))} />
                <Tooltip
                  labelFormatter={(label) => `Model: ${String(label)}`}
                  formatter={(value) => formatCurrency(Number(value))}
                />
                <Bar dataKey="avg_cost_usd" fill="#f59e0b" />
              </BarChart>
            </ResponsiveContainer>
          )}
        </SectionCard>

        <SectionCard title="Cost vs Latency">
          {isPageLoading ? (
            <Skeleton className="h-64" />
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <ScatterChart>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis type="number" dataKey="avg_latency_ms" name="Latency" unit="ms" />
                <YAxis type="number" dataKey="avg_cost_usd" name="Cost" tickFormatter={(value) => formatCurrency(Number(value))} />
                <Tooltip
                  cursor={{ strokeDasharray: '3 3' }}
                  content={({ active, payload }) => {
                    if (!active || !payload || payload.length === 0) return null
                    const point = payload[0]?.payload as ModelHealthRow | undefined
                    if (!point) return null
                    return (
                      <div className="rounded-md border bg-background p-2 text-xs shadow">
                        <div className="font-semibold">{point.model}</div>
                        <div className="text-muted-foreground">Latency: {point.avg_latency_ms.toFixed(1)} ms</div>
                        <div className="text-muted-foreground">Cost: {formatCurrency(point.avg_cost_usd)}</div>
                      </div>
                    )
                  }}
                />
                <Scatter data={sortedRows} fill="#8b5cf6" />
              </ScatterChart>
            </ResponsiveContainer>
          )}
        </SectionCard>
      </div>

      <SectionCard
        title="Model Health Table"
        action={
          recommendedModel ? (
            <div className="flex items-center gap-2">
              <Badge variant="secondary" className="gap-1">
                <HeartPulse className="h-3 w-3" />
                Recommended Model: {recommendedModel.model}
              </Badge>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-primary"
                onClick={() => openExplainDrawer(recommendedModel)}
              >
                Why this model?
              </Button>
            </div>
          ) : noRecommendationAvailable ? (
            <Badge variant="outline">No recommended model available</Badge>
          ) : undefined
        }
      >
        {isPageLoading ? (
          <Skeleton className="h-96" />
        ) : (
          <div className="border rounded-md">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead><SortHeader field="model" label="Model" /></TableHead>
                  <TableHead><SortHeader field="provider" label="Provider" /></TableHead>
                  <TableHead><SortHeader field="model_type" label="Model Type" /></TableHead>
                  <TableHead><SortHeader field="avg_latency_ms" label="Avg Latency (ms)" /></TableHead>
                  <TableHead><SortHeader field="p95_latency_ms" label="P95 Latency (ms)" /></TableHead>
                  <TableHead><SortHeader field="success_rate" label="Success Rate" /></TableHead>
                  <TableHead><SortHeader field="avg_cost_usd" label="Avg Cost (USD)" /></TableHead>
                  <TableHead><SortHeader field="requests" label="Requests" /></TableHead>
                  <TableHead><SortHeader field="status" label="Status" /></TableHead>
                  <TableHead>Explain</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sortedRows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={10} className="text-center text-muted-foreground">
                      No model health data available
                    </TableCell>
                  </TableRow>
                ) : (
                  sortedRows.map((row) => (
                    <TableRow key={`${row.model}-${row.provider}`}>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <span className={row.model === recommendedModel?.model ? 'font-semibold' : undefined}>{row.model}</span>
                          {row.model === recommendedModel?.model && <Badge variant="outline">Recommended</Badge>}
                        </div>
                      </TableCell>
                      <TableCell>{row.provider}</TableCell>
                      <TableCell className="capitalize">{row.model_type}</TableCell>
                      <TableCell className="tabular-nums">{row.avg_latency_ms.toFixed(1)}</TableCell>
                      <TableCell className="tabular-nums">{row.p95_latency_ms.toFixed(1)}</TableCell>
                      <TableCell className="tabular-nums">{formatPercent(row.success_rate)}</TableCell>
                      <TableCell className="tabular-nums">{formatCurrency(row.avg_cost_usd)}</TableCell>
                      <TableCell className="tabular-nums">{row.samples.toLocaleString()}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className={getStatusBadgeClassName(row.status)}>
                          {row.status}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-7 px-2"
                          onClick={() => openExplainDrawer(row)}
                        >
                          Explain
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        )}
      </SectionCard>

      <Sheet open={explainOpen} onOpenChange={setExplainOpen}>
        <SheetContent className="w-[600px] sm:w-[720px] overflow-y-auto">
          <SheetHeader>
            <SheetTitle>Model Explanation</SheetTitle>
            <SheetDescription>{explainRow?.model ?? '—'}</SheetDescription>
          </SheetHeader>

          {explainRow ? (
            <div className="mt-6 space-y-6">
              <section className="space-y-3">
                <h3 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Status Explanation</h3>
                <div className="grid grid-cols-2 gap-3 text-sm">
                  <div><span className="text-muted-foreground">Model:</span> {explainRow.model}</div>
                  <div><span className="text-muted-foreground">Provider:</span> {explainRow.provider}</div>
                  <div><span className="text-muted-foreground">Success Rate:</span> {formatPercent(explainRow.success_rate)}</div>
                  <div><span className="text-muted-foreground">Avg Latency:</span> {explainRow.avg_latency_ms.toFixed(1)} ms</div>
                  <div><span className="text-muted-foreground">Avg Cost:</span> {formatCurrency(explainRow.avg_cost_usd)}</div>
                  <div><span className="text-muted-foreground">Model type:</span> {explainRow.model_type}</div>
                  <div><span className="text-muted-foreground">Requests:</span> {explainRow.samples.toLocaleString()}</div>
                  <div className="col-span-2 flex items-center gap-2">
                    <span className="text-muted-foreground">Status:</span>
                    <Badge variant="outline" className={getStatusBadgeClassName(explainRow.status)}>
                      {explainRow.status}
                    </Badge>
                  </div>
                </div>
                <p className="text-sm text-muted-foreground">{statusExplanation}</p>
              </section>

              <section className="space-y-3">
                <h3 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Recommendation Explanation</h3>
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className={getRecommendationBadgeClassName(getRecommendationState(explainRow))}>
                    {getRecommendationState(explainRow) === 'recommended'
                      ? 'Recommended'
                      : getRecommendationState(explainRow) === 'eligible'
                        ? 'Eligible'
                        : 'Excluded'}
                  </Badge>
                </div>
                <p className="text-sm text-muted-foreground">{recommendationExplanation}</p>
              </section>

              <section className="space-y-3">
                <h3 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Comparison Against Peers</h3>
                <div className="space-y-1 text-sm">
                  <p>Reliability rank: {getRankLabel(reliabilityRanking, explainRow)}</p>
                  <p>Latency rank: {getRankLabel(latencyRanking, explainRow)}</p>
                  <p>Cost rank: {getRankLabel(costRanking, explainRow)}</p>
                </div>
                <div className="space-y-1 text-sm text-muted-foreground">
                  <p>Fastest model: {fastestModel?.model ?? '—'}</p>
                  <p>Cheapest eligible model: {cheapestEligibleModel?.model ?? '—'}</p>
                  <p>Most reliable model: {mostReliableModel?.model ?? '—'}</p>
                </div>
              </section>

              <section className="space-y-3">
                <h3 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Recommendation Details</h3>
                <p className="text-sm text-muted-foreground">{recommendationDetails}</p>
              </section>
            </div>
          ) : null}
        </SheetContent>
      </Sheet>
    </div>
  )
}
