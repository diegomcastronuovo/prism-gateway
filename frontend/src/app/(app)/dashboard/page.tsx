'use client'

import { useEffect, useMemo, useState } from 'react'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { StatCard } from '@/components/shared/stat-card'
import { Skeleton } from '@/components/ui/skeleton'
import { useDashboard } from '@/features/dashboard/api/use-dashboard'
import { SystemInfoPanel } from '@/features/dashboard/components/system-info-panel'
import { SystemHealthCard } from '@/features/dashboard/components/system-health-card'
import { Server, Users, Layers, GitBranch, Boxes, DollarSign, Zap, TrendingDown, ShieldCheck } from 'lucide-react'
import { SectionCard } from '@/components/shared/section-card'
import { useGlobalConfig } from '@/features/global-config/api/use-global-config'
import { useModelBenchmarks } from '@/features/models/api/use-models'
import { useTenantBudgets } from '@/features/budgets/api/use-budgets'

function DashboardContent() {
  const { data, isLoading, error } = useDashboard()
  const { data: globalConfigData } = useGlobalConfig(true)
  const { data: modelBenchmarks } = useModelBenchmarks(24)
  const { data: tenantBudgets } = useTenantBudgets()
  const [feVersion, setFeVersion] = useState<{ version?: string; details?: string } | null>(null)

  useEffect(() => {
    let mounted = true
    async function loadVersion() {
      try {
        const res = await fetch('/api/version', { cache: 'no-store' })
        if (!res.ok) return
        const v = await res.json()
        if (mounted) setFeVersion({ version: v.version, details: v.details })
      } catch {}
    }
    loadVersion()
    return () => { mounted = false }
  }, [])

  const dashboardError = error as Error | null
  const hasDashboardData = Boolean(data)
  const globalConfig = globalConfigData?.config as Record<string, unknown> | undefined

  const benchmarks = modelBenchmarks && modelBenchmarks.length > 0 ? modelBenchmarks : (data?.benchmarks ?? [])

  const totalBudgetUSD = useMemo(() => {
    if (!tenantBudgets) return null
    const sum = tenantBudgets.reduce((acc, b) => acc + (b.monthly_usd ?? 0), 0)
    return sum
  }, [tenantBudgets])

  const fastestModel = useMemo(() => {
    if (!benchmarks.length) return null
    return benchmarks.reduce((best, b) => b.avg_latency_ms < best.avg_latency_ms ? b : best)
  }, [benchmarks])

  const cheapestModel = useMemo(() => {
    if (!benchmarks.length) return null
    const withCost = benchmarks.filter(b => b.avg_cost_usd > 0)
    if (!withCost.length) return null
    return withCost.reduce((best, b) => b.avg_cost_usd < best.avg_cost_usd ? b : best)
  }, [benchmarks])

  const mostReliableModel = useMemo(() => {
    if (!benchmarks.length) return null
    return benchmarks.reduce((best, b) => b.success_rate > best.success_rate ? b : best)
  }, [benchmarks])

  const summaryCards = useMemo(() => {
    if (!data && !globalConfig) return null

    const modelsCount = Array.isArray(globalConfig?.models)
      ? globalConfig.models.length
      : data?.models?.length

    const providersCount = globalConfig?.providers && typeof globalConfig.providers === 'object'
      ? Object.keys(globalConfig.providers as Record<string, unknown>).length
      : data?.providers?.length

    const routeGroupsCount = Array.isArray((globalConfig as Record<string, unknown> | undefined)?.route_groups)
      ? ((globalConfig as Record<string, unknown>).route_groups as unknown[]).length
      : data?.routeGroups?.length

    return (
      <>
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-5 mb-6">
          <StatCard
            title="Backend Version"
            value={data?.version?.backend_version || 'Unknown'}
            icon={Server}
            description={data?.version?.release_notes || 'No release notes'}
            className="border-t-4 border-t-pink-500"
          />
          <StatCard
            title="Tenants"
            value={(data?.tenants?.length || 0).toString()}
            icon={Users}
            description="View tenants"
            href="/tenants"
            className="border-t-4 border-t-cyan-400"
          />
          <StatCard
            title="Models"
            value={String(modelsCount ?? 0)}
            icon={Layers}
            description="View models"
            href="/models"
            className="border-t-4 border-t-purple-500"
          />
          <StatCard
            title="Providers"
            value={String(providersCount ?? 0)}
            icon={Boxes}
            description="View providers"
            href="/providers"
            className="border-t-4 border-t-amber-400"
          />
          <StatCard
            title="Route Groups"
            value={String(routeGroupsCount ?? 0)}
            icon={GitBranch}
            description="View route groups"
            href="/route-groups"
            className="border-t-4 border-t-blue-500"
          />
        </div>
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-4 mb-6">
          <StatCard
            title="Total Budget"
            value={totalBudgetUSD !== null ? `$${totalBudgetUSD.toLocaleString('en-US', { maximumFractionDigits: 0 })}` : '—'}
            icon={DollarSign}
            description="Sum of monthly budgets"
            href="/budgets"
            className="border-t-4 border-t-emerald-500"
          />
          <StatCard
            title="Fastest Model"
            value={fastestModel ? fastestModel.model : '—'}
            icon={Zap}
            description={fastestModel ? `${fastestModel.avg_latency_ms.toFixed(0)} ms avg latency` : 'No benchmark data'}
            href="/observability"
            className="border-t-4 border-t-yellow-400"
          />
          <StatCard
            title="Cheapest Model"
            value={cheapestModel ? cheapestModel.model : '—'}
            icon={TrendingDown}
            description={cheapestModel ? `$${cheapestModel.avg_cost_usd.toFixed(6)} avg cost` : 'No benchmark data'}
            href="/observability"
            className="border-t-4 border-t-teal-500"
          />
          <StatCard
            title="Most Reliable"
            value={mostReliableModel ? mostReliableModel.model : '—'}
            icon={ShieldCheck}
            description={mostReliableModel ? `${(mostReliableModel.success_rate * 100).toFixed(1)}% success rate` : 'No benchmark data'}
            href="/observability"
            className="border-t-4 border-t-green-500"
          />
        </div>
      </>
    )
  }, [data, globalConfig, totalBudgetUSD, fastestModel, cheapestModel, mostReliableModel])

  return (
    <div>
      <PageHeader
        title="Dashboard"
        description="Overview of your AI Gateway metrics and activity"
        action={
          <div className="rounded-lg border border-t-4 border-t-purple-500 px-4 py-2 text-right min-w-[240px] max-w-[280px] bg-card">
            <div className="text-sm font-medium">
              Frontend Version: {feVersion?.version ?? '—'}
            </div>
            {feVersion?.details && (
              <div className="text-xs text-muted-foreground mt-0.5">
                {feVersion.details}
              </div>
            )}
          </div>
        }
      />

      {isLoading && !hasDashboardData ? (
        <>
          <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-5 mb-6">
            {[...Array(5)].map((_, i) => (
              <Skeleton key={i} className="h-32" />
            ))}
          </div>
          <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-5 mb-6">
            {[...Array(5)].map((_, i) => (
              <Skeleton key={i} className="h-32" />
            ))}
          </div>
        </>
      ) : dashboardError && !hasDashboardData ? (
        <SectionCard
          title="Dashboard Data"
          className="border-t-4 border-t-pink-500"
        >
          <div className="text-center py-8">
            <p className="text-destructive mb-2">Failed to load dashboard data</p>
            <p className="text-sm text-muted-foreground">{dashboardError.message}</p>
          </div>
        </SectionCard>
      ) : (
        summaryCards
      )}

      <div className="grid gap-6 md:grid-cols-2 mb-6">
        <div>
          {isLoading && !hasDashboardData ? (
            <Skeleton className="h-64" />
          ) : dashboardError && !hasDashboardData ? (
            <SectionCard
              title="System Information"
              className="border-t-4 border-t-cyan-400"
            >
              <div className="text-center py-8">
                <p className="text-destructive mb-2">Failed to load system information</p>
                <p className="text-sm text-muted-foreground">{dashboardError.message}</p>
              </div>
            </SectionCard>
          ) : data ? (
            <SystemInfoPanel version={data.version} />
          ) : (
            <SectionCard
              title="System Information"
              className="border-t-4 border-t-cyan-400"
            >
              <div className="text-center py-8 text-muted-foreground">
                No system information available
              </div>
            </SectionCard>
          )}
        </div>
        <SystemHealthCard />
      </div>
    </div>
  )
}

export default function DashboardPage() {
  return (
    <RequireAdminRole allowedRoles={['admin', 'local_admin', 'user', 'audit', 'finance']}>
      <DashboardContent />
    </RequireAdminRole>
  )
}
