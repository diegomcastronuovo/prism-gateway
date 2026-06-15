'use client'

import { useMemo, useState } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard } from '@/components/shared/section-card'
import { ArrowLeft, Activity, Zap, TrendingDown, Download } from 'lucide-react'
import { useRoutingInsights } from '../api/use-routing-insights'
import type { RoutingDecision, RoutingInsightsFilters } from '../types/routing-insights'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from 'recharts'

interface RoutingInsightsViewProps {
  onBack: () => void
}

const STRATEGY_COLORS: Record<string, string> = {
  smart: '#3b82f6',
  round_robin: '#6b7280',
  priority: '#9333ea',
}

const TIME_RANGE_OPTIONS = [
  { label: 'Last 1h', hours: 1 },
  { label: 'Last 6h', hours: 6 },
  { label: 'Last 24h', hours: 24 },
  { label: 'Last 7 days', hours: 168 },
  { label: 'Last 30 days', hours: 720 },
] as const

function formatTimestamp(timestamp: string): string {
  const date = new Date(timestamp)
  return date.toLocaleString()
}

function getStatusVariant(status: string): 'default' | 'destructive' | 'secondary' {
  if (status === 'success' || status === 'ok') return 'default'
  if (status === 'error') return 'destructive'
  return 'secondary'
}

function getStatusBadgeClassName(status: string): string {
  if (status === 'success' || status === 'ok') {
    return 'bg-green-100 text-green-800 border-green-200 hover:bg-green-100'
  }
  if (status === 'error') {
    return 'bg-red-100 text-red-800 border-red-200 hover:bg-red-100'
  }
  return ''
}

function getStrategyBadgeClassName(strategy: string): string {
  if (strategy === 'smart') return 'bg-blue-100 text-blue-800 border-blue-200 hover:bg-blue-100'
  if (strategy === 'round_robin') return 'bg-gray-100 text-gray-800 border-gray-200 hover:bg-gray-100'
  if (strategy === 'priority') return 'bg-purple-100 text-purple-800 border-purple-200 hover:bg-purple-100'
  return ''
}

type RoutingSortField = 'timestamp' | 'tenant_id' | 'selected_model' | 'provider' | 'latency_ms'
type RoutingSortDirection = 'asc' | 'desc'

function SortHeader({
  label,
  field,
  sortField,
  sortDirection,
  onSortChange,
}: {
  label: string
  field: RoutingSortField
  sortField: RoutingSortField
  sortDirection: RoutingSortDirection
  onSortChange: (field: RoutingSortField) => void
}) {
  const active = sortField === field
  const indicator = active ? (sortDirection === 'asc' ? '↑' : '↓') : ''
  return (
    <button
      type="button"
      className="inline-flex items-center gap-1 font-medium hover:text-foreground"
      onClick={() => onSortChange(field)}
    >
      {label}
      <span className="text-xs text-muted-foreground">{indicator}</span>
    </button>
  )
}

