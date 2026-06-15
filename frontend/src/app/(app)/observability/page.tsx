'use client'

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Activity, CheckCircle, DollarSign, Zap, Search, GitBranch, Download, HeartPulse, Database, Route, Network, ShieldAlert, Gauge } from 'lucide-react'
import { useAuth } from '@/hooks/use-auth'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { useObservabilityGlobalAccess } from '@/features/observability/api/use-observability-access'
import { useObservabilityMetrics, useRequestLogs } from '@/features/observability/api/use-observability'
import { useModelPerformance } from '@/features/observability/api/use-real-metrics'
import { ModelPerformanceTable } from '@/features/observability/components/model-performance-table'
import {
  RequestLogsTable,
  type RequestLogSortDirection,
  type RequestLogSortField,
} from '@/features/observability/components/request-logs-table'
import { ProviderHealthTable } from '@/features/observability/components/provider-health-table'
import { RequestExplorerView } from '@/features/observability/components/request-explorer-dialog'
import { RoutingInsightsView } from '@/features/observability/components/routing-insights-view'
import { ModelHealthView } from '@/features/observability/components/model-health-view'
import { SemanticCacheView } from '@/features/observability/components/semantic-cache-view'
import { SemanticRoutingView } from '@/features/observability/components/semantic-routing-view'
import { SemanticCorrelationView } from '@/features/observability/components/semantic-correlation-view'
import { RouterPerformanceView } from '@/features/observability/components/router-performance-view'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import type { RequestLogEntry } from '@/features/observability/types'
import { useModels } from '@/features/models/api/use-models'
import {
  type ApiKeyUsageDrilldown,
  type ApiKeyUsageRow,
  fetchAllApiKeysUsage,
  fetchApiKeyDrilldown,
} from '@/features/observability/api/fetch-api-keys-usage'

type ViewMode = 'observability' | 'explorer' | 'routing' | 'model-health' | 'semantic-cache' | 'semantic-routing' | 'semantic-correlation' | 'router-performance'

function formatSmallCurrency(value: number | null): string {
  if (value === null || value === undefined) return '—'
  if (!Number.isFinite(value)) return '—'

  if (Math.abs(value) < 0.01) {
    const precise = value.toFixed(6).replace(/0+$/, '').replace(/\.$/, '')
    return `$${precise}`
  }

  return `$${value.toFixed(2)}`
}

