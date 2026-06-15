'use client'

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { StatCard } from '@/components/shared/stat-card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  ArrowLeft,
  Activity,
  Timer,
  ArrowUp,
  Percent,
  Cpu,
  Download,
  Gauge,
} from 'lucide-react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  BarChart,
  Bar,
} from 'recharts'
import { parseObservabilityErrorResponse } from '@/features/observability/api/parse-observability-response'

type RouterPerformanceSummary = {
  avg_router_pre_ms?: number
  p50_router_pre_ms?: number
  p95_router_pre_ms?: number
  avg_router_post_ms?: number
  p50_router_post_ms?: number
  p95_router_post_ms?: number
  avg_total_latency_ms?: number
  p50_total_latency_ms?: number
  p95_total_latency_ms?: number
  avg_llm_latency_ms?: number
  p50_llm_latency_ms?: number
  p95_llm_latency_ms?: number
}

type RouterPerformanceTimeseries = {
  bucket_start: string
  requests?: number
  avg_router_pre_ms?: number
  avg_llm_latency_ms?: number
  avg_router_post_ms?: number
  avg_total_latency_ms?: number
  p95_router_pre_ms?: number
  p95_llm_latency_ms?: number
  p95_router_post_ms?: number
}

type PreBreakdownAvgMs = {
  tenant_config?: number
  tool_routes?: number
  dynamic_routes?: number
  decision_ops?: number
  budget_pressure?: number
  semantic?: number
  model_resolution?: number
}

type RouterPerformanceBreakdowns = {
  pre_breakdown_avg_ms?: PreBreakdownAvgMs
  tool_routes_breakdown_avg_ms?: Record<string, number>
}

type RouterPerformanceResponse = {
  summary?: RouterPerformanceSummary
  timeseries?: RouterPerformanceTimeseries[]
  breakdowns?: RouterPerformanceBreakdowns
}

type FiltersState = {
  from: string
  to: string
}

function toIsoString(value: string): string | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return date.toISOString()
}

function formatLatency(value?: number | null): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return `${Number(value).toFixed(1)} ms`
}