export function RoutingInsightsView({ onBack }: RoutingInsightsViewProps) {
  const [filters, setFilters] = useState<RoutingInsightsFilters>({
    window_hours: 24,
    limit: 50,
    offset: 0,
  })
  const [selectedDecision, setSelectedDecision] = useState<RoutingDecision | null>(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [currentPage, setCurrentPage] = useState(0)
  const [sortField, setSortField] = useState<RoutingSortField>('timestamp')
  const [sortDirection, setSortDirection] = useState<RoutingSortDirection>('desc')
  const [showRawJson, setShowRawJson] = useState(false)

  const { data, isLoading } = useRoutingInsights(filters)

  const updateFilter = (key: keyof RoutingInsightsFilters, value: string | boolean | number) => {
    setCurrentPage(0)
    setFilters((prev) => ({
      ...prev,
      offset: 0,
      [key]: value === '' ? undefined : value,
    }))
  }

  const goToPage = (page: number) => {
    const safePage = Math.max(0, page)
    const limit = filters.limit || 50
    setCurrentPage(safePage)
    setFilters((prev) => ({
      ...prev,
      limit,
      offset: safePage * limit,
    }))
  }

  const handleRowClick = (decision: RoutingDecision) => {
    setSelectedDecision(decision)
    setShowRawJson(false)
    setDrawerOpen(true)
  }

  const decisions = useMemo(() => data?.decisions ?? [], [data?.decisions])
  const trafficOverTime = useMemo(() => data?.traffic_over_time ?? [], [data?.traffic_over_time])
  const summary = useMemo(() => data?.summary ?? {}, [data?.summary])
  const tenantOptions = useMemo(
    () => Array.from(new Set(decisions.map((d) => d.tenant_id).filter(Boolean))).sort(),
    [decisions]
  )
  const modelOptions = useMemo(
    () => Array.from(new Set(decisions.map((d) => d.selected_model).filter(Boolean))).sort(),
    [decisions]
  )
  const providerOptions = useMemo(
    () => Array.from(new Set(decisions.map((d) => d.provider).filter(Boolean))).sort(),
    [decisions]
  )
  const strategyOptions = useMemo(
    () => Array.from(new Set(decisions.map((d) => d.strategy).filter(Boolean))).sort(),
    [decisions]
  )

  const filteredDecisions = useMemo(() => {
    if (!filters.strategy) return decisions
    return decisions.filter((d) => d.strategy === filters.strategy)
  }, [decisions, filters.strategy])

  const sortedDecisions = useMemo(() => {
    const sorted = [...filteredDecisions]
    sorted.sort((a, b) => {
      let compare = 0
      if (sortField === 'timestamp') {
        compare = new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
      } else if (sortField === 'latency_ms') {
        compare = (a.latency_ms || 0) - (b.latency_ms || 0)
      } else {
        const left = String(a[sortField] ?? '').toLowerCase()
        const right = String(b[sortField] ?? '').toLowerCase()
        compare = left.localeCompare(right)
      }
      return sortDirection === 'asc' ? compare : -compare
    })
    return sorted
  }, [filteredDecisions, sortDirection, sortField])

  const handleSortChange = (field: RoutingSortField) => {
    if (field === sortField) {
      setSortDirection((prev) => (prev === 'asc' ? 'desc' : 'asc'))
      return
    }
    setSortField(field)
    setSortDirection('asc')
  }

  const exportRoutingDecisionsToCsv = () => {
    const headers = [
      'timestamp',
      'tenant_id',
      'request_id',
      'model',
      'provider',
      'strategy',
      'fallback_used',
      'status',
      'latency_ms',
    ]
    const rows = sortedDecisions.map((decision) => [
      decision.timestamp,
      decision.tenant_id,
      decision.request_id,
      decision.selected_model,
      decision.provider,
      decision.strategy,
      String(decision.fallback_used),
      decision.status,
      String(decision.latency_ms),
    ])
    const content = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n')
    const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const now = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    const fileName = `routing_decisions_${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}.csv`

    const link = document.createElement('a')
    link.href = url
    link.download = fileName
    link.click()
    URL.revokeObjectURL(url)
  }

  const hasRouteGroup = sortedDecisions.some((d) => d.route_group)
  const canReplay = false
  const selectedWindowHours = filters.window_hours ?? 24
  const selectedWindowLabel = TIME_RANGE_OPTIONS.find((option) => option.hours === selectedWindowHours)?.label ?? `Last ${selectedWindowHours}h`

  const latencyDistribution = useMemo(() => {
    return trafficOverTime.map((item) => ({
      bucket: new Date(item.bucket).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit' }),
      requests: item.requests || 0,
      successes: item.successes || 0,
      errors: item.errors || 0,
    }))
  }, [trafficOverTime])

  const strategyUsage = useMemo(() => {
    if (data?.strategy_distribution && Array.isArray(data.strategy_distribution)) {
      return data.strategy_distribution.map((item) => ({
        strategy: item.strategy,
        count: item.count,
        fill: STRATEGY_COLORS[item.strategy] || '#94a3b8',
      }))
    }
    return []
  }, [data?.strategy_distribution])

  const decisionReasonDistribution = useMemo(() => {
    const counts = new Map<string, number>()
    for (const decision of decisions) {
      const reason = decision.decision_reason?.trim() || 'unknown'
      counts.set(reason, (counts.get(reason) || 0) + 1)
    }
    const total = decisions.length
    return Array.from(counts.entries())
      .sort((a, b) => b[1] - a[1])
      .map(([reason, count]) => ({
        reason,
        count,
        percentage: total > 0 ? (count / total) * 100 : 0,
      }))
  }, [decisions])

  const failureRows = useMemo(
    () => decisions.filter((decision) => !['ok', 'success'].includes(decision.status.toLowerCase())),
    [decisions]
  )

  const errorTypeDistribution = useMemo(() => {
    const counts = new Map<string, number>()
    for (const decision of failureRows) {
      const errorType = decision.error_type?.trim()
      if (!errorType) continue
      counts.set(errorType, (counts.get(errorType) || 0) + 1)
    }
    const total = Array.from(counts.values()).reduce((sum, value) => sum + value, 0)
    return Array.from(counts.entries())
      .sort((a, b) => b[1] - a[1])
      .map(([errorType, count]) => ({
        errorType,
        count,
        percentage: total > 0 ? (count / total) * 100 : 0,
      }))
  }, [failureRows])

  const fallbackUsageData = useMemo(() => {
    const totalRequests = Number(summary.total_requests ?? 0)
    const fallbackRequests = Number(summary.fallback_requests ?? 0)
    const noFallback = Math.max(0, totalRequests - fallbackRequests)
    return [
      {
        label: 'No fallback',
        count: noFallback,
        percentage: totalRequests > 0 ? (noFallback / totalRequests) * 100 : 0,
      },
      {
        label: 'Fallback used',
        count: fallbackRequests,
        percentage: totalRequests > 0 ? (fallbackRequests / totalRequests) * 100 : 0,
      },
    ]
  }, [summary])

  const fallbackAttemptsDistribution = useMemo(() => {
    const counts = new Map<string, number>()
    for (const decision of decisions) {
      if (decision.fallback_used === false) {
        counts.set('0', (counts.get('0') || 0) + 1)
      } else if (decision.fallback_used === true) {
        if (typeof decision.fallback_attempts === 'number' && Number.isFinite(decision.fallback_attempts)) {
          const key = `${Math.max(0, Math.round(decision.fallback_attempts))}`
          counts.set(key, (counts.get(key) || 0) + 1)
        } else {
          counts.set('unknown', (counts.get('unknown') || 0) + 1)
        }
      }
    }

    return Array.from(counts.entries())
      .sort((a, b) => {
        if (a[0] === 'unknown') return 1
        if (b[0] === 'unknown') return -1
        return Number(a[0]) - Number(b[0])
      })
      .map(([attempts, count]) => ({
        attempts: attempts === 'unknown' ? 'unknown attempts' : `${attempts} attempt${attempts === '1' ? '' : 's'}`,
        count,
      }))
  }, [decisions])

  const topFailureCauses = useMemo(() => {
    const counts = new Map<string, number>()
    for (const decision of failureRows) {
      const errorType = decision.error_type?.trim() || 'unknown'
      counts.set(errorType, (counts.get(errorType) || 0) + 1)
    }
    const totalFailures = failureRows.length
    return Array.from(counts.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .map(([errorType, count]) => ({
        errorType,
        count,
        percentage: totalFailures > 0 ? (count / totalFailures) * 100 : 0,
      }))
  }, [failureRows])

  const pagination = data?.pagination
  const total = pagination?.total ?? sortedDecisions.length
  const limit = pagination?.limit ?? filters.limit ?? 50
  const totalPages = Math.max(1, Math.ceil(total / limit))

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-4">
        <Button variant="outline" onClick={onBack}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Observability
        </Button>
        <h2 className="text-2xl font-bold">Routing Insights</h2>
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor="time-range-select">Time Range</Label>
          <Select
            value={String(selectedWindowHours)}
            onValueChange={(value) => updateFilter('window_hours', Number(value))}
          >
            <SelectTrigger id="time-range-select" className="w-[180px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TIME_RANGE_OPTIONS.map((option) => (
                <SelectItem key={option.hours} value={String(option.hours)}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {isLoading ? (
          <>
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
          </>
        ) : (data?.metrics?.total_routed_requests ?? 0) === 0 ? (
          <SectionCard title="Routing Summary">
            <p className="text-sm text-muted-foreground">No routing data available for the selected window.</p>
          </SectionCard>
        ) : (
          <>
            <StatCard
              title="Total Routed Requests"
              value={(data?.metrics.total_routed_requests ?? sortedDecisions.length).toLocaleString()}
              icon={Activity}
              description="Routed"
            />
            <StatCard
              title="Smart Routing"
              value={
                (() => {
                  const totalWithStrategy = decisions.filter((d) => d.strategy).length
                  const smartCount = decisions.filter((d) => d.strategy === 'smart').length
                  if (totalWithStrategy === 0) return '—'
                  const percentage = (smartCount / totalWithStrategy) * 100
                  return `${percentage.toFixed(1)}%`
                })()
              }
              icon={Zap}
              description="Usage"
            />
            <StatCard
              title="Fallback Rate"
              value={
                data?.metrics.fallback_usage_pct != null
                  ? `${data.metrics.fallback_usage_pct.toFixed(1)}%`
                  : '—'
              }
              icon={TrendingDown}
              description="Usage"
            />
          </>
        )}
      </div>

      {/* Traffic Over Time */}
      <SectionCard title={`Traffic Over Time (${selectedWindowLabel})`}>
        {isLoading ? (
          <Skeleton className="h-64" />
        ) : trafficOverTime.length === 0 ? (
          <p className="text-sm text-muted-foreground">No routing data available for the selected window.</p>
        ) : (
          <ResponsiveContainer width="100%" height={260}>
            <BarChart data={latencyDistribution}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="bucket" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="requests" fill="#0ea5e9" name="Requests" />
              <Bar dataKey="errors" fill="#ef4444" name="Errors" />
            </BarChart>
          </ResponsiveContainer>
        )}
      </SectionCard>

      {/* Routing Strategy Breakdown */}
      <SectionCard title="Routing Strategy Usage">
        {isLoading ? (
          <Skeleton className="h-64" />
        ) : strategyUsage.length === 0 ? (
          <p className="text-sm text-muted-foreground">No routing data available for the selected window.</p>
        ) : (
          <ResponsiveContainer width="100%" height={260}>
            <PieChart>
              <Pie
                data={strategyUsage}
                dataKey="count"
                nameKey="strategy"
                innerRadius={50}
                outerRadius={90}
                label={(props) => {
                  const payload = props.payload as { strategy: string; count: number }
                  const percent = props.percent ?? 0
                  return `${payload.strategy}: ${payload.count} (${(percent * 100).toFixed(1)}%)`
                }}
              >
                {strategyUsage.map((entry) => (
                  <Cell key={entry.strategy} fill={entry.fill} />
                ))}
              </Pie>
              <Tooltip formatter={(value: unknown) => [`${Number(value)} requests`, 'Count']} />
            </PieChart>
          </ResponsiveContainer>
        )}
      </SectionCard>

      <SectionCard
        title="Routing Diagnostics"
        description="Aggregated routing outcomes and failure patterns for the selected time window."
      >
        <div className="grid gap-6 lg:grid-cols-2">
          <SectionCard title="Decision Reason Distribution">
            {isLoading ? (
              <Skeleton className="h-56" />
            ) : decisionReasonDistribution.length === 0 ? (
              <p className="text-sm text-muted-foreground">No routing data available for the selected window.</p>
            ) : (
              <ResponsiveContainer width="100%" height={220}>
                <BarChart data={decisionReasonDistribution}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="reason" />
                  <YAxis />
                  <Tooltip
                    formatter={(value: any, _name, item) => [
                      `${Number(value || 0).toLocaleString()} (${item?.payload?.percentage?.toFixed(1)}%)`,
                      'Count',
                    ]}
                  />
                  <Bar dataKey="count" fill="#0ea5e9" />
                </BarChart>
              </ResponsiveContainer>
            )}
          </SectionCard>

          <SectionCard title="Error Type Distribution">
            {isLoading ? (
              <Skeleton className="h-56" />
            ) : errorTypeDistribution.length === 0 ? (
              <p className="text-sm text-muted-foreground">No errors recorded in the selected window.</p>
            ) : (
              <ResponsiveContainer width="100%" height={220}>
                <BarChart data={errorTypeDistribution}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="errorType" />
                  <YAxis />
                  <Tooltip
                    formatter={(value: any, _name, item) => [
                      `${Number(value || 0).toLocaleString()} (${item?.payload?.percentage?.toFixed(1)}%)`,
                      'Count',
                    ]}
                  />
                  <Bar dataKey="count" fill="#ef4444" />
                </BarChart>
              </ResponsiveContainer>
            )}
          </SectionCard>

          <SectionCard title="Fallback Analysis">
            {isLoading ? (
              <Skeleton className="h-56" />
            ) : (summary.total_requests ?? 0) === 0 ? (
              <p className="text-sm text-muted-foreground">No fallback activity in the selected window.</p>
            ) : (
              <div className="grid gap-6 md:grid-cols-2">
                <div>
                  <h4 className="mb-2 text-sm font-medium">Fallback Usage</h4>
                  <ResponsiveContainer width="100%" height={180}>
                    <BarChart data={fallbackUsageData}>
                      <CartesianGrid strokeDasharray="3 3" />
                      <XAxis dataKey="label" />
                      <YAxis />
                      <Tooltip
                        formatter={(value: any, _name, item) => [
                          `${Number(value || 0).toLocaleString()} (${item?.payload?.percentage?.toFixed(1)}%)`,
                          'Count',
                        ]}
                      />
                      <Bar dataKey="count" fill="#6366f1" />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
                <div>
                  <h4 className="mb-2 text-sm font-medium">Fallback Attempts Distribution</h4>
                  <ResponsiveContainer width="100%" height={180}>
                    <BarChart data={fallbackAttemptsDistribution}>
                      <CartesianGrid strokeDasharray="3 3" />
                      <XAxis dataKey="attempts" />
                      <YAxis />
                      <Tooltip formatter={(value) => [`${Number(value)} requests`, 'Count']} />
                      <Bar dataKey="count" fill="#06b6d4" />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </div>
            )}
          </SectionCard>

          <SectionCard title="Top Failure Causes">
            {isLoading ? (
              <Skeleton className="h-56" />
            ) : topFailureCauses.length === 0 ? (
              <p className="text-sm text-muted-foreground">No failures recorded in the selected window.</p>
            ) : (
              <div className="border rounded-md">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Error Type</TableHead>
                      <TableHead className="text-right">Count</TableHead>
                      <TableHead className="text-right">%</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {topFailureCauses.map((row) => (
                      <TableRow key={row.errorType}>
                        <TableCell>{row.errorType}</TableCell>
                        <TableCell className="text-right tabular-nums">{row.count}</TableCell>
                        <TableCell className="text-right tabular-nums">{row.percentage.toFixed(1)}%</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </SectionCard>
        </div>
      </SectionCard>

      {/* Filters */}
      <SectionCard title="Filters">
        <div className="grid grid-cols-6 gap-4">
          <div className="space-y-2">
            <Label htmlFor="tenant-filter">Tenant</Label>
            <Select value={filters.tenant_id || 'all'} onValueChange={(v) => updateFilter('tenant_id', v === 'all' ? '' : v)}>
              <SelectTrigger id="tenant-filter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                {tenantOptions.map((tenant) => (
                  <SelectItem key={tenant} value={tenant}>
                    {tenant}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="model-filter">Model</Label>
            <Select value={filters.model || 'all'} onValueChange={(v) => updateFilter('model', v === 'all' ? '' : v)}>
              <SelectTrigger id="model-filter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                {modelOptions.map((model) => (
                  <SelectItem key={model} value={model}>
                    {model}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="provider-filter">Provider</Label>
            <Select value={filters.provider || 'all'} onValueChange={(v) => updateFilter('provider', v === 'all' ? '' : v)}>
              <SelectTrigger id="provider-filter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                {providerOptions.map((provider) => (
                  <SelectItem key={provider} value={provider}>
                    {provider}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="strategy-filter">Strategy</Label>
            <Select value={filters.strategy || 'all'} onValueChange={(v) => updateFilter('strategy', v === 'all' ? '' : v)}>
              <SelectTrigger id="strategy-filter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                {strategyOptions.map((strategy) => (
                  <SelectItem key={strategy} value={strategy}>
                    {strategy}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="fallback-filter">Fallback</Label>
            <Select
              value={filters.fallback_used === undefined ? 'all' : String(filters.fallback_used)}
              onValueChange={(v) => updateFilter('fallback_used', v === 'all' ? '' : v === 'true')}
            >
              <SelectTrigger id="fallback-filter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                <SelectItem value="true">Yes</SelectItem>
                <SelectItem value="false">No</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="status-filter">Status</Label>
            <Select value={filters.status || 'all'} onValueChange={(v) => updateFilter('status', v === 'all' ? '' : v)}>
              <SelectTrigger id="status-filter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                <SelectItem value="ok">OK</SelectItem>
                <SelectItem value="error">Error</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
      </SectionCard>

      {/* Routing Decisions Table */}
      <SectionCard
        title="Routing Decisions"
        action={
          <Button
            variant="outline"
            size="sm"
            onClick={exportRoutingDecisionsToCsv}
            disabled={sortedDecisions.length === 0}
          >
            <Download className="mr-2 h-4 w-4" />
            Export CSV
          </Button>
        }
      >
        {isLoading ? (
          <Skeleton className="h-96" />
        ) : (
          <div className="border rounded-md">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    <SortHeader
                      label="Timestamp"
                      field="timestamp"
                      sortField={sortField}
                      sortDirection={sortDirection}
                      onSortChange={handleSortChange}
                    />
                  </TableHead>
                  <TableHead>
                    <SortHeader
                      label="Tenant"
                      field="tenant_id"
                      sortField={sortField}
                      sortDirection={sortDirection}
                      onSortChange={handleSortChange}
                    />
                  </TableHead>
                  <TableHead>Request ID</TableHead>
                  <TableHead>
                    <SortHeader
                      label="Model"
                      field="selected_model"
                      sortField={sortField}
                      sortDirection={sortDirection}
                      onSortChange={handleSortChange}
                    />
                  </TableHead>
                  <TableHead>
                    <SortHeader
                      label="Provider"
                      field="provider"
                      sortField={sortField}
                      sortDirection={sortDirection}
                      onSortChange={handleSortChange}
                    />
                  </TableHead>
                  <TableHead>Strategy</TableHead>
                  {hasRouteGroup && <TableHead>Route Group</TableHead>}
                  <TableHead>Fallback</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>
                    <SortHeader
                      label="Latency (ms)"
                      field="latency_ms"
                      sortField={sortField}
                      sortDirection={sortDirection}
                      onSortChange={handleSortChange}
                    />
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sortedDecisions.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={hasRouteGroup ? 10 : 9} className="text-center text-muted-foreground">
                      No routing decisions available
                    </TableCell>
                  </TableRow>
                ) : (
                  sortedDecisions.map((decision) => (
                    <TableRow
                      key={decision.id}
                      className="cursor-pointer hover:bg-muted/50"
                      onClick={() => handleRowClick(decision)}
                    >
                      <TableCell className="font-mono text-xs">
                        {formatTimestamp(decision.timestamp)}
                      </TableCell>
                      <TableCell className="font-medium">{decision.tenant_id}</TableCell>
                      <TableCell className="font-mono text-xs">{decision.request_id}</TableCell>
                      <TableCell>{decision.selected_model}</TableCell>
                      <TableCell>{decision.provider}</TableCell>
                      <TableCell>
                        <Badge className={getStrategyBadgeClassName(decision.strategy)} variant="outline">
                          {decision.strategy}
                        </Badge>
                      </TableCell>
                      {hasRouteGroup && (
                        <TableCell>
                          {decision.route_group ? (
                            <Badge variant="outline">{decision.route_group}</Badge>
                          ) : (
                            <span className="text-muted-foreground text-sm">-</span>
                          )}
                        </TableCell>
                      )}
                      <TableCell>
                        {decision.fallback_used ? (
                          <Badge variant="secondary">Yes</Badge>
                        ) : (
                          <span className="text-muted-foreground text-sm">-</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <Badge className={getStatusBadgeClassName(decision.status)} variant={getStatusVariant(decision.status)}>
                          {decision.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="tabular-nums">{decision.latency_ms}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        )}
        {!isLoading && (
          <div className="mt-4 flex items-center justify-end gap-2">
            <Button variant="outline" size="sm" onClick={() => goToPage(currentPage - 1)} disabled={currentPage === 0}>
              Previous
            </Button>
            <span className="text-sm text-muted-foreground">
              Page {currentPage + 1} of {totalPages}
            </span>
            {Array.from({ length: totalPages }, (_, i) => i)
              .filter((p) => p >= Math.max(0, currentPage - 2) && p <= Math.min(totalPages - 1, currentPage + 2))
              .map((p) => (
                <Button
                  key={p}
                  variant={p === currentPage ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => goToPage(p)}
                >
                  {p + 1}
                </Button>
              ))}
            <Button
              variant="outline"
              size="sm"
              onClick={() => goToPage(currentPage + 1)}
              disabled={currentPage >= totalPages - 1}
            >
              Next
            </Button>
          </div>
        )}
      </SectionCard>

      {/* Decision Details Drawer */}
      <Sheet open={drawerOpen} onOpenChange={setDrawerOpen}>
        <SheetContent className="w-[600px] sm:w-[700px] overflow-y-auto">
          <SheetHeader>
            <SheetTitle>Decision Details</SheetTitle>
            <SheetDescription>
              Request ID: {selectedDecision?.request_id}
            </SheetDescription>
          </SheetHeader>

          {selectedDecision && (
            <div className="space-y-6 mt-6">
              <div className="flex items-center gap-2">
                {canReplay && (
                  <Button variant="outline" size="sm">
                    Replay Request
                  </Button>
                )}
                <Button variant="outline" size="sm" onClick={() => setShowRawJson((v) => !v)}>
                  View Raw JSON
                </Button>
              </div>

              <SectionCard title="Request Info">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <h4 className="text-sm font-medium mb-1">Request ID</h4>
                  <p className="text-sm font-mono">{selectedDecision.request_id}</p>
                </div>
                <div>
                  <h4 className="text-sm font-medium mb-1">Tenant</h4>
                  <p className="text-sm">{selectedDecision.tenant_id}</p>
                </div>
                <div>
                  <h4 className="text-sm font-medium mb-1">Selected Model</h4>
                  <p className="text-sm">{selectedDecision.selected_model}</p>
                </div>
                <div>
                  <h4 className="text-sm font-medium mb-1">Provider</h4>
                  <p className="text-sm">{selectedDecision.provider}</p>
                </div>
                <div>
                  <h4 className="text-sm font-medium mb-1">Strategy</h4>
                  <Badge variant="secondary">{selectedDecision.strategy}</Badge>
                </div>
                <div>
                  <h4 className="text-sm font-medium mb-1">Status</h4>
                  <Badge variant={getStatusVariant(selectedDecision.status)}>{selectedDecision.status}</Badge>
                </div>
                <div>
                  <h4 className="text-sm font-medium mb-1">Fallback</h4>
                  <p className="text-sm">{selectedDecision.fallback_used ? 'Yes' : '-'}</p>
                </div>
                <div>
                  <h4 className="text-sm font-medium mb-1">Latency (ms)</h4>
                  <p className="text-sm tabular-nums">{selectedDecision.latency_ms}</p>
                </div>
                {selectedDecision.attempt !== undefined && (
                  <div>
                    <h4 className="text-sm font-medium mb-1">Attempt</h4>
                    <p className="text-sm">{selectedDecision.attempt}</p>
                  </div>
                )}
                {selectedDecision.route_group && (
                  <div>
                    <h4 className="text-sm font-medium mb-1">Route Group</h4>
                    <Badge variant="outline">{selectedDecision.route_group}</Badge>
                  </div>
                )}
              </div>
              </SectionCard>

              <SectionCard title="Routing Info">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <h4 className="text-sm font-medium mb-1">Strategy</h4>
                    <Badge className={getStrategyBadgeClassName(selectedDecision.strategy)} variant="outline">
                      {selectedDecision.strategy}
                    </Badge>
                  </div>
                  <div>
                    <h4 className="text-sm font-medium mb-1">Fallback Used</h4>
                    <p className="text-sm">{selectedDecision.fallback_used ? 'Yes' : '-'}</p>
                  </div>
                  <div>
                    <h4 className="text-sm font-medium mb-1">Selected Model</h4>
                    <p className="text-sm">{selectedDecision.selected_model}</p>
                  </div>
                </div>
              </SectionCard>

              <SectionCard title="Execution">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <h4 className="text-sm font-medium mb-1">Status</h4>
                    <Badge className={getStatusBadgeClassName(selectedDecision.status)} variant={getStatusVariant(selectedDecision.status)}>
                      {selectedDecision.status}
                    </Badge>
                  </div>
                  <div>
                    <h4 className="text-sm font-medium mb-1">Latency (ms)</h4>
                    <p className="text-sm tabular-nums">{selectedDecision.latency_ms}</p>
                  </div>
                  <div>
                    <h4 className="text-sm font-medium mb-1">Cache Status</h4>
                    <p className="text-sm">{selectedDecision.cache_status || '-'}</p>
                  </div>
                </div>
              </SectionCard>

              {showRawJson && (
                <SectionCard title="Raw Request JSON">
                  <pre className="bg-muted p-3 rounded-md text-xs overflow-auto max-h-64">
                    {JSON.stringify(selectedDecision.raw_request ?? selectedDecision, null, 2)}
                  </pre>
                </SectionCard>
              )}
            </div>
          )}
        </SheetContent>
      </Sheet>
    </div>
  )
}