export default function ObservabilityPage() {
  const { user, isRefreshingSession } = useAuth()
  const accessQuery = useObservabilityGlobalAccess()
  const canFetchObservability = Boolean(
    user && !isRefreshingSession && accessQuery.isSuccess
  )

  const [viewMode, setViewMode] = useState<ViewMode>('observability')
  const [logsPage, setLogsPage] = useState(0)
  const [logSortField, setLogSortField] = useState<RequestLogSortField>('timestamp')
  const [logSortDirection, setLogSortDirection] = useState<RequestLogSortDirection>('desc')
  const logLimit = 50
  const windowHours = 24

  const { data: modelsCatalog = [] } = useModels()
  const apiKeysUsageQuery = useQuery({
    queryKey: ['observability', 'api-keys-usage', windowHours],
    queryFn: () => fetchAllApiKeysUsage(windowHours),
    enabled: canFetchObservability,
  })
  const apiKeyUsageRows = apiKeysUsageQuery.data ?? []
  const isLoadingApiKeysUsage = apiKeysUsageQuery.isLoading
  const apiKeysUsageError = apiKeysUsageQuery.isError ? apiKeysUsageQuery.error : null

  const { data: modelPerformance = [], isLoading: modelsLoading } = useModelPerformance(windowHours, canFetchObservability)
  const { data: observabilityPanels, isLoading: panelsLoading, error: panelsError } = useObservabilityMetrics(
    windowHours,
    'hour',
    canFetchObservability
  )

  const apiKeyDrilldownQuery = useQuery({
    queryKey: [
      'observability',
      'api-keys-drilldown',
      windowHours,
      apiKeyUsageRows.map((row) => row.api_key_id).join(','),
    ],
    queryFn: async () => {
      const drilldowns = await Promise.all(
        apiKeyUsageRows.map((row) => fetchApiKeyDrilldown(row.api_key_id, windowHours).catch(() => null))
      )
      return drilldowns.filter((row): row is ApiKeyUsageDrilldown => row !== null)
    },
    enabled: canFetchObservability && apiKeyUsageRows.length > 0,
  })
  const apiKeyDrilldowns = apiKeyDrilldownQuery.data ?? []
  const isLoadingDrilldowns = apiKeyDrilldownQuery.isLoading
  const drilldownError = apiKeyDrilldownQuery.isError ? apiKeyDrilldownQuery.error : null
  const { data: logsData, isLoading: logsLoading } = useRequestLogs(
    logLimit,
    logsPage * logLimit,
    undefined,
    canFetchObservability
  )

  const logs = useMemo(() => logsData?.logs ?? [], [logsData?.logs])
  const pagination = logsData?.pagination

  const sortedLogs = useMemo(() => {
    const sorted = [...logs]
    sorted.sort((a: RequestLogEntry, b: RequestLogEntry) => {
      let compare = 0
      if (logSortField === 'timestamp') {
        compare = new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
      } else if (logSortField === 'latency_ms') {
        compare = (a.latency_ms || 0) - (b.latency_ms || 0)
      } else {
        const left = String(a[logSortField] ?? '').toLowerCase()
        const right = String(b[logSortField] ?? '').toLowerCase()
        compare = left.localeCompare(right)
      }
      return logSortDirection === 'asc' ? compare : -compare
    })
    return sorted
  }, [logs, logSortDirection, logSortField])

  const modelCatalogMap = useMemo(() => {
    const map = new Map<string, { provider: string; type?: string }>()
    for (const model of modelsCatalog) {
      const id = String(model.id ?? '').trim().toLowerCase()
      if (!id) continue
      map.set(id, { provider: String(model.provider ?? '').trim(), type: String(model.type ?? '') || undefined })
    }
    return map
  }, [modelsCatalog])

  const totalRequests = useMemo(
    () => apiKeyUsageRows.reduce((sum, row) => sum + row.requests, 0),
    [apiKeyUsageRows]
  )

  const totalCost = useMemo(
    () =>
      apiKeyUsageRows.reduce(
        (sum, row) => sum + (row.avg_cost_per_request_effective ?? 0) * row.requests,
        0
      ),
    [apiKeyUsageRows]
  )

  const weightedSuccessRate = useMemo(() => {
    if (totalRequests <= 0) return 0
    const total = apiKeyUsageRows.reduce(
      (sum, row) => sum + (row.success_rate ?? 0) * row.requests,
      0
    )
    return total / totalRequests
  }, [apiKeyUsageRows, totalRequests])

  const weightedAvgLatency = useMemo(() => {
    if (totalRequests <= 0) return 0
    const total = apiKeyUsageRows.reduce(
      (sum, row) => sum + (row.avg_latency_ms ?? 0) * row.requests,
      0
    )
    return total / totalRequests
  }, [apiKeyUsageRows, totalRequests])

  const logStatsByModel = useMemo(() => {
    const map = new Map<string, { count: number; successCount: number; latencies: number[]; providers: Map<string, number> }>()
    for (const log of logs) {
      const model = String(log.model ?? '-').trim() || '-'
      const provider = String(log.provider ?? '-').trim() || '-'
      const latency = Number(log.latency_ms ?? 0)
      const status = String(log.status ?? '').toLowerCase()
      const isSuccess = status === 'success' || status === 'ok'
      const entry =
        map.get(model) ?? {
          count: 0,
          successCount: 0,
          latencies: [] as number[],
          providers: new Map<string, number>(),
        }
      entry.count += 1
      entry.successCount += isSuccess ? 1 : 0
      entry.latencies.push(latency)
      entry.providers.set(provider, (entry.providers.get(provider) ?? 0) + 1)
      map.set(model, entry)
    }
    return map
  }, [logs])

  const logStatsByProvider = useMemo(() => {
    const map = new Map<string, { count: number; successCount: number; latencies: number[] }>()
    for (const log of logs) {
      const provider = String(log.provider ?? '-').trim() || '-'
      const latency = Number(log.latency_ms ?? 0)
      const status = String(log.status ?? '').toLowerCase()
      const isSuccess = status === 'success' || status === 'ok'
      const entry = map.get(provider) ?? { count: 0, successCount: 0, latencies: [] as number[] }
      entry.count += 1
      entry.successCount += isSuccess ? 1 : 0
      entry.latencies.push(latency)
      map.set(provider, entry)
    }
    return map
  }, [logs])

  const percentile = (values: number[], pct: number): number => {
    if (values.length === 0) return 0
    const sorted = [...values].sort((a, b) => a - b)
    const index = Math.max(0, Math.min(sorted.length - 1, Math.ceil((pct / 100) * sorted.length) - 1))
    return sorted[index] ?? 0
  }

  const modelPerformanceRows = useMemo(() => {
    const agg = new Map<string, { requests: number; effectiveSpend: number }>()
    for (const drilldown of apiKeyDrilldowns) {
      for (const row of drilldown.requests_by_model) {
        const model = String(row.model ?? '-').trim() || '-'
        const entry = agg.get(model) ?? { requests: 0, effectiveSpend: 0 }
        entry.requests += Number(row.requests ?? 0)
        entry.effectiveSpend += Number(row.effective_spend ?? 0)
        agg.set(model, entry)
      }
    }

    for (const model of modelPerformance) {
      const modelId = String(model.model ?? '-').trim() || '-'
      if (!agg.has(modelId)) {
        agg.set(modelId, { requests: 0, effectiveSpend: 0 })
      }
    }

    return Array.from(agg.entries())
      .map(([model, entry]) => {
        const perf = modelPerformance.find((row) => row.model === model)
        const stats = logStatsByModel.get(model)
        const catalog = modelCatalogMap.get(model.toLowerCase())
        const providerFromLogs = (() => {
          if (!stats) return ''
          let best = ''
          let bestCount = 0
          for (const [provider, count] of Array.from(stats.providers.entries())) {
            if (count > bestCount) {
              best = provider
              bestCount = count
            }
          }
          return best
        })()
        const avgLatency =
          perf?.avg_latency_ms ?? (stats && stats.count > 0 ? stats.latencies.reduce((sum, v) => sum + v, 0) / stats.count : 0)
        const p95Latency = perf?.p95_latency_ms ?? (stats ? percentile(stats.latencies, 95) : 0)
        const successRate =
          perf?.success_rate ?? (stats && stats.count > 0 ? stats.successCount / stats.count : 0)
        const avgCost = entry.requests > 0 ? entry.effectiveSpend / entry.requests : 0
        const samples = entry.requests > 0 ? entry.requests : perf?.samples ?? stats?.count ?? 0
        return {
          model,
          provider: perf?.provider || catalog?.provider || providerFromLogs || '-',
          avg_latency_ms: avgLatency,
          p95_latency_ms: p95Latency,
          success_rate: successRate,
          avg_cost_usd: avgCost,
          samples,
        }
      })
      .sort((a, b) => b.samples - a.samples)
  }, [apiKeyDrilldowns, logStatsByModel, modelCatalogMap, modelPerformance])

  const providerHealthRows = useMemo(() => {
    const requestMap = new Map<string, number>()
    for (const drilldown of apiKeyDrilldowns) {
      for (const row of drilldown.requests_by_provider) {
        const provider = String(row.provider ?? '-').trim() || '-'
        requestMap.set(provider, (requestMap.get(provider) ?? 0) + Number(row.requests ?? 0))
      }
    }

    const panelProviders = observabilityPanels?.provider_health ?? []
    for (const row of panelProviders) {
      const provider = String(row.provider ?? '-').trim() || '-'
      if (!requestMap.has(provider)) {
        requestMap.set(provider, Number(row.total_requests ?? 0))
      }
    }

    const providers = new Set<string>([
      ...Array.from(requestMap.keys()),
      ...Array.from(logStatsByProvider.keys()),
    ])
    return Array.from(providers)
      .map((provider) => {
        const stats = logStatsByProvider.get(provider)
        const panelRow = panelProviders.find((row) => row.provider === provider)
        const totalRequests = requestMap.get(provider) ?? panelRow?.total_requests ?? stats?.count ?? 0
        const successRate =
          panelRow?.success_rate ?? (stats && stats.count > 0 ? (stats.successCount / stats.count) * 100 : 0)
        const avgLatency =
          panelRow?.avg_latency ?? (stats && stats.count > 0 ? stats.latencies.reduce((sum, v) => sum + v, 0) / stats.count : 0)
        return {
          provider,
          success_rate: successRate,
          avg_latency: Number(avgLatency.toFixed(1)),
          total_requests: totalRequests,
        }
      })
      .sort((a, b) => b.total_requests - a.total_requests)
  }, [apiKeyDrilldowns, logStatsByProvider, observabilityPanels?.provider_health])

  const trafficData = useMemo(() => {
    if (observabilityPanels?.traffic_data?.length) {
      return observabilityPanels.traffic_data
    }
    const map = new Map<string, { requests: number; errors: number }>()
    for (const drilldown of apiKeyDrilldowns) {
      for (const row of drilldown.traffic_over_time) {
        const bucket = String(row.bucket ?? '')
        if (!bucket) continue
        const entry = map.get(bucket) ?? { requests: 0, errors: 0 }
        entry.requests += Number(row.requests ?? 0)
        entry.errors += Number(row.errors ?? 0)
        map.set(bucket, entry)
      }
    }
    return Array.from(map.entries())
      .map(([bucket, values]) => ({ time_bucket: bucket, requests: values.requests, errors: values.errors }))
      .sort((a, b) => new Date(a.time_bucket).getTime() - new Date(b.time_bucket).getTime())
  }, [apiKeyDrilldowns, observabilityPanels?.traffic_data])

  const handleLogSortChange = (field: RequestLogSortField) => {
    if (field === logSortField) {
      setLogSortDirection((prev) => (prev === 'asc' ? 'desc' : 'asc'))
      return
    }
    setLogSortField(field)
    setLogSortDirection('asc')
  }

  const currentOffset = pagination?.offset ?? logsPage * logLimit
  const currentLimit = pagination?.limit ?? logLimit
  const currentPage = Math.floor(currentOffset / currentLimit)
  const totalLogs = pagination?.total ?? logsData?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(totalLogs / currentLimit))

  const exportLogsToCsv = () => {
    const headers = [
      'timestamp',
      'tenant_id',
      'model',
      'provider',
      'latency_ms',
      'status',
      'fallback_used',
      'cache.status',
    ]
    const rows = sortedLogs.map((log) => [
      log.timestamp,
      log.tenant_id,
      log.model,
      log.provider,
      String(log.latency_ms ?? ''),
      log.status,
      String(log.fallback_used),
      log.cache_status ?? '',
    ])
    const content = [headers.join(','), ...rows.map((r) => r.join(','))].join('\n')
    const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const now = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    const fileName = `request_logs_${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}.csv`

    const link = document.createElement('a')
    link.href = url
    link.download = fileName
    link.click()
    URL.revokeObjectURL(url)
  }

  if (user && isRefreshingSession) {
    return (
      <div>
        <PageHeader
          title="Observability"
          description="Real-time metrics from production backend"
        />
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-6">
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
        </div>
        <Skeleton className="h-64 mb-6" />
        <Skeleton className="h-64" />
      </div>
    )
  }

  if (user && accessQuery.isError) {
    const err = accessQuery.error as Error & { status?: number }
    const isAccessDenied = err.status === 403 || err.status === 401
    return (
      <div>
        <PageHeader
          title="Observability"
          description="Real-time metrics from production backend"
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
              <p className="text-destructive mb-2">Unable to load observability</p>
              <p className="text-sm text-muted-foreground">
                {accessQuery.error instanceof Error ? accessQuery.error.message : 'Unknown error'}
              </p>
            </div>
          )}
        </SectionCard>
      </div>
    )
  }

  if (user && accessQuery.isLoading) {
    return (
      <div>
        <PageHeader
          title="Observability"
          description="Real-time metrics from production backend"
        />
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-6">
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
        </div>
        <Skeleton className="h-64 mb-6" />
        <Skeleton className="h-64" />
      </div>
    )
  }

  if (viewMode === 'explorer') {
    return <RequestExplorerView onBack={() => setViewMode('observability')} />
  }

  if (viewMode === 'routing') {
    return <RoutingInsightsView onBack={() => setViewMode('observability')} />
  }

  if (viewMode === 'model-health') {
    return <ModelHealthView onBack={() => setViewMode('observability')} />
  }

  if (viewMode === 'semantic-cache') {
    return <SemanticCacheView onBack={() => setViewMode('observability')} />
  }

  if (viewMode === 'semantic-routing') {
    return <SemanticRoutingView onBack={() => setViewMode('observability')} />
  }

  if (viewMode === 'semantic-correlation') {
    return <SemanticCorrelationView onBack={() => setViewMode('observability')} />
  }

  if (viewMode === 'router-performance') {
    return <RouterPerformanceView onBack={() => setViewMode('observability')} />
  }

  return (
    <RequireAdminRole allowedRoles={['admin', 'audit', 'finance']}>
    <div>
      <PageHeader
        title="Observability"
        description="Real-time metrics from production backend"
      />

      {/* Navigation grid */}
      {(() => {
        const obsNav = [
          { label: 'Router Perf.', mode: 'router-performance' as ViewMode, icon: Gauge },
          { label: 'Semantic Corr.', mode: 'semantic-correlation' as ViewMode, icon: Network },
          { label: 'Sem. Routing', mode: 'semantic-routing' as ViewMode, icon: Route },
          { label: 'Semantic Cache', mode: 'semantic-cache' as ViewMode, icon: Database },
          { label: 'Model Health', mode: 'model-health' as ViewMode, icon: HeartPulse },
          { label: 'Routing', mode: 'routing' as ViewMode, icon: GitBranch },
          { label: 'Request Explorer', mode: 'explorer' as ViewMode, icon: Search },
        ]
        const cols = Math.ceil(obsNav.length / 2)
        return (
          <div
            className="mt-4 mb-6 grid gap-1.5"
            style={{ gridTemplateColumns: `repeat(${cols}, 1fr)` }}
          >
            {obsNav.map((item) => (
              <Button
                key={item.label}
                variant="outline"
                className="flex h-16 flex-col items-center justify-center gap-1 px-1"
                onClick={() => setViewMode(item.mode)}
              >
                <item.icon className="h-4 w-4 shrink-0" />
                <span className="text-center text-[11px] font-medium leading-tight">{item.label}</span>
              </Button>
            ))}
          </div>
        )
      })()}

      {/* Summary Metrics - API Key Usage */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-6">
        {isLoadingApiKeysUsage ? (
          <>
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
          </>
        ) : (
          <>
            <StatCard
              title="Total Requests"
              value={totalRequests.toLocaleString()}
              icon={Activity}
              description="From API key usage"
              className="border-t-4 border-t-pink-500"
            />
            <StatCard
              title="Total Cost"
              value={formatSmallCurrency(totalCost)}
              icon={DollarSign}
              description="Effective cost"
              className="border-t-4 border-t-amber-400"
            />
            <StatCard
              title="Success Rate"
              value={`${(weightedSuccessRate * 100).toFixed(1)}%`}
              icon={CheckCircle}
              description="Weighted by requests"
              className="border-t-4 border-t-emerald-500"
            />
            <StatCard
              title="Avg Latency"
              value={`${weightedAvgLatency.toFixed(0)}ms`}
              icon={Zap}
              description="Weighted by requests"
              className="border-t-4 border-t-cyan-400"
            />
          </>
        )}
      </div>

      {/* Model Performance - FROM REAL BACKEND */}
      <div className="grid gap-6 mb-6">
        <SectionCard title="Model Performance" className="border-t-4 border-t-purple-500">
          {isLoadingApiKeysUsage || isLoadingDrilldowns || logsLoading || modelsLoading ? (
            <Skeleton className="h-64" />
          ) : apiKeysUsageError || drilldownError ? (
            <div className="h-[250px] flex items-center justify-center">
              <div className="text-center space-y-2">
                <p className="text-sm text-destructive">Failed to load model performance</p>
                <p className="text-xs text-muted-foreground">
                  {apiKeysUsageError instanceof Error
                    ? apiKeysUsageError.message
                    : drilldownError instanceof Error
                    ? drilldownError.message
                    : 'Backend error retrieving API key usage'}
                </p>
              </div>
            </div>
          ) : (
            <ModelPerformanceTable models={modelPerformanceRows} />
          )}
        </SectionCard>
      </div>

      {/* Traffic Over Time - placeholder until endpoint exists */}
      <div className="grid gap-6 mb-6">
        <SectionCard title="Traffic Over Time" className="border-t-4 border-t-blue-500">
          {panelsLoading ? (
            <Skeleton className="h-64" />
          ) : isLoadingApiKeysUsage || isLoadingDrilldowns ? (
            <Skeleton className="h-64" />
          ) : panelsError || apiKeysUsageError || drilldownError ? (
            <div className="h-[250px] flex items-center justify-center">
              <div className="text-center space-y-2">
                <p className="text-sm text-destructive">Failed to load traffic data</p>
                <p className="text-xs text-muted-foreground">
                  {panelsError instanceof Error
                    ? panelsError.message
                    : apiKeysUsageError instanceof Error
                    ? apiKeysUsageError.message
                    : drilldownError instanceof Error
                    ? drilldownError.message
                    : 'Backend error retrieving API key usage'}
                </p>
              </div>
            </div>
          ) : trafficData.length === 0 ? (
            <div className="h-[250px] flex items-center justify-center text-sm text-muted-foreground">
              No traffic data available
            </div>
          ) : (
            <ResponsiveContainer width="100%" height={250}>
              <LineChart data={trafficData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis
                  dataKey="time_bucket"
                  tickFormatter={(value) =>
                    value ? new Date(value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : ''
                  }
                />
                <YAxis />
                <Tooltip labelFormatter={(value) => new Date(String(value)).toLocaleString()} />
                <Line type="monotone" dataKey="requests" stroke="#8884d8" strokeWidth={2} name="Requests" />
                <Line type="monotone" dataKey="errors" stroke="#ef4444" strokeWidth={2} name="Errors" />
              </LineChart>
            </ResponsiveContainer>
          )}
        </SectionCard>
      </div>

      {/* Provider Health */}
      <div className="grid gap-6 mb-6">
        <SectionCard title="Provider Health" className="border-t-4 border-t-amber-400">
          {panelsLoading || isLoadingApiKeysUsage || isLoadingDrilldowns || logsLoading ? (
            <Skeleton className="h-48" />
          ) : panelsError || apiKeysUsageError || drilldownError ? (
            <div className="h-48 flex items-center justify-center">
              <div className="text-center space-y-2">
                <p className="text-sm text-destructive">Failed to load provider health data</p>
                <p className="text-xs text-muted-foreground">
                  {panelsError instanceof Error
                    ? panelsError.message
                    : apiKeysUsageError instanceof Error
                    ? apiKeysUsageError.message
                    : drilldownError instanceof Error
                    ? drilldownError.message
                    : 'Backend error retrieving API key usage'}
                </p>
              </div>
            </div>
          ) : (
            <ProviderHealthTable providers={providerHealthRows} />
          )}
        </SectionCard>
      </div>

      {/* Recent Request Logs */}
      <div className="grid gap-6 mb-6">
        <SectionCard
          title="Recent Request Logs"
          className="border-t-4 border-t-cyan-400"
          action={
            <Button variant="outline" size="sm" onClick={exportLogsToCsv} disabled={sortedLogs.length === 0}>
              <Download className="mr-2 h-4 w-4" />
              Export CSV
            </Button>
          }
        >
          {logsLoading ? (
            <Skeleton className="h-64" />
          ) : (
            <>
              <RequestLogsTable
                logs={sortedLogs}
                sortField={logSortField}
                sortDirection={logSortDirection}
                onSortChange={handleLogSortChange}
              />
              <div className="mt-4 flex items-center justify-between">
                <div className="text-sm text-muted-foreground">
                  Showing {logs.length === 0 ? 0 : currentOffset + 1} - {Math.min(currentOffset + logs.length, totalLogs)} of {totalLogs}
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setLogsPage((prev) => Math.max(0, prev - 1))}
                    disabled={currentPage === 0}
                  >
                    Previous
                  </Button>
                  {Array.from({ length: totalPages }, (_, i) => i)
                    .filter((p) => p >= Math.max(0, currentPage - 2) && p <= Math.min(totalPages - 1, currentPage + 2))
                    .map((p) => (
                      <Button
                        key={p}
                        variant={p === currentPage ? 'default' : 'outline'}
                        size="sm"
                        onClick={() => setLogsPage(p)}
                      >
                        {p + 1}
                      </Button>
                    ))}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setLogsPage((prev) => Math.min(totalPages - 1, prev + 1))}
                    disabled={currentPage >= totalPages - 1}
                  >
                    Next
                  </Button>
                </div>
              </div>
            </>
          )}
        </SectionCard>
      </div>
    </div>
    </RequireAdminRole>
  )
}