function formatBucketLabel(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

export function RouterPerformanceView({ onBack }: { onBack: () => void }) {
  const now = new Date()
  const defaultTo = new Date(now.getTime() - now.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
  const defaultFrom = new Date(now.getTime() - (24 * 60 * 60 * 1000) - now.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 16)
  const [filters, setFilters] = useState<FiltersState>({
    from: defaultFrom,
    to: defaultTo,
  })

  const { data, isLoading, error } = useQuery<RouterPerformanceResponse>({
    queryKey: ['router-performance', filters],
    queryFn: async () => {
      const params = new URLSearchParams()
      const fromIso = toIsoString(filters.from)
      const toIso = toIsoString(filters.to)

      if (fromIso) params.set('from', fromIso)
      if (toIso) params.set('to', toIso)
      params.set('bucket', 'hour')

      const query = params.toString()
      const res = await fetch(
        `/api/observability/router-performance${query ? `?${query}` : ''}`,
        { credentials: 'include', cache: 'no-store' }
      )

      if (!res.ok) {
        return parseObservabilityErrorResponse(res, 'Failed to fetch router performance')
      }

      return res.json()
    },
    refetchInterval: 30000,
    retry: (failureCount, err) => {
      const status = (err as Error & { status?: number }).status
      if (status === 401 || status === 403) return false
      return failureCount < 2
    },
    staleTime: 10000,
  })

  const summary = data?.summary || {}
  const timeseries = data?.timeseries || []
  const breakdowns = data?.breakdowns || {}

  const preBreakdown = breakdowns.pre_breakdown_avg_ms || {}
  const toolBreakdown = breakdowns.tool_routes_breakdown_avg_ms || {}

  const lineData = useMemo(
    () =>
      timeseries.map((entry) => ({
        ...entry,
        bucket_label: formatBucketLabel(entry.bucket_start),
        avg_router_only_ms:
          entry.avg_router_pre_ms !== undefined || entry.avg_router_post_ms !== undefined
            ? (entry.avg_router_pre_ms ?? 0) + (entry.avg_router_post_ms ?? 0)
            : undefined,
      })),
    [timeseries]
  )

  const chartKey = `${filters.from}-${filters.to}`

  const routerBreakdownData = useMemo(
    () =>
      [
        { label: 'tenant_config', value: preBreakdown.tenant_config },
        { label: 'tool_routes', value: preBreakdown.tool_routes },
        { label: 'dynamic_routes', value: preBreakdown.dynamic_routes },
        { label: 'decision_ops', value: preBreakdown.decision_ops },
        { label: 'budget_pressure', value: preBreakdown.budget_pressure },
        { label: 'semantic', value: preBreakdown.semantic },
        { label: 'model_resolution', value: preBreakdown.model_resolution },
      ].filter((item) => item.value !== undefined && item.value > 0),
    [preBreakdown]
  )

  const toolRoutesData = useMemo(
    () =>
      [
        { label: 'embedding_model', value: toolBreakdown.embedding_model },
        { label: 'embedding_generate', value: toolBreakdown.embedding_generate },
        { label: 'semantic_db', value: toolBreakdown.semantic_db },
        { label: 'match_eval', value: toolBreakdown.match_eval },
      ].filter((item) => item.value !== undefined),
    [toolBreakdown]
  )

  const exportCsv = () => {
    const headers = [
      'bucket_start',
      'requests',
      'avg_router_pre_ms',
      'avg_llm_latency_ms',
      'avg_router_post_ms',
      'avg_total_latency_ms',
      'p95_router_pre_ms',
      'p95_llm_latency_ms',
      'p95_router_post_ms',
    ]
    const rows = timeseries.map((row) => [
      row.bucket_start,
      String(row.requests ?? ''),
      String(row.avg_router_pre_ms ?? ''),
      String(row.avg_llm_latency_ms ?? ''),
      String(row.avg_router_post_ms ?? ''),
      String(row.avg_total_latency_ms ?? ''),
      String(row.p95_router_pre_ms ?? ''),
      String(row.p95_llm_latency_ms ?? ''),
      String(row.p95_router_post_ms ?? ''),
    ])
    const content = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n')
    const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const now = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    const fileName = `router_performance_${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}.csv`

    const link = document.createElement('a')
    link.href = url
    link.download = fileName
    link.click()
    URL.revokeObjectURL(url)
  }

  const statusError = error as Error & { status?: number }
  const isAccessDenied = statusError?.status === 401 || statusError?.status === 403

  return (
    <div className="space-y-6">
      <PageHeader
        title="Router Performance"
        description="Latency breakdown across router and LLM phases"
        action={
          <Button variant="outline" onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Observability
          </Button>
        }
      />

      <SectionCard title="Filters">
        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-4">
          <input
            type="datetime-local"
            value={filters.from}
            onChange={(event) => setFilters((prev) => ({ ...prev, from: event.target.value }))}
            className="rounded-md border border-input bg-background px-3 py-2 text-sm"
          />
          <input
            type="datetime-local"
            value={filters.to}
            onChange={(event) => setFilters((prev) => ({ ...prev, to: event.target.value }))}
            className="rounded-md border border-input bg-background px-3 py-2 text-sm"
          />
        </div>
      </SectionCard>

      {error ? (
        <SectionCard title={isAccessDenied ? 'Admin only' : 'Error'}>
          <p className="text-sm text-muted-foreground">
            {isAccessDenied
              ? 'This dashboard is only available to admins.'
              : statusError?.message || 'Unable to load router performance data.'}
          </p>
        </SectionCard>
      ) : (
        <div className="space-y-4">
          <SectionCard title="Router Latency (Pre + Post, model excluded)">
            <div className="grid gap-4 md:grid-cols-3">
              {isLoading ? (
                <>
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                </>
              ) : (
                <>
                  <StatCard title="Pre Avg" value={formatLatency(summary.avg_router_pre_ms)} icon={Activity} description="Pre-processing average" />
                  <StatCard title="Pre P50" value={formatLatency(summary.p50_router_pre_ms)} icon={Percent} description="Median" />
                  <StatCard title="Pre P95" value={formatLatency(summary.p95_router_pre_ms)} icon={ArrowUp} description="95th percentile" />
                  <StatCard title="Post Avg" value={formatLatency(summary.avg_router_post_ms)} icon={Activity} description="Post-processing average" />
                  <StatCard title="Post P50" value={formatLatency(summary.p50_router_post_ms)} icon={Percent} description="Median" />
                  <StatCard title="Post P95" value={formatLatency(summary.p95_router_post_ms)} icon={ArrowUp} description="95th percentile" />
                  <StatCard
                    title="Router Total Avg"
                    value={formatLatency(
                      summary.avg_router_pre_ms !== undefined && summary.avg_router_post_ms !== undefined
                        ? summary.avg_router_pre_ms + summary.avg_router_post_ms
                        : undefined
                    )}
                    icon={Timer}
                    description="Pre + Post combined"
                  />
                </>
              )}
            </div>
          </SectionCard>

          <SectionCard title="End-to-End Latency (Router + Model)">
            <div className="grid gap-4 md:grid-cols-3">
              {isLoading ? (
                <>
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                  <Skeleton className="h-32" />
                </>
              ) : (
                <>
                  <StatCard title="Total Avg" value={formatLatency(summary.avg_total_latency_ms)} icon={Gauge} description="Full request average" />
                  <StatCard title="Total P50" value={formatLatency(summary.p50_total_latency_ms)} icon={Percent} description="Median" />
                  <StatCard title="Total P95" value={formatLatency(summary.p95_total_latency_ms)} icon={ArrowUp} description="95th percentile" />
                  <StatCard title="LLM Avg" value={formatLatency(summary.avg_llm_latency_ms)} icon={Cpu} description="Model execution average" />
                  <StatCard title="LLM P50" value={formatLatency(summary.p50_llm_latency_ms)} icon={Percent} description="Median" />
                  <StatCard title="LLM P95" value={formatLatency(summary.p95_llm_latency_ms)} icon={ArrowUp} description="95th percentile" />
                </>
              )}
            </div>
          </SectionCard>
        </div>
      )}

      <SectionCard title="Total Latency Over Time">
        {isLoading ? (
          <Skeleton className="h-64" />
        ) : timeseries.length === 0 ? (
          <div className="h-[240px] flex items-center justify-center text-sm text-muted-foreground">No data</div>
        ) : (
          <ResponsiveContainer width="100%" height={260}>
            <LineChart key={chartKey} data={lineData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="bucket_label" />
              <YAxis />
              <Tooltip />
              <Line type="monotone" dataKey="avg_total_latency_ms" stroke="#0ea5e9" strokeWidth={3} name="Total (E2E)" dot={false} />
              <Line type="monotone" dataKey="avg_router_only_ms" stroke="#e11d48" strokeWidth={2} strokeDasharray="5 3" name="Router Only (Pre+Post)" dot={false} />
              <Line type="monotone" dataKey="avg_router_pre_ms" stroke="#6366f1" strokeWidth={2} name="Router Pre" dot={false} />
              <Line type="monotone" dataKey="avg_router_post_ms" stroke="#f59e0b" strokeWidth={2} name="Router Post" dot={false} />
              <Line type="monotone" dataKey="avg_llm_latency_ms" stroke="#10b981" strokeWidth={2} name="LLM" dot={false} />
              <Legend />
            </LineChart>
          </ResponsiveContainer>
        )}
      </SectionCard>

      <div className="grid gap-6 md:grid-cols-2">
        <SectionCard title="Router Breakdown">
          {isLoading ? (
            <Skeleton className="h-64" />
          ) : routerBreakdownData.length === 0 ? (
            <div className="h-[240px] flex items-center justify-center text-sm text-muted-foreground">No data</div>
          ) : (
            <ResponsiveContainer width="100%" height={300}>
              <BarChart key={`router-${chartKey}`} data={routerBreakdownData} margin={{ bottom: 50 }}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis
                  dataKey="label"
                  interval={0}
                  angle={-35}
                  textAnchor="end"
                  height={70}
                  tick={{ fontSize: 12 }}
                />
                <YAxis />
                <Tooltip formatter={(value) => formatLatency(Number(value))} />
                <Bar dataKey="value" fill="#0ea5e9" name="Avg ms" />
              </BarChart>
            </ResponsiveContainer>
          )}
        </SectionCard>

        <SectionCard title="Tool Routes Breakdown">
          {isLoading ? (
            <Skeleton className="h-64" />
          ) : toolRoutesData.length === 0 ? (
            <div className="h-[240px] flex items-center justify-center text-sm text-muted-foreground">No data</div>
          ) : (
            <ResponsiveContainer width="100%" height={300}>
              <BarChart key={`tools-${chartKey}`} data={toolRoutesData} margin={{ bottom: 50 }}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis
                  dataKey="label"
                  interval={0}
                  angle={-35}
                  textAnchor="end"
                  height={70}
                  tick={{ fontSize: 12 }}
                />
                <YAxis />
                <Tooltip formatter={(value) => formatLatency(Number(value))} />
                <Bar dataKey="value" fill="#6366f1" name="Avg ms" />
              </BarChart>
            </ResponsiveContainer>
          )}
        </SectionCard>
      </div>

      <SectionCard
        title="Latency Detail"
        action={
          <Button variant="outline" size="sm" onClick={exportCsv} disabled={timeseries.length === 0}>
            <Download className="mr-2 h-4 w-4" />
            Download CSV
          </Button>
        }
      >
        {isLoading ? (
          <Skeleton className="h-64" />
        ) : (
          <div className="border rounded-md">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time Stamp</TableHead>
                  <TableHead className="text-right">Requests</TableHead>
                  <TableHead className="text-right">Avg Router Pre</TableHead>
                  <TableHead className="text-right">Avg LLM</TableHead>
                  <TableHead className="text-right">Avg Router Post</TableHead>
                  <TableHead className="text-right">Avg Total</TableHead>
                  <TableHead className="text-right">P95 Router Pre</TableHead>
                  <TableHead className="text-right">P95 LLM</TableHead>
                  <TableHead className="text-right">P95 Router Post</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {timeseries.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={9} className="text-center text-muted-foreground">
                      No data
                    </TableCell>
                  </TableRow>
                ) : (
                  timeseries.map((row, idx) => (
                    <TableRow key={`${row.bucket_start}-${idx}`}>
                      <TableCell className="font-mono text-xs">{formatBucketLabel(row.bucket_start)}</TableCell>
                      <TableCell className="text-right tabular-nums">{row.requests ?? 0}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatLatency(row.avg_router_pre_ms)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatLatency(row.avg_llm_latency_ms)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatLatency(row.avg_router_post_ms)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatLatency(row.avg_total_latency_ms)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatLatency(row.p95_router_pre_ms)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatLatency(row.p95_llm_latency_ms)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatLatency(row.p95_router_post_ms)}</TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        )}
      </SectionCard>
    </div>
  )
}
