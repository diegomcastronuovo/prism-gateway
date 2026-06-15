'use client'

import { useMemo, useState, type ComponentProps } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard as BaseSectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Wallet, DollarSign, TrendingDown, AlertCircle, Download, Gauge, Table as TableIcon, Info, HelpCircle, Database, Zap, TrendingUp, CheckCircle, Star, DollarSignIcon, Clock, Sparkles, AlertTriangle, KeyRound, Coins, ShieldAlert } from 'lucide-react'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { useTenantBudgets, type TenantBudget } from '@/features/budgets/api/use-budgets'
import { BudgetsTable } from '@/features/budgets/components/budgets-table'
import { EditBudgetDialog } from '@/features/budgets/components/edit-budget-dialog'
import { useModelPerformance } from '@/features/observability/api/use-real-metrics'
import { useModels } from '@/features/models/api/use-models'
import { formatCurrency } from '@/lib/utils/format'
import { assertFinopsUnauthorized, fetchTenantRequestStatsBatch } from '@/lib/finops-fetch'
import { useToast } from '@/hooks/use-toast'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils/cn'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

const SectionCard = (props: ComponentProps<typeof BaseSectionCard>) => (
  <BaseSectionCard
    {...props}
    className={cn('border-t-4 border-t-pink-500', props.className)}
  />
)

function BarChartSection({
  title,
  budgets,
  valueAccessor,
  valueLabel,
  maxValue,
}: {
  title: string
  budgets: TenantBudget[]
  valueAccessor: (budget: TenantBudget) => number
  valueLabel: (value: number, budget: TenantBudget) => string
  maxValue?: number
}) {
  const max = maxValue || budgets.reduce((m, b) => Math.max(m, valueAccessor(b)), 0)

  return (
    <SectionCard title={title}>
      <div className="space-y-3">
        {budgets.map((budget) => {
          const value = valueAccessor(budget)
          const width = max > 0 ? Math.max((value / max) * 100, 2) : 0

          return (
            <div key={budget.tenant_id} className="space-y-1">
              <div className="flex items-center justify-between text-sm">
                <span className="font-medium">{budget.tenant_id}</span>
                <span className="text-muted-foreground">{valueLabel(value, budget)}</span>
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

type WindowHours = 24 | 168 | 720
type FinOpsViewMode = 'budgets' | 'dashboard' | 'analytics' | 'anomalies' | 'api_keys_usage' | 'jwt_sub_usage' | 'api_keys_raw_usage' | 'jwt_sub_raw_usage' | 'models_monetization'

type UsageSummary = {
  tenant_id: string
  total_cost_usd: number
  total_requests: number
  models: { model: string; cost_usd: number; requests: number }[]
}

type TenantAnalyticsRow = {
  tenant_id: string
  requests: number
  spend_usd: number
  success_rate: number | null
  avg_latency_ms: number | null
  top_model: string | null
  budget_usd: number | null
  remaining_budget_usd: number | null
  utilization_pct: number | null
  status: string
  models: { model: string; requests: number; cost_usd: number }[]
}

type ApiKeyUsageRow = {
  api_key_id: string
  api_key_name: string
  tenant_id: string
  requests: number
  total_cost_usd: number
  avg_cost_per_request_effective?: number | null
  success_rate: number | null
  avg_latency_ms: number | null
  top_model: string | null
  top_provider: string | null
  last_seen_at: string | null
}

type JwtSubUsageRow = {
  jwt_sub: string
  tenant_id: string
  requests: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  total_cost_usd: number
  avg_cost_per_request_effective?: number | null
  first_seen: string | null
  last_seen: string | null
  success_rate?: number | null
  avg_latency_ms?: number | null
  top_model?: string | null
  top_provider?: string | null
}

type ApiKeyUsageListResponse = {
  data: ApiKeyUsageRow[]
  object: string
  summary?: {
    total_active_api_keys: number
    total_requests: number
    total_spend: number
    avg_success_rate: number | null
    highest_spend_key: string | null
    most_active_key: string | null
  }
  pagination: {
    limit: number
    offset: number
    returned: number
    total: number
  }
}

type JwtSubUsageListResponse = {
  data: JwtSubUsageRow[]
  object: string
  summary?: {
    total_active_jwt_subs: number
    total_requests: number
    total_spend: number
    avg_success_rate: number | null
    highest_spend_sub: string | null
    most_active_sub: string | null
  }
  pagination: {
    limit: number
    offset: number
    returned: number
    total: number
  }
}

type ApiKeyUsageDrilldown = {
  api_key_id: string
  api_key_name: string
  tenant_id: string
  summary: {
    requests: number
    total_cost_usd: number
    avg_cost_per_request_effective?: number | null
    success_rate: number | null
    avg_latency_ms: number | null
    top_model: string | null
    top_provider: string | null
    last_seen_at: string | null
  }
  requests_by_model: { model: string; requests: number; cost_usd: number }[]
  requests_by_provider: { provider: string; requests: number }[]
  traffic_over_time: { bucket: string; requests: number; errors: number }[]
  recent_requests: {
    request_id: string
    timestamp: string
    model: string
    provider: string
    latency_ms: number
    status: string
    fallback_used: boolean
    cost_usd?: number
    error_type?: string
  }[]
  latency_percentiles?: { p50: number; p95: number; max: number }
}

type ApiKeyMonetizationRow = {
  api_key_id: string
  api_key_name: string
  tenant_id: string
  requests: number
  spend: number
  avg_cost_per_request_effective: number | null
  avg_price_per_request: number | null
  total_price: number | null
  margin: number | null
  margin_pct: number | null
  avg_latency_ms: number | null
  success_rate: number | null
  top_model: string | null
  last_seen: string | null
}

type ApiKeyMonetizationDetail = {
  api_key_id: string
  api_key_name: string
  tenant_id: string
  summary: {
    requests: number
    spend: number
    avg_cost_per_request_effective: number | null
    avg_price_per_request: number | null
    total_price: number | null
    margin: number | null
    margin_pct: number | null
    avg_latency_ms: number | null
    success_rate: number | null
    last_seen: string | null
    top_model: string | null
    top_provider: string | null
  }
  requests_by_model: Array<{
    model: string
    requests: number
    spend: number
    /** Effective cost (ML/infra); prefer over token-only `spend` for FinOps dashboard */
    effective_spend: number
    avg_cost_per_request_effective?: number | null
    avg_price_per_request?: number | null
    total_price?: number | null
    margin?: number | null
    margin_pct?: number | null
  }>
  recent_requests: Array<{
    request_id: string
    timestamp: string
    model: string
    provider: string
    status: string
    latency_ms: number | null
    cost_usd: number
  }>
}

type JwtMonetizationRow = {
  jwt_sub: string
  tenant_id: string
  requests: number
  total_tokens: number
  total_cost_usd: number
  avg_cost_per_request_effective: number | null
  avg_price_per_request: number | null
  total_price: number | null
  margin: number | null
  margin_pct: number | null
  first_seen: string | null
  last_seen: string | null
}

type JwtSubUsageDrilldown = {
  jwt_sub: string
  tenant_id?: string | null
  summary: {
    requests: number
    total_cost_usd: number
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
  requests_by_model: { model: string; requests: number; cost_usd: number; total_tokens?: number }[]
  requests_by_provider: { provider: string; requests: number; cost_usd: number; total_tokens?: number }[]
  traffic_over_time: { bucket: string; requests: number; total_cost_usd?: number; total_tokens?: number }[]
}

type ApiKeyRawUsageRow = {
  timestamp: string
  tenant_id: string
  api_key_name: string
  api_key_id: string
  request_id: string
  model: string
  provider: string
  status: string
  latency_ms: number
  cost_usd: number
  prompt_tokens: number
  cached_tokens?: number
  completion_tokens: number
  total_tokens: number
}

type ApiKeyRawUsageResponse = {
  data: ApiKeyRawUsageRow[]
  object: string
  pagination: {
    limit: number
    offset: number
    returned: number
    total: number
  }
  endpoint_available: boolean
  error_message?: string
}

type JwtSubRawUsageRow = {
  timestamp: string
  tenant_id: string
  jwt_sub: string
  request_id: string
  model: string
  provider: string
  status: string
  latency_ms: number
  cost_usd: number
  prompt_tokens: number
  cached_tokens?: number
  completion_tokens: number
  total_tokens: number
}

type JwtSubRawUsageResponse = {
  data: JwtSubRawUsageRow[]
  object: string
  pagination: {
    limit: number
    offset: number
    returned: number
    total: number
  }
  endpoint_available: boolean
  error_message?: string
}


function resolveProviderFromModel(modelName: string, modelProviderMap: Map<string, string>): string {
  const normalized = String(modelName ?? '').trim().toLowerCase()
  if (!normalized) return 'other'
  const mapped = modelProviderMap.get(normalized)
  if (mapped) return mapped
  if (normalized.includes('gpt') || normalized.includes('openai') || normalized.includes('o1') || normalized.includes('o3')) return 'openai'
  if (normalized.includes('gemini')) return 'gemini'
  if (normalized.includes('claude') || normalized.includes('anthropic')) return 'anthropic'
  if (normalized.includes('grok') || normalized.includes('xai')) return 'xai'
  if (normalized.includes('llama') || normalized.includes('meta')) return 'meta'
  if (normalized.includes('mistral')) return 'mistral'
  return 'other'
}

function formatSmallPercent(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  if (value <= 0) return '0.0%'
  if (value < 0.01) return `${value.toFixed(3)}%`
  return `${value.toFixed(1)}%`
}

function formatSmallCurrency(value: number): string {
  if (value < 0.01) {
    return `$${value.toFixed(6).replace(/0+$/, '').replace(/\.$/, '')}`
  }
  return `$${value.toFixed(2)}`
}


function getSuccessRateClass(successRate: number | null): string {
  if (successRate == null || !Number.isFinite(successRate)) return ''
  const pct = successRate * 100
  if (pct >= 98) return 'text-green-700'
  if (pct >= 90) return 'text-orange-700'
  return 'text-red-700'
}

function formatCurrencyFloorCents(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  const sign = value < 0 ? -1 : 1
  const floored = Math.floor(Math.abs(value) * 100) / 100
  return formatCurrency(sign * floored)
}

function formatAvgCostUsd(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value) || value <= 0) return '$0.000000'
  let decimals = 2
  if (value < 0.001) decimals = 6
  else if (value < 0.01) decimals = 4
  else decimals = 2
  return `$${value.toFixed(decimals)}`
}

function formatLatencyMs(ms: number | null | undefined): string {
  const v = Number(ms)
  if (!Number.isFinite(v) || v < 0) return '-'
  if (v < 1000) return `${Math.round(v)} ms`
  return `${(v / 1000).toFixed(1)} s`
}

async function fetchUsageSummaries(month: string): Promise<UsageSummary[]> {
  const resp = await fetch(`/api/finops/usage-summaries?month=${encodeURIComponent(month)}`, {
    credentials: 'include',
    cache: 'no-store',
  })
  if (!resp.ok) {
    await assertFinopsUnauthorized(resp)
    const err = await resp.json().catch(() => ({ error: 'Failed to fetch usage summaries' }))
    const msg = typeof err.error === 'string' ? err.error : 'Failed to fetch usage summaries'
    const e = new Error(msg) as Error & { status?: number }
    e.status = resp.status
    throw e
  }
  const data = await resp.json()
  return data.data || []
}

/** Normalized row from GET /admin/budgets/overview (field names vary by gateway version). */
type DashboardBudgetOverviewRow = {
  tenant_id: string
  current_spend: number
  budget_limit: number
  utilization_pct: number | null
  status: string
  enforcement_mode: string | null
  warn_pct: number | null
  hard_pct: number | null
  remaining_usd?: number | null
}

function normalizeBudgetOverviewPayload(raw: unknown): DashboardBudgetOverviewRow[] {
  if (!raw || typeof raw !== 'object') return []
  const o = raw as Record<string, unknown>
  const arr = Array.isArray(o.data)
    ? o.data
    : Array.isArray(o.tenants)
      ? o.tenants
      : Array.isArray(o.rows)
        ? o.rows
        : []
  return arr
    .map((item): DashboardBudgetOverviewRow | null => {
      const r = item as Record<string, unknown>
      const tenantId = String(r.tenant_id ?? r.tenantId ?? '').trim()
      if (!tenantId) return null
      const currentSpend = Number(
        r.current_spend ?? r.current_spend_usd ?? r.spend_usd ?? r.spend ?? 0
      )
      const budgetLimit = Number(
        r.budget_limit ?? r.budget_usd ?? r.monthly_usd ?? r.limit_usd ?? 0
      )
      const utilizationPct =
        r.utilization_pct != null && Number.isFinite(Number(r.utilization_pct))
          ? Number(r.utilization_pct)
          : budgetLimit > 0
            ? (currentSpend / budgetLimit) * 100
            : null
      const statusRaw = String(r.status ?? r.budget_status ?? 'not_configured')
      const enforcementMode =
        r.enforcement_mode != null || r.mode != null
          ? String(r.enforcement_mode ?? r.mode ?? '')
          : null
      return {
        tenant_id: tenantId,
        current_spend: currentSpend,
        budget_limit: budgetLimit,
        utilization_pct: utilizationPct,
        status: statusRaw,
        enforcement_mode: enforcementMode || null,
        warn_pct: r.warn_pct == null ? null : Number(r.warn_pct),
        hard_pct: r.hard_pct == null ? null : Number(r.hard_pct),
        remaining_usd: r.remaining_usd == null ? null : Number(r.remaining_usd),
      }
    })
    .filter((x): x is DashboardBudgetOverviewRow => x !== null)
}

async function fetchBudgetOverview(): Promise<unknown> {
  const resp = await fetch('/api/budgets/overview', { credentials: 'include', cache: 'no-store' })
  if (!resp.ok) {
    await assertFinopsUnauthorized(resp)
    const err = (await resp.json().catch(() => ({}))) as { error?: string }
    const msg = typeof err.error === 'string' ? err.error : 'Failed to fetch budget overview'
    const e = new Error(msg) as Error & { status?: number }
    e.status = resp.status
    throw e
  }
  return resp.json()
}

export default function BudgetsPage() {
  const { toast } = useToast()
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [selectedBudget, setSelectedBudget] = useState<TenantBudget | null>(null)
  const [windowHours, setWindowHours] = useState<WindowHours>(720)
  const [viewMode, setViewMode] = useState<FinOpsViewMode>('dashboard')
  const [selectedMonth, setSelectedMonth] = useState<string>(() => {
    const d = new Date()
    const yyyy = d.getFullYear()
    const mm = String(d.getMonth() + 1).padStart(2, '0')
    return `${yyyy}-${mm}`
  })
  const [explainOpen, setExplainOpen] = useState(false)
  const [budgetExplainOpen, setBudgetExplainOpen] = useState(false)
  const [budgetExplainMetric, setBudgetExplainMetric] = useState<'projected_spend' | 'in_warning' | 'active_overrides' | 'projected_overruns'>('projected_spend')
  const [analyticsTenantFilter, setAnalyticsTenantFilter] = useState<string>('all')
  const [analyticsModelFilter, setAnalyticsModelFilter] = useState<string>('all')
  const [analyticsProviderFilter, setAnalyticsProviderFilter] = useState<string>('all')
  const [analyticsStatusFilter, setAnalyticsStatusFilter] = useState<string>('all')
  const [analyticsAnomalyFilter, setAnalyticsAnomalyFilter] = useState<string>('all')
  const [selectedTenantRow, setSelectedTenantRow] = useState<TenantAnalyticsRow | null>(null)
  const [tenantDrilldownOpen, setTenantDrilldownOpen] = useState(false)

  const budgetsQuery = useTenantBudgets()
  // React Query keeps previous data on error — never use stale budgets when the query failed
  const budgets = budgetsQuery.isError ? [] : (budgetsQuery.data ?? [])
  const isLoading = budgetsQuery.isLoading
  const error = budgetsQuery.isError ? budgetsQuery.error : null

  const usageSummariesQuery = useQuery({
    queryKey: ['finops', 'usage-summaries', selectedMonth],
    queryFn: () => fetchUsageSummaries(selectedMonth),
  })
  const usageSummaries = usageSummariesQuery.isError ? [] : (usageSummariesQuery.data ?? [])
  const isLoadingUsage = usageSummariesQuery.isLoading
  const usageSummariesError = usageSummariesQuery.isError ? usageSummariesQuery.error : null
  const { data: modelPerf = [], isLoading: isLoadingPerf } = useModelPerformance(24)
  
  // Anomaly types
  type AnomalyRow = {
    anomaly_id?: string
    timestamp: string
    tenant_id: string
    model: string
    provider: string
    expected_cost_usd: number
    observed_cost_usd: number
    deviation_pct: number
    anomaly_type: string
    status: string
  }
  
  // Anomaly detection state
  const [anomalyTenant, setAnomalyTenant] = useState<string>('all')
  const [anomalyModel, setAnomalyModel] = useState<string>('all')
  const [anomalyProvider, setAnomalyProvider] = useState<string>('all')
  const [anomalyStatus, setAnomalyStatus] = useState<string>('all')
  const [anomalyPage, setAnomalyPage] = useState(0)
  const [anomalyLimit] = useState(50)
  const [selectedAnomaly, setSelectedAnomaly] = useState<AnomalyRow | null>(null)
  const [anomalyDrawerOpen, setAnomalyDrawerOpen] = useState(false)
  const { data: anomalyExplainData, isLoading: isLoadingExplain } = useQuery({
    queryKey: ['anomaly-explain', 30],
    queryFn: () => fetchAnomalyExplain(30),
    enabled: anomalyDrawerOpen,
  })
  // API keys usage state
  const [apiKeyTenant, setApiKeyTenant] = useState<string>('all')
  const [apiKeyProvider, setApiKeyProvider] = useState<string>('all')
  const [apiKeyModel, setApiKeyModel] = useState<string>('all')
  const [apiKeyStatus, setApiKeyStatus] = useState<string>('all')
  const [apiKeyNameFilter, setApiKeyNameFilter] = useState<string>('')
  const [apiKeyPage, setApiKeyPage] = useState(0)
  const [apiKeyLimit] = useState(50)
  const [selectedApiKey, setSelectedApiKey] = useState<ApiKeyUsageRow | null>(null)
  const [apiKeyDrawerOpen, setApiKeyDrawerOpen] = useState(false)
  const [monetizationTab, setMonetizationTab] = useState<'api_keys' | 'jwt_subs'>('api_keys')
  const [monetizationApiKeyPage, setMonetizationApiKeyPage] = useState(0)
  const [monetizationJwtPage, setMonetizationJwtPage] = useState(0)
  const [monetizationLimit] = useState(50)
  const [selectedMonetizationApiKey, setSelectedMonetizationApiKey] = useState<ApiKeyMonetizationRow | null>(null)
  const [monetizationApiKeyDrawerOpen, setMonetizationApiKeyDrawerOpen] = useState(false)
  const [isDownloadingBilling, setIsDownloadingBilling] = useState(false)
  const [jwtSubTenant, setJwtSubTenant] = useState<string>('all')
  const [jwtSubProvider, setJwtSubProvider] = useState<string>('all')
  const [jwtSubModel, setJwtSubModel] = useState<string>('all')
  const [jwtSubStatus, setJwtSubStatus] = useState<string>('all')
  const [jwtSubFilter, setJwtSubFilter] = useState<string>('')
  const [jwtSubPage, setJwtSubPage] = useState(0)
  const [jwtSubLimit] = useState(50)
  const [selectedJwtSub, setSelectedJwtSub] = useState<JwtSubUsageRow | null>(null)
  const [jwtSubDrawerOpen, setJwtSubDrawerOpen] = useState(false)
  const now = new Date()
  const defaultRawTo = new Date(now.getTime() - now.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
  const defaultRawFrom = new Date(now.getTime() - (24 * 60 * 60 * 1000) - now.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
  const [rawFrom, setRawFrom] = useState<string>(defaultRawFrom)
  const [rawTo, setRawTo] = useState<string>(defaultRawTo)
  const [rawTenant, setRawTenant] = useState<string>('all')
  const [rawApiKeyName, setRawApiKeyName] = useState<string>('all')
  const [rawModel, setRawModel] = useState<string>('all')
  const [rawProvider, setRawProvider] = useState<string>('all')
  const [rawStatus, setRawStatus] = useState<string>('all')
  const [rawPage, setRawPage] = useState(0)
  const [rawLimit] = useState(50)
  const [jwtRawFrom, setJwtRawFrom] = useState<string>(defaultRawFrom)
  const [jwtRawTo, setJwtRawTo] = useState<string>(defaultRawTo)
  const [jwtRawTenant, setJwtRawTenant] = useState<string>('all')
  const [jwtRawSub, setJwtRawSub] = useState<string>('all')
  const [jwtRawModel, setJwtRawModel] = useState<string>('all')
  const [jwtRawProvider, setJwtRawProvider] = useState<string>('all')
  const [jwtRawStatus, setJwtRawStatus] = useState<string>('all')
  const [jwtRawPage, setJwtRawPage] = useState(0)
  const [jwtRawLimit] = useState(50)
  type AnomalyListResponse = {
    data: AnomalyRow[]
    object: string
    pagination: {
      limit: number
      offset: number
      returned: number
      total: number
    }
  }
  type AnomalyStats = {
    summary: { active_anomalies: number; cost_spike_24h_usd: number; affected_tenants: number; affected_models: number }
    timeline: { bucket: string; anomalies: number; observed_cost_usd?: number }[]
    top_tenants: { tenant_id: string; anomalies: number }[]
    deviation_histogram: { range: string; count: number }[]
    object: string
    window_hours: number
  }
  type AnomalyExplainDriver = {
    label: string
    delta_spend: number
  }
  type AnomalyExplainRow = {
    tenant_id: string
    top_drivers: {
      models: AnomalyExplainDriver[]
      providers: AnomalyExplainDriver[]
      api_keys: AnomalyExplainDriver[]
    }
  }
  type AnomalyExplainResponse = {
    data: AnomalyExplainRow[]
    object: string
    window_days: number
  }
  async function fetchAnomalyExplain(windowDays: number): Promise<AnomalyExplainResponse> {
    const qs = new URLSearchParams()
    qs.set('window_days', String(windowDays))
    const resp = await fetch(`/api/finops/anomalies/explain?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('Anomaly explain endpoint not available:', resp.status)
      return { data: [], object: 'anomaly_explain', window_days: windowDays }
    }
    const raw = await resp.json()
    return {
      data: Array.isArray(raw?.data) ? raw.data : [],
      object: typeof raw?.object === 'string' ? raw.object : 'anomaly_explain',
      window_days: Number(raw?.window_days ?? windowDays),
    }
  }
  async function fetchAnomalies(params: { windowHours: WindowHours; tenant?: string; model?: string; provider?: string; status?: string; limit?: number; offset?: number }): Promise<AnomalyListResponse> {
    const qs = new URLSearchParams()
    qs.set('window_hours', String(params.windowHours))
    if (params.tenant && params.tenant !== 'all') qs.set('tenant_id', params.tenant)
    if (params.model && params.model !== 'all') qs.set('model', params.model)
    if (params.provider && params.provider !== 'all') qs.set('provider', params.provider)
    if (params.status && params.status !== 'all') qs.set('status', params.status)
    qs.set('limit', String(params.limit ?? 50))
    qs.set('offset', String(params.offset ?? 0))
    const resp = await fetch(`/api/finops/anomalies?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('Anomaly endpoint not available:', resp.status)
      return { data: [], object: 'list', pagination: { limit: params.limit ?? 50, offset: params.offset ?? 0, returned: 0, total: 0 } }
    }
    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const data = Array.isArray(payload?.data) ? payload.data : []
    const pagination = payload?.pagination ?? {}
    return {
      data,
      object: typeof payload?.object === 'string' ? payload.object : 'list',
      pagination: {
        limit: Number(pagination.limit ?? params.limit ?? 50),
        offset: Number(pagination.offset ?? params.offset ?? 0),
        returned: Number(pagination.returned ?? data.length),
        total: Number(pagination.total ?? data.length),
      },
    }
  }
  async function fetchAnomalyStats(params: { windowHours: WindowHours; tenant?: string; model?: string; provider?: string }): Promise<AnomalyStats> {
    const qs = new URLSearchParams()
    qs.set('window_hours', String(params.windowHours))
    if (params.tenant && params.tenant !== 'all') qs.set('tenant_id', params.tenant)
    if (params.model && params.model !== 'all') qs.set('model', params.model)
    if (params.provider && params.provider !== 'all') qs.set('provider', params.provider)
    const resp = await fetch(`/api/finops/anomalies/stats?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('Anomaly stats endpoint not available:', resp.status)
      return {
        summary: { active_anomalies: 0, cost_spike_24h_usd: 0, affected_tenants: 0, affected_models: 0 },
        timeline: [],
        top_tenants: [],
        deviation_histogram: [],
        object: 'anomaly_stats',
        window_hours: params.windowHours,
      }
    }
    const raw = await resp.json()
    const payload = raw?.data ?? raw
    return {
      summary: {
        active_anomalies: Number(payload?.summary?.active_anomalies ?? 0),
        cost_spike_24h_usd: Number(payload?.summary?.cost_spike_24h_usd ?? 0),
        affected_tenants: Number(payload?.summary?.affected_tenants ?? 0),
        affected_models: Number(payload?.summary?.affected_models ?? 0),
      },
      timeline: Array.isArray(payload?.timeline) ? payload.timeline : [],
      top_tenants: Array.isArray(payload?.top_tenants) ? payload.top_tenants : [],
      deviation_histogram: Array.isArray(payload?.deviation_histogram) ? payload.deviation_histogram : [],
      object: typeof payload?.object === 'string' ? payload.object : 'anomaly_stats',
      window_hours: Number(payload?.window_hours ?? params.windowHours),
    }
  }
  async function fetchApiKeysUsage(params: {
    windowHours: WindowHours
    tenant?: string
    provider?: string
    model?: string
    status?: string
    apiKeyName?: string
    limit?: number
    offset?: number
  }): Promise<ApiKeyUsageListResponse> {
    const qs = new URLSearchParams()
    qs.set('window_hours', String(params.windowHours))
    if (params.tenant && params.tenant !== 'all') qs.set('tenant_id', params.tenant)
    if (params.provider && params.provider !== 'all') qs.set('provider', params.provider)
    if (params.model && params.model !== 'all') qs.set('model', params.model)
    if (params.status && params.status !== 'all') qs.set('status', params.status)
    if (params.apiKeyName && params.apiKeyName.trim()) qs.set('api_key_name', params.apiKeyName.trim())
    qs.set('limit', String(params.limit ?? 50))
    qs.set('offset', String(params.offset ?? 0))

    const resp = await fetch(`/api/finops/api-keys/usage?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('API keys usage endpoint not available:', resp.status)
      return { data: [], object: 'list', pagination: { limit: params.limit ?? 50, offset: params.offset ?? 0, returned: 0, total: 0 } }
    }

    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const apiKeysArray = Array.isArray(payload?.data) ? payload.data : []
    const data: ApiKeyUsageRow[] = apiKeysArray.map((row: Record<string, unknown>) => ({
      api_key_id: String(row.api_key_id ?? ''),
      api_key_name: String(row.api_key_name ?? ''),
      tenant_id: String(row.tenant_id ?? ''),
      requests: Number(row.requests ?? 0),
      total_cost_usd: Number(row.spend ?? row.total_cost_usd ?? 0),
      avg_cost_per_request_effective:
        row.avg_cost_per_request_effective == null ? null : Number(row.avg_cost_per_request_effective),
      success_rate: row.success_rate == null ? null : Number(row.success_rate),
      avg_latency_ms: row.avg_latency_ms == null ? null : Number(row.avg_latency_ms),
      top_model: row.top_model == null ? null : String(row.top_model),
      top_provider: row.top_provider == null ? null : String(row.top_provider),
      last_seen_at: row.last_seen == null ? (row.last_seen_at == null ? null : String(row.last_seen_at)) : String(row.last_seen),
    }))
    const summaryRaw = payload?.summary
    const paginationRaw = payload?.pagination
    const totalFromSummary = summaryRaw?.total_active_api_keys
    return {
      data,
      summary: summaryRaw
        ? {
            total_active_api_keys: Number(summaryRaw.total_active_api_keys ?? data.length),
            total_requests: Number(summaryRaw.total_requests ?? data.reduce((sum, row) => sum + row.requests, 0)),
            total_spend: Number(summaryRaw.total_spend ?? data.reduce((sum, row) => sum + row.total_cost_usd, 0)),
            avg_success_rate: summaryRaw.avg_success_rate == null ? null : Number(summaryRaw.avg_success_rate),
            highest_spend_key: summaryRaw.highest_spend_key == null ? null : String(summaryRaw.highest_spend_key),
            most_active_key: summaryRaw.most_active_key == null ? null : String(summaryRaw.most_active_key),
          }
        : undefined,
      object: typeof payload?.object === 'string' ? payload.object : 'list',
      pagination: {
        limit: Number(paginationRaw?.limit ?? params.limit ?? 50),
        offset: Number(paginationRaw?.offset ?? params.offset ?? 0),
        returned: Number(paginationRaw?.returned ?? data.length),
        total: Number(paginationRaw?.total ?? totalFromSummary ?? data.length),
      },
    }
  }

  async function fetchJwtSubsUsage(params: {
    windowHours: WindowHours
    tenant?: string
    provider?: string
    model?: string
    status?: string
    jwtSub?: string
    limit?: number
    offset?: number
  }): Promise<JwtSubUsageListResponse> {
    const qs = new URLSearchParams()
    const now = new Date()
    const fromIso = new Date(now.getTime() - params.windowHours * 60 * 60 * 1000).toISOString()
    const toIso = now.toISOString()
    qs.set('from', fromIso)
    qs.set('to', toIso)
    if (params.tenant && params.tenant !== 'all') qs.set('tenant_id', params.tenant)
    if (params.provider && params.provider !== 'all') qs.set('provider', params.provider)
    if (params.model && params.model !== 'all') qs.set('model', params.model)
    if (params.status && params.status !== 'all') qs.set('status', params.status)
    if (params.jwtSub && params.jwtSub.trim()) qs.set('jwt_sub', params.jwtSub.trim())
    qs.set('sort_by', 'cost_usd')
    qs.set('sort_order', 'desc')
    qs.set('limit', String(params.limit ?? 50))
    qs.set('offset', String(params.offset ?? 0))

    const resp = await fetch(`/api/finops/jwt-subs/usage?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('JWT subs usage endpoint not available:', resp.status)
      return { data: [], object: 'list', pagination: { limit: params.limit ?? 50, offset: params.offset ?? 0, returned: 0, total: 0 } }
    }

    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const subsArray = Array.isArray(payload?.data) ? payload.data : []
    const data: JwtSubUsageRow[] = subsArray.map((row: Record<string, unknown>) => ({
      jwt_sub: String(row.jwt_sub ?? ''),
      tenant_id: String(row.tenant_id ?? ''),
      requests: Number(row.requests ?? 0),
      prompt_tokens: Number(row.prompt_tokens ?? 0),
      completion_tokens: Number(row.completion_tokens ?? 0),
      total_tokens: Number(row.total_tokens ?? 0),
      total_cost_usd: Number(row.total_cost_usd ?? row.cost_usd ?? row.spend ?? 0),
      avg_cost_per_request_effective:
        row.avg_cost_per_request_effective == null ? null : Number(row.avg_cost_per_request_effective),
      first_seen: row.first_seen == null ? null : String(row.first_seen),
      last_seen: row.last_seen == null ? null : String(row.last_seen),
      success_rate: row.success_rate == null ? null : Number(row.success_rate),
      avg_latency_ms: row.avg_latency_ms == null ? null : Number(row.avg_latency_ms),
      top_model: row.top_model == null ? null : String(row.top_model),
      top_provider: row.top_provider == null ? null : String(row.top_provider),
    }))
    const summaryRaw = payload?.summary
    const paginationRaw = payload?.pagination
    const totalFromSummary = summaryRaw?.total_active_jwt_subs
    return {
      data,
      summary: summaryRaw
        ? {
            total_active_jwt_subs: Number(summaryRaw.total_active_jwt_subs ?? data.length),
            total_requests: Number(summaryRaw.total_requests ?? data.reduce((sum, row) => sum + row.requests, 0)),
            total_spend: Number(summaryRaw.total_spend ?? data.reduce((sum, row) => sum + row.total_cost_usd, 0)),
            avg_success_rate: summaryRaw.avg_success_rate == null ? null : Number(summaryRaw.avg_success_rate),
            highest_spend_sub: summaryRaw.highest_spend_sub == null ? null : String(summaryRaw.highest_spend_sub),
            most_active_sub: summaryRaw.most_active_sub == null ? null : String(summaryRaw.most_active_sub),
          }
        : undefined,
      object: typeof payload?.object === 'string' ? payload.object : 'list',
      pagination: {
        limit: Number(paginationRaw?.limit ?? params.limit ?? 50),
        offset: Number(paginationRaw?.offset ?? params.offset ?? 0),
        returned: Number(paginationRaw?.returned ?? data.length),
        total: Number(paginationRaw?.total ?? totalFromSummary ?? data.length),
      },
    }
  }

  async function fetchJwtSubUsageDetail(params: {
    jwtSub: string
    windowHours: WindowHours
    tenant?: string
    groupBy?: 'model' | 'provider' | 'day'
  }): Promise<{ summary?: Record<string, unknown>; breakdown: Record<string, unknown>[] } | null> {
    const qs = new URLSearchParams()
    const now = new Date()
    const fromIso = new Date(now.getTime() - params.windowHours * 60 * 60 * 1000).toISOString()
    const toIso = now.toISOString()
    qs.set('from', fromIso)
    qs.set('to', toIso)
    if (params.tenant && params.tenant !== 'all') qs.set('tenant_id', params.tenant)
    if (params.groupBy) qs.set('group_by', params.groupBy)
    const resp = await fetch(`/api/finops/jwt-subs/${encodeURIComponent(params.jwtSub)}/usage?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('JWT sub drilldown endpoint not available:', resp.status)
      return null
    }
    const raw = await resp.json()
    const payload = raw?.data ?? raw
    return {
      summary: payload?.summary as Record<string, unknown> | undefined,
      breakdown: Array.isArray(payload?.breakdown) ? payload.breakdown : [],
    }
  }

  async function fetchJwtSubDrilldown(jwtSub: string, window: WindowHours, tenant?: string): Promise<JwtSubUsageDrilldown | null> {
    const [byModel, byProvider, byDay] = await Promise.all([
      fetchJwtSubUsageDetail({ jwtSub, windowHours: window, tenant, groupBy: 'model' }),
      fetchJwtSubUsageDetail({ jwtSub, windowHours: window, tenant, groupBy: 'provider' }),
      fetchJwtSubUsageDetail({ jwtSub, windowHours: window, tenant, groupBy: 'day' }),
    ])
    if (!byModel) return null
    const summary = byModel.summary ?? {}
    return {
      jwt_sub: jwtSub,
      summary: {
        requests: Number(summary.requests ?? 0),
        total_cost_usd: Number(summary.total_cost_usd ?? summary.total_spend_usd ?? summary.spend ?? 0),
        prompt_tokens: Number(summary.prompt_tokens ?? 0),
        completion_tokens: Number(summary.completion_tokens ?? 0),
        total_tokens: Number(summary.total_tokens ?? 0),
      },
      requests_by_model: (byModel.breakdown ?? []).map((row) => ({
        model: String(row.group ?? row.model ?? '-'),
        requests: Number(row.requests ?? 0),
        cost_usd: Number(row.total_cost_usd ?? row.cost_usd ?? 0),
        total_tokens: Number(row.total_tokens ?? 0),
      })),
      requests_by_provider: (byProvider?.breakdown ?? []).map((row) => ({
        provider: String(row.group ?? row.provider ?? '-'),
        requests: Number(row.requests ?? 0),
        cost_usd: Number(row.total_cost_usd ?? row.cost_usd ?? 0),
        total_tokens: Number(row.total_tokens ?? 0),
      })),
      traffic_over_time: (byDay?.breakdown ?? []).map((row) => ({
        bucket: String(row.group ?? row.bucket ?? ''),
        requests: Number(row.requests ?? 0),
        total_cost_usd: Number(row.total_cost_usd ?? row.cost_usd ?? 0),
        total_tokens: Number(row.total_tokens ?? 0),
      })),
    }
  }

  async function fetchApiKeyDrilldown(apiKeyId: string, window: WindowHours): Promise<ApiKeyUsageDrilldown | null> {
    const qs = new URLSearchParams()
    qs.set('window_hours', String(window))
    qs.set('limit', '50')
    qs.set('offset', '0')
    const resp = await fetch(`/api/finops/api-keys/${encodeURIComponent(apiKeyId)}/usage?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('API key drilldown endpoint not available:', resp.status)
      return null
    }
    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const requestsByModelRaw = Array.isArray(payload?.requests_by_model) ? payload.requests_by_model : []
    const requestsByProviderRaw = Array.isArray(payload?.requests_by_provider) ? payload.requests_by_provider : []
    const trafficOverTimeRaw = Array.isArray(payload?.traffic_over_time) ? payload.traffic_over_time : []
    const recentRequestsRaw = Array.isArray(payload?.recent_requests) ? payload.recent_requests : []
    const latRaw = (payload?.latency_stats ?? payload?.summary?.latency_stats ?? payload?.latency_percentiles ?? payload?.summary?.latency_percentiles) as Record<string, unknown> | undefined
    const p50 = Number(latRaw?.p50 ?? latRaw?.p50_ms ?? 0)
    const p95 = Number(latRaw?.p95 ?? latRaw?.p95_ms ?? 0)
    const pmax = Number(latRaw?.max ?? latRaw?.p100 ?? latRaw?.max_ms ?? 0)
    return {
      api_key_id: String(payload?.api_key_id ?? apiKeyId),
      api_key_name: String(payload?.api_key_name ?? '-'),
      tenant_id: String(payload?.tenant_id ?? '-'),
      summary: {
        requests: Number(payload?.summary?.requests ?? 0),
        total_cost_usd: Number(payload?.summary?.total_cost_usd ?? payload?.summary?.spend ?? 0),
        avg_cost_per_request_effective:
          payload?.summary?.avg_cost_per_request_effective == null
            ? null
            : Number(payload.summary.avg_cost_per_request_effective),
        success_rate: payload?.summary?.success_rate == null ? null : Number(payload.summary.success_rate),
        avg_latency_ms: payload?.summary?.avg_latency_ms == null ? null : Number(payload.summary.avg_latency_ms),
        top_model: payload?.summary?.top_model == null ? null : String(payload.summary.top_model),
        top_provider: payload?.summary?.top_provider == null ? null : String(payload.summary.top_provider),
        last_seen_at: payload?.summary?.last_seen_at == null
          ? (payload?.summary?.last_seen == null ? null : String(payload.summary.last_seen))
          : String(payload.summary.last_seen_at),
      },
      requests_by_model: requestsByModelRaw.map((row: Record<string, unknown>) => ({
        model: String(row.model ?? '-'),
        requests: Number(row.requests ?? 0),
        cost_usd: Number(row.cost_usd ?? row.spend ?? 0),
      })),
      requests_by_provider: requestsByProviderRaw.map((row: Record<string, unknown>) => ({
        provider: String(row.provider ?? '-'),
        requests: Number(row.requests ?? 0),
      })),
      traffic_over_time: trafficOverTimeRaw.map((row: Record<string, unknown>) => ({
        bucket: String(row.bucket ?? ''),
        requests: Number(row.requests ?? 0),
        errors: Number(row.errors ?? 0),
      })),
      recent_requests: recentRequestsRaw.map((row: Record<string, unknown>) => ({
        request_id: String(row.request_id ?? ''),
        timestamp: String(row.timestamp ?? ''),
        model: String(row.model ?? '-'),
        provider: String(row.provider ?? '-'),
        latency_ms: Number(row.latency_ms ?? 0),
        status: String(row.status ?? '-'),
        fallback_used: Boolean(row.fallback_used),
        cost_usd: Number(row.cost_usd ?? row.spend ?? 0),
        error_type: String(row.error_type ?? ''),
      })),
      latency_percentiles: { p50, p95, max: pmax },
    }
  }
  async function fetchApiKeyRawUsage(params: {
    from?: string
    to?: string
    tenant?: string
    apiKeyName?: string
    model?: string
    provider?: string
    status?: string
    limit?: number
    offset?: number
  }): Promise<ApiKeyRawUsageResponse> {
    const qs = new URLSearchParams()
    const fromIso = params.from ? new Date(params.from).toISOString() : ''
    const toIso = params.to ? new Date(params.to).toISOString() : ''
    if (fromIso) qs.set('from', fromIso)
    if (toIso) qs.set('to', toIso)
    if (params.tenant && params.tenant !== 'all') qs.set('tenant_id', params.tenant)
    if (params.apiKeyName && params.apiKeyName !== 'all') qs.set('api_key_name', params.apiKeyName)
    if (params.model && params.model !== 'all') qs.set('model', params.model)
    if (params.provider && params.provider !== 'all') qs.set('provider', params.provider)
    if (params.status && params.status !== 'all') qs.set('status', params.status)
    qs.set('limit', String(params.limit ?? 50))
    qs.set('offset', String(params.offset ?? 0))

    const resp = await fetch(`/api/finops/api-keys/requests?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      const err = await resp.json().catch(() => ({ error: 'Failed to fetch API key raw usage' }))
      const endpointUnavailable = resp.status === 404 || resp.status === 502
      return {
        data: [],
        object: 'api_key_raw_usage',
        pagination: { limit: params.limit ?? 50, offset: params.offset ?? 0, returned: 0, total: 0 },
        endpoint_available: !endpointUnavailable,
        error_message: String(err?.error ?? 'Failed to fetch API key raw usage'),
      }
    }
    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const rawRows = Array.isArray(payload?.data) ? payload.data : []
    const paginationRaw = payload?.pagination ?? {}
    return {
      data: rawRows.map((row: Record<string, unknown>) => ({
        timestamp: String(row.timestamp ?? ''),
        tenant_id: String(row.tenant_id ?? ''),
        api_key_name: String(row.api_key_name ?? ''),
        api_key_id: String(row.api_key_id ?? ''),
        request_id: String(row.request_id ?? ''),
        model: String(row.model ?? ''),
        provider: String(row.provider ?? ''),
        status: String(row.status ?? ''),
        latency_ms: Number(row.latency_ms ?? 0),
        cost_usd: Number(row.cost_usd ?? 0),
        prompt_tokens: Number(row.prompt_tokens ?? 0),
        cached_tokens: Number(row.cached_tokens ?? 0),
        completion_tokens: Number(row.completion_tokens ?? 0),
        total_tokens: Number(row.total_tokens ?? 0),
      })),
      object: typeof payload?.object === 'string' ? payload.object : 'api_key_raw_usage',
      pagination: {
        limit: Number(paginationRaw.limit ?? params.limit ?? 50),
        offset: Number(paginationRaw.offset ?? params.offset ?? 0),
        returned: Number(paginationRaw.returned ?? rawRows.length),
        total: Number(paginationRaw.total ?? rawRows.length),
      },
      endpoint_available: true,
    }
  }

  async function fetchApiKeysMonetization(params: {
    windowHours: WindowHours
    limit?: number
    offset?: number
  }): Promise<{ data: ApiKeyMonetizationRow[]; pagination: { limit: number; offset: number; returned: number; total: number } } | null> {
    const qs = new URLSearchParams()
    qs.set('window_hours', String(params.windowHours))
    qs.set('limit', String(params.limit ?? 50))
    qs.set('offset', String(params.offset ?? 0))

    const resp = await fetch(`/api/finops/api-keys/usage?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('API keys monetization endpoint not available:', resp.status)
      return null
    }

    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const rows = Array.isArray(payload?.data) ? payload.data : []
    const paginationRaw = payload?.pagination ?? {}
    return {
      data: rows.map((row: Record<string, unknown>) => ({
        api_key_id: String(row.api_key_id ?? ''),
        api_key_name: String(row.api_key_name ?? ''),
        tenant_id: String(row.tenant_id ?? ''),
        requests: Number(row.requests ?? 0),
        spend: Number(row.spend ?? row.total_cost_usd ?? 0),
        avg_cost_per_request_effective: row.avg_cost_per_request_effective == null ? null : Number(row.avg_cost_per_request_effective),
        avg_price_per_request: row.avg_price_per_request == null ? null : Number(row.avg_price_per_request),
        total_price: row.total_price == null ? null : Number(row.total_price),
        margin: row.margin == null ? null : Number(row.margin),
        margin_pct: row.margin_pct == null ? null : Number(row.margin_pct),
        avg_latency_ms: row.avg_latency_ms == null ? null : Number(row.avg_latency_ms),
        success_rate: row.success_rate == null ? null : Number(row.success_rate),
        top_model: row.top_model == null ? null : String(row.top_model),
        last_seen: row.last_seen == null ? (row.last_seen_at == null ? null : String(row.last_seen_at)) : String(row.last_seen),
      })),
      pagination: {
        limit: Number(paginationRaw.limit ?? params.limit ?? 50),
        offset: Number(paginationRaw.offset ?? params.offset ?? 0),
        returned: Number(paginationRaw.returned ?? rows.length),
        total: Number(paginationRaw.total ?? rows.length),
      },
    }
  }

  async function fetchApiKeyMonetizationDrilldown(apiKeyId: string, window: WindowHours): Promise<ApiKeyMonetizationDetail | null> {
    const qs = new URLSearchParams()
    qs.set('window_hours', String(window))
    qs.set('limit', '50')
    qs.set('offset', '0')
    const resp = await fetch(`/api/finops/api-keys/${encodeURIComponent(apiKeyId)}/usage?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('API key monetization drilldown endpoint not available:', resp.status)
      return null
    }
    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const requestsByModelRaw = Array.isArray(payload?.requests_by_model) ? payload.requests_by_model : []
    const recentRequestsRaw = Array.isArray(payload?.recent_requests) ? payload.recent_requests : []
    return {
      api_key_id: String(payload?.api_key_id ?? apiKeyId),
      api_key_name: String(payload?.api_key_name ?? '-'),
      tenant_id: String(payload?.tenant_id ?? '-'),
      summary: {
        requests: Number(payload?.summary?.requests ?? 0),
        spend: Number(payload?.summary?.spend ?? payload?.summary?.total_cost_usd ?? 0),
        avg_cost_per_request_effective:
          payload?.summary?.avg_cost_per_request_effective == null ? null : Number(payload.summary.avg_cost_per_request_effective),
        avg_price_per_request:
          payload?.summary?.avg_price_per_request == null ? null : Number(payload.summary.avg_price_per_request),
        total_price: payload?.summary?.total_price == null ? null : Number(payload.summary.total_price),
        margin: payload?.summary?.margin == null ? null : Number(payload.summary.margin),
        margin_pct: payload?.summary?.margin_pct == null ? null : Number(payload.summary.margin_pct),
        avg_latency_ms: payload?.summary?.avg_latency_ms == null ? null : Number(payload.summary.avg_latency_ms),
        success_rate: payload?.summary?.success_rate == null ? null : Number(payload.summary.success_rate),
        last_seen: payload?.summary?.last_seen == null ? (payload?.summary?.last_seen_at == null ? null : String(payload.summary.last_seen_at)) : String(payload.summary.last_seen),
        top_model: payload?.summary?.top_model == null ? null : String(payload.summary.top_model),
        top_provider: payload?.summary?.top_provider == null ? null : String(payload.summary.top_provider),
      },
      requests_by_model: requestsByModelRaw.map((row: Record<string, unknown>) => {
        const tokenSpend = Number(row.spend ?? row.cost_usd ?? 0)
        const effRaw = row.effective_spend
        const effective_spend =
          effRaw != null && Number.isFinite(Number(effRaw)) ? Number(effRaw) : tokenSpend
        return {
          model: String(row.model ?? '-'),
          requests: Number(row.requests ?? 0),
          spend: tokenSpend,
          effective_spend,
          avg_cost_per_request_effective:
            row.avg_cost_per_request_effective == null ? null : Number(row.avg_cost_per_request_effective),
          avg_price_per_request: row.avg_price_per_request == null ? null : Number(row.avg_price_per_request),
          total_price: row.total_price == null ? null : Number(row.total_price),
          margin: row.margin == null ? null : Number(row.margin),
          margin_pct: row.margin_pct == null ? null : Number(row.margin_pct),
        }
      }),
      recent_requests: recentRequestsRaw.map((row: Record<string, unknown>) => ({
        request_id: String(row.request_id ?? ''),
        timestamp: String(row.timestamp ?? ''),
        model: String(row.model ?? '-'),
        provider: String(row.provider ?? '-'),
        status: String(row.status ?? '-'),
        latency_ms: row.latency_ms == null ? null : Number(row.latency_ms),
        cost_usd: Number(row.cost_usd ?? row.spend ?? 0),
      })),
    }
  }

  async function fetchJwtMonetization(params: {
    windowHours: WindowHours
    limit?: number
    offset?: number
  }): Promise<{ data: JwtMonetizationRow[]; pagination: { limit: number; offset: number; returned: number; total: number } } | null> {
    const qs = new URLSearchParams()
    qs.set('window_hours', String(params.windowHours))
    qs.set('limit', String(params.limit ?? 50))
    qs.set('offset', String(params.offset ?? 0))

    const resp = await fetch(`/api/finops/jwt-subs/usage?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      console.warn('JWT subs monetization endpoint not available:', resp.status)
      return null
    }

    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const rows = Array.isArray(payload?.data) ? payload.data : []
    const paginationRaw = payload?.pagination ?? {}
    return {
      data: rows.map((row: Record<string, unknown>) => ({
        jwt_sub: String(row.jwt_sub ?? ''),
        tenant_id: String(row.tenant_id ?? ''),
        requests: Number(row.requests ?? 0),
        total_tokens: Number(row.total_tokens ?? 0),
        total_cost_usd: Number(row.total_cost_usd ?? row.cost_usd ?? row.spend ?? 0),
        avg_cost_per_request_effective: row.avg_cost_per_request_effective == null ? null : Number(row.avg_cost_per_request_effective),
        avg_price_per_request: row.avg_price_per_request == null ? null : Number(row.avg_price_per_request),
        total_price: row.total_price == null ? null : Number(row.total_price),
        margin: row.margin == null ? null : Number(row.margin),
        margin_pct: row.margin_pct == null ? null : Number(row.margin_pct),
        first_seen: row.first_seen == null ? null : String(row.first_seen),
        last_seen: row.last_seen == null ? null : String(row.last_seen),
      })),
      pagination: {
        limit: Number(paginationRaw.limit ?? params.limit ?? 50),
        offset: Number(paginationRaw.offset ?? params.offset ?? 0),
        returned: Number(paginationRaw.returned ?? rows.length),
        total: Number(paginationRaw.total ?? rows.length),
      },
    }
  }
  async function fetchJwtSubRawUsage(params: {
    from?: string
    to?: string
    tenant?: string
    jwtSub?: string
    model?: string
    provider?: string
    status?: string
    limit?: number
    offset?: number
  }): Promise<JwtSubRawUsageResponse> {
    const qs = new URLSearchParams()
    const fromIso = params.from ? new Date(params.from).toISOString() : ''
    const toIso = params.to ? new Date(params.to).toISOString() : ''
    if (fromIso) qs.set('from', fromIso)
    if (toIso) qs.set('to', toIso)
    if (params.tenant && params.tenant !== 'all') qs.set('tenant_id', params.tenant)
    if (params.jwtSub && params.jwtSub !== 'all') qs.set('jwt_sub', params.jwtSub)
    if (params.model && params.model !== 'all') qs.set('model', params.model)
    if (params.provider && params.provider !== 'all') qs.set('provider', params.provider)
    if (params.status && params.status !== 'all') qs.set('status', params.status)
    qs.set('limit', String(params.limit ?? 50))
    qs.set('offset', String(params.offset ?? 0))

    const resp = await fetch(`/api/finops/jwt-subs/requests?${qs.toString()}`)
    if (!resp.ok) {
      await assertFinopsUnauthorized(resp)
      const err = await resp.json().catch(() => ({ error: 'Failed to fetch JWT sub raw usage' }))
      const endpointUnavailable = resp.status === 404 || resp.status === 502
      return {
        data: [],
        object: 'jwt_sub_raw_usage',
        pagination: { limit: params.limit ?? 50, offset: params.offset ?? 0, returned: 0, total: 0 },
        endpoint_available: !endpointUnavailable,
        error_message: String(err?.error ?? 'Failed to fetch JWT sub raw usage'),
      }
    }
    const raw = await resp.json()
    const payload = raw?.data ?? raw
    const rawRows = Array.isArray(payload?.data) ? payload.data : []
    const paginationRaw = payload?.pagination ?? {}
    return {
      data: rawRows.map((row: Record<string, unknown>) => ({
        timestamp: String(row.timestamp ?? ''),
        tenant_id: String(row.tenant_id ?? ''),
        jwt_sub: String(row.jwt_sub ?? ''),
        request_id: String(row.request_id ?? ''),
        model: String(row.model ?? ''),
        provider: String(row.provider ?? ''),
        status: String(row.status ?? ''),
        latency_ms: Number(row.latency_ms ?? 0),
        cost_usd: Number(row.cost_usd ?? 0),
        prompt_tokens: Number(row.prompt_tokens ?? 0),
        cached_tokens: Number(row.cached_tokens ?? 0),
        completion_tokens: Number(row.completion_tokens ?? 0),
        total_tokens: Number(row.total_tokens ?? 0),
      })),
      object: typeof payload?.object === 'string' ? payload.object : 'jwt_sub_raw_usage',
      pagination: {
        limit: Number(paginationRaw.limit ?? params.limit ?? 50),
        offset: Number(paginationRaw.offset ?? params.offset ?? 0),
        returned: Number(paginationRaw.returned ?? rawRows.length),
        total: Number(paginationRaw.total ?? rawRows.length),
      },
      endpoint_available: true,
    }
  }
  const { data: anomalyResponse, isLoading: isLoadingAnoms, error: anomaliesListError } = useQuery({
    queryKey: ['finops', 'anomalies', windowHours, anomalyTenant, anomalyModel, anomalyProvider, anomalyStatus, anomalyPage, anomalyLimit],
    queryFn: () => fetchAnomalies({ windowHours, tenant: anomalyTenant, model: anomalyModel, provider: anomalyProvider, status: anomalyStatus, limit: anomalyLimit, offset: anomalyPage * anomalyLimit }),
    enabled: viewMode === 'anomalies',
  })
  const anomalies = Array.isArray(anomalyResponse?.data) ? anomalyResponse.data : []
  const anomalyPagination = anomalyResponse?.pagination ?? { limit: anomalyLimit, offset: 0, returned: 0, total: 0 }
  const { data: anomalyStats, isLoading: isLoadingAnomStats, error: anomalyStatsError } = useQuery({
    queryKey: ['finops', 'anomalies-stats', windowHours, anomalyTenant, anomalyModel, anomalyProvider],
    queryFn: () => fetchAnomalyStats({ windowHours, tenant: anomalyTenant, model: anomalyModel, provider: anomalyProvider }),
    enabled: viewMode === 'anomalies',
  })
  const { data: apiKeyUsageResponse, isLoading: isLoadingApiKeysUsage, error: apiKeysUsageError } = useQuery({
    queryKey: ['finops', 'api-keys-usage', windowHours, apiKeyTenant, apiKeyProvider, apiKeyModel, apiKeyStatus, apiKeyNameFilter, apiKeyPage, apiKeyLimit],
    queryFn: () => fetchApiKeysUsage({
      windowHours,
      tenant: apiKeyTenant,
      provider: apiKeyProvider,
      model: apiKeyModel,
      status: apiKeyStatus,
      apiKeyName: apiKeyNameFilter,
      limit: apiKeyLimit,
      offset: apiKeyPage * apiKeyLimit,
    }),
    enabled: viewMode === 'api_keys_usage',
  })
  const apiKeyUsageRows = useMemo(
    () => {
      const rows = Array.isArray(apiKeyUsageResponse?.data) ? apiKeyUsageResponse.data : []
      if (!apiKeyNameFilter) return rows
      return rows.filter((row) => String(row.api_key_name ?? '') === apiKeyNameFilter)
    },
    [apiKeyUsageResponse, apiKeyNameFilter]
  )
  const apiKeyNameOptions = useMemo(
    () => Array.from(new Set((apiKeyUsageResponse?.data ?? []).map((row) => String(row.api_key_name ?? '').trim()).filter(Boolean))).sort(),
    [apiKeyUsageResponse]
  )
  const apiKeyUsageSummary = apiKeyUsageResponse?.summary
  const { data: allModels } = useModels()
  const modelTypeMap = useMemo(() => {
    const map = new Map<string, string>()
    for (const m of allModels ?? []) {
      map.set(m.id, m.type ?? '')
    }
    return map
  }, [allModels])
  const apiKeyPagination = apiKeyUsageResponse?.pagination ?? { limit: apiKeyLimit, offset: 0, returned: 0, total: 0 }
  const { data: apiKeyDrilldown, isLoading: isLoadingApiKeyDrilldown, error: apiKeyDrilldownError } = useQuery({
    queryKey: ['finops', 'api-key-drilldown', selectedApiKey?.api_key_id, windowHours],
    queryFn: () => fetchApiKeyDrilldown(selectedApiKey!.api_key_id, windowHours),
    enabled: viewMode === 'api_keys_usage' && apiKeyDrawerOpen && !!selectedApiKey?.api_key_id,
  })
  const dashboardModelAggregationQuery = useQuery<{ rows: { model: string; requests: number; effective_spend: number }[] }>({
    queryKey: ['finops', 'dashboard', 'model-aggregation', windowHours],
    enabled: viewMode === 'dashboard',
    queryFn: async () => {
      const pageSize = 200
      let offset = 0
      const allKeys: ApiKeyMonetizationRow[] = []
      for (;;) {
        const page = await fetchApiKeysMonetization({ windowHours, limit: pageSize, offset })
        if (!page?.data?.length) break
        allKeys.push(...page.data)
        if (page.data.length < pageSize) break
        offset += pageSize
      }
      if (allKeys.length === 0) return { rows: [] }

      const drilldowns = await Promise.all(
        allKeys.map((row) => fetchApiKeyMonetizationDrilldown(row.api_key_id, windowHours).catch(() => null))
      )

      const byModel = new Map<string, { model: string; requests: number; effective_spend: number }>()
      for (const drilldown of drilldowns) {
        if (!drilldown) continue
        for (const row of drilldown.requests_by_model) {
          const key = String(row.model ?? '').trim().toLowerCase()
          if (!key) continue
          const lineEffective = row.effective_spend
          const existing = byModel.get(key)
          byModel.set(key, {
            model: existing?.model || row.model,
            requests: (existing?.requests || 0) + Number(row.requests ?? 0),
            effective_spend: (existing?.effective_spend || 0) + lineEffective,
          })
        }
      }

      return { rows: Array.from(byModel.values()) }
    },
  })
  const { data: monetizationApiKeysResponse, isLoading: isLoadingMonetizationApiKeys, error: monetizationApiKeysError } = useQuery({
    queryKey: ['finops', 'monetization', 'api-keys', windowHours, monetizationApiKeyPage, monetizationLimit],
    queryFn: () => fetchApiKeysMonetization({ windowHours, limit: monetizationLimit, offset: monetizationApiKeyPage * monetizationLimit }),
    enabled: viewMode === 'models_monetization' && monetizationTab === 'api_keys',
  })
  const monetizationApiKeys = monetizationApiKeysResponse?.data ?? []
  const monetizationApiKeysPagination = monetizationApiKeysResponse?.pagination ?? { limit: monetizationLimit, offset: 0, returned: 0, total: 0 }
  const { data: monetizationApiKeyDrilldown, isLoading: isLoadingMonetizationDrilldown, error: monetizationDrilldownError } = useQuery({
    queryKey: ['finops', 'monetization', 'api-key-drilldown', selectedMonetizationApiKey?.api_key_id, windowHours],
    queryFn: () => fetchApiKeyMonetizationDrilldown(selectedMonetizationApiKey!.api_key_id, windowHours),
    enabled: viewMode === 'models_monetization' && monetizationApiKeyDrawerOpen && !!selectedMonetizationApiKey?.api_key_id,
  })
  const dashboardModelRows = useMemo(() => {
    const rows = dashboardModelAggregationQuery.data?.rows ?? []
    if (rows.length === 0) return []
    const metaMap = new Map<string, { provider: string; type: string }>()
    for (const model of allModels ?? []) {
      metaMap.set(model.id.trim().toLowerCase(), {
        provider: model.provider,
        type: model.type || 'llm',
      })
    }
    return rows.map((row) => {
      const meta = metaMap.get(row.model.trim().toLowerCase())
      const avg = row.requests > 0 ? row.effective_spend / row.requests : 0
      return {
        model: row.model,
        provider: meta?.provider,
        model_type: meta?.type,
        requests: row.requests,
        effective_spend: row.effective_spend,
        avg_cost_per_request: avg,
      }
    })
  }, [dashboardModelAggregationQuery.data, allModels])
  const { data: jwtSubUsageResponse, isLoading: isLoadingJwtSubUsage, error: jwtSubUsageError } = useQuery({
    queryKey: ['finops', 'jwt-subs-usage', windowHours, jwtSubTenant, jwtSubProvider, jwtSubModel, jwtSubStatus, jwtSubFilter, jwtSubPage, jwtSubLimit],
    queryFn: () => fetchJwtSubsUsage({
      windowHours,
      tenant: jwtSubTenant,
      provider: jwtSubProvider,
      model: jwtSubModel,
      status: jwtSubStatus,
      jwtSub: jwtSubFilter,
      limit: jwtSubLimit,
      offset: jwtSubPage * jwtSubLimit,
    }),
    enabled: viewMode === 'jwt_sub_usage',
  })
  const { data: monetizationJwtResponse, isLoading: isLoadingMonetizationJwt, error: monetizationJwtError } = useQuery({
    queryKey: ['finops', 'monetization', 'jwt-subs', windowHours, monetizationJwtPage, monetizationLimit],
    queryFn: () => fetchJwtMonetization({ windowHours, limit: monetizationLimit, offset: monetizationJwtPage * monetizationLimit }),
    enabled: viewMode === 'models_monetization' && monetizationTab === 'jwt_subs',
  })
  const monetizationJwtRows = monetizationJwtResponse?.data ?? []
  const monetizationJwtPagination = monetizationJwtResponse?.pagination ?? { limit: monetizationLimit, offset: 0, returned: 0, total: 0 }
  const jwtSubUsageRows = useMemo(
    () => {
      const rows = Array.isArray(jwtSubUsageResponse?.data) ? jwtSubUsageResponse.data : []
      if (!jwtSubFilter) return rows
      return rows.filter((row) => String(row.jwt_sub ?? '') === jwtSubFilter)
    },
    [jwtSubUsageResponse, jwtSubFilter]
  )
  const jwtSubOptions = useMemo(
    () => Array.from(new Set((jwtSubUsageResponse?.data ?? []).map((row) => String(row.jwt_sub ?? '').trim()).filter(Boolean))).sort(),
    [jwtSubUsageResponse]
  )
  const jwtSubUsageSummary = jwtSubUsageResponse?.summary
  const jwtSubPagination = jwtSubUsageResponse?.pagination ?? { limit: jwtSubLimit, offset: 0, returned: 0, total: 0 }
  const { data: jwtSubDrilldown, isLoading: isLoadingJwtSubDrilldown, error: jwtSubDrilldownError } = useQuery({
    queryKey: ['finops', 'jwt-sub-drilldown', selectedJwtSub?.jwt_sub, windowHours, jwtSubTenant],
    queryFn: () => fetchJwtSubDrilldown(selectedJwtSub!.jwt_sub, windowHours, jwtSubTenant),
    enabled: viewMode === 'jwt_sub_usage' && jwtSubDrawerOpen && !!selectedJwtSub?.jwt_sub,
  })
  const { data: apiKeyRawUsageResponse, isLoading: isLoadingApiKeyRawUsage, error: apiKeyRawUsageError } = useQuery({
    queryKey: ['finops', 'api-keys-raw-usage', rawFrom, rawTo, rawTenant, rawApiKeyName, rawModel, rawProvider, rawStatus, rawPage, rawLimit],
    queryFn: () => fetchApiKeyRawUsage({
      from: rawFrom,
      to: rawTo,
      tenant: rawTenant,
      apiKeyName: rawApiKeyName,
      model: rawModel,
      provider: rawProvider,
      status: rawStatus,
      limit: rawLimit,
      offset: rawPage * rawLimit,
    }),
    enabled: viewMode === 'api_keys_raw_usage',
  })
  const apiKeyRawRows = useMemo(
    () => [...(apiKeyRawUsageResponse?.data ?? [])].sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()),
    [apiKeyRawUsageResponse]
  )
  const apiKeyRawPagination = apiKeyRawUsageResponse?.pagination ?? { limit: rawLimit, offset: 0, returned: 0, total: 0 }
  const apiKeyRawNameOptions = useMemo(
    () => Array.from(new Set((apiKeyRawUsageResponse?.data ?? []).map((row) => String(row.api_key_name ?? '').trim()).filter(Boolean))).sort(),
    [apiKeyRawUsageResponse]
  )
  const { data: jwtSubRawUsageResponse, isLoading: isLoadingJwtSubRawUsage, error: jwtSubRawUsageError } = useQuery({
    queryKey: ['finops', 'jwt-subs-raw-usage', jwtRawFrom, jwtRawTo, jwtRawTenant, jwtRawSub, jwtRawModel, jwtRawProvider, jwtRawStatus, jwtRawPage, jwtRawLimit],
    queryFn: () => fetchJwtSubRawUsage({
      from: jwtRawFrom,
      to: jwtRawTo,
      tenant: jwtRawTenant,
      jwtSub: jwtRawSub,
      model: jwtRawModel,
      provider: jwtRawProvider,
      status: jwtRawStatus,
      limit: jwtRawLimit,
      offset: jwtRawPage * jwtRawLimit,
    }),
    enabled: viewMode === 'jwt_sub_raw_usage',
  })
  const jwtSubRawRows = useMemo(
    () => [...(jwtSubRawUsageResponse?.data ?? [])].sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()),
    [jwtSubRawUsageResponse]
  )
  const jwtSubRawPagination = jwtSubRawUsageResponse?.pagination ?? { limit: jwtRawLimit, offset: 0, returned: 0, total: 0 }
  const jwtSubRawOptions = useMemo(
    () => Array.from(new Set((jwtSubRawUsageResponse?.data ?? []).map((row) => String(row.jwt_sub ?? '').trim()).filter(Boolean))).sort(),
    [jwtSubRawUsageResponse]
  )

  /** Union of tenants from usage + budgets so we probe /tenant-request-stats for budget-only tenants too */
  const tenantIdsForRequestStats = useMemo(() => {
    const s = new Set<string>()
    for (const u of usageSummaries) s.add(u.tenant_id)
    for (const b of budgets) s.add(b.tenant_id)
    return Array.from(s).sort()
  }, [usageSummaries, budgets])

  const tenantStatsQuery = useQuery({
    queryKey: ['finops', 'tenant-request-stats', selectedMonth, windowHours, tenantIdsForRequestStats.join(',')],
    queryFn: () => fetchTenantRequestStatsBatch(tenantIdsForRequestStats, windowHours),
    enabled: tenantIdsForRequestStats.length > 0,
    gcTime: 0,
  })
  const tenantRequestStats = tenantStatsQuery.isError ? [] : (tenantStatsQuery.data?.rows ?? [])
  const forbiddenTenantIdsFromStats = tenantStatsQuery.isError ? [] : (tenantStatsQuery.data?.forbiddenTenantIds ?? [])
  const isLoadingTenantStats = tenantStatsQuery.isLoading
  const tenantRequestStatsError = tenantStatsQuery.isError ? tenantStatsQuery.error : null

  const forbiddenTenantIdsSet = useMemo(
    () => new Set(forbiddenTenantIdsFromStats),
    [forbiddenTenantIdsFromStats]
  )
  const budgetsVisible = useMemo(
    () => budgets.filter((b) => !forbiddenTenantIdsSet.has(b.tenant_id)),
    [budgets, forbiddenTenantIdsSet]
  )
  const usageSummariesVisible = useMemo(
    () => usageSummaries.filter((u) => !forbiddenTenantIdsSet.has(u.tenant_id)),
    [usageSummaries, forbiddenTenantIdsSet]
  )

  const dashboardBudgetOverviewQuery = useQuery({
    queryKey: ['finops', 'dashboard', 'budget-overview'],
    queryFn: fetchBudgetOverview,
    enabled: viewMode === 'dashboard',
    staleTime: 60_000,
  })
  const dashboardOverviewRowsRaw = useMemo(
    () => normalizeBudgetOverviewPayload(dashboardBudgetOverviewQuery.data),
    [dashboardBudgetOverviewQuery.data]
  )
  const dashboardOverviewRows = useMemo(
    () => dashboardOverviewRowsRaw.filter((r) => !forbiddenTenantIdsSet.has(r.tenant_id)),
    [dashboardOverviewRowsRaw, forbiddenTenantIdsSet]
  )

  // Export Model Cost Efficiency table (including insights) to CSV
  const handleExportModelEfficiency = () => {
    try {
      const exportHeaders = ['model', 'provider', 'model_type', 'requests', 'effective_spend', 'avg_cost_per_request']
      const exportLines = [exportHeaders.join(',')]
      for (const r of dashboardModelRows) {
        const line = [
          r.model,
          r.provider ?? '-',
          r.model_type ?? '-',
          String(r.requests ?? 0),
          String(r.effective_spend ?? 0),
          String(r.avg_cost_per_request ?? 0),
        ]
        exportLines.push(line.map((v) => (typeof v === 'string' && v.includes(',') ? `"${v}"` : v)).join(','))
      }

      const exportCsv = exportLines.join('\n')
      const exportBlob = new Blob([exportCsv], { type: 'text/csv;charset=utf-8;' })
      const exportUrl = URL.createObjectURL(exportBlob)
      const exportLink = document.createElement('a')
      exportLink.href = exportUrl
      exportLink.download = `model_cost_efficiency_${selectedMonth}.csv`
      document.body.appendChild(exportLink)
      exportLink.click()
      document.body.removeChild(exportLink)
      URL.revokeObjectURL(exportUrl)
    } catch (e) {
      console.error('Failed to export Model Cost Efficiency CSV', e)
    }
  }

  const resolveBillingFilename = (contentDisposition: string | null): string | null => {
    if (!contentDisposition) return null
    const utf8Match = contentDisposition.match(/filename\*\s*=\s*UTF-8''([^;]+)/i)
    if (utf8Match?.[1]) {
      try {
        return decodeURIComponent(utf8Match[1].trim())
      } catch {
        return utf8Match[1].trim()
      }
    }
    const asciiMatch = contentDisposition.match(/filename\s*=\s*"?([^";]+)"?/i)
    return asciiMatch?.[1]?.trim() ?? null
  }

  const handleDownloadBillingReport = async () => {
    setIsDownloadingBilling(true)
    try {
      const resp = await fetch(`/api/finops/billing/report?window_hours=${encodeURIComponent(String(windowHours))}`, {
        credentials: 'include',
      })
      if (!resp.ok) {
        await assertFinopsUnauthorized(resp)
        throw new Error('Failed to download billing report.')
      }
      const blob = await resp.blob()
      const filename = resolveBillingFilename(resp.headers.get('content-disposition')) ?? 'billing_report.csv'
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = filename
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(url)
    } catch (err) {
      console.error('Failed to download billing report', err)
      toast({
        title: 'Error',
        description: 'Failed to download billing report.',
        variant: 'destructive',
      })
    } finally {
      setIsDownloadingBilling(false)
    }
  }

  // Calculate projected spend based on window
  const calculateProjectedSpend = useMemo(() => {
    return (currentSpend: number): number => {
      // Simple projection: assume current spend is proportional to elapsed time
      // Adjust elapsed fraction based on window size
      // For demo: 24h = 0.1, 7d = 0.3, 30d = 0.5
      const elapsedFractionMap: Record<WindowHours, number> = {
        24: 0.1,
        168: 0.3,
        720: 0.5,
      }
      const elapsedFraction = elapsedFractionMap[windowHours]
      return elapsedFraction > 0 ? currentSpend / elapsedFraction : currentSpend
    }
  }, [windowHours])

  const handleEdit = (budget: TenantBudget) => {
    setSelectedBudget(budget)
    setEditDialogOpen(true)
  }

  const configuredBudgets: TenantBudget[] = budgetsVisible.filter(
    (b): b is TenantBudget => b.status !== 'not_configured'
  )
  const totalBudgets = configuredBudgets.length
  const totalSpend = configuredBudgets.reduce((sum, b) => sum + b.current_spend_usd, 0)
  const totalReserved = configuredBudgets.reduce((sum, b) => sum + (b.reserved_usd ?? 0), 0)
  const totalRemaining = configuredBudgets.reduce((sum, b) => {
    const hasOverride = b.override_limit_usd !== null && b.override_limit_usd !== undefined
    const effectiveLimit = hasOverride ? (b.override_limit_usd ?? b.monthly_usd ?? 0) : (b.monthly_usd ?? 0)
    const effectiveSpend = b.effective_spend_usd ?? b.current_spend_usd
    return sum + (effectiveLimit - effectiveSpend)
  }, 0)
  const budgetsExceeded = configuredBudgets.filter((b) => b.status === 'exceeded').length
  const budgetsInWarning = configuredBudgets.filter((b) => b.status === 'warning').length
  const activeOverrides = budgetsVisible.filter(
    (b) => b.enforcement_paused === true || (b.override_limit_usd !== null && b.override_limit_usd !== undefined)
  ).length
  
  // Calculate projected overruns
  const projectedOverruns = useMemo(() => {
    return configuredBudgets.filter((b) => {
      const hasOverride = b.override_limit_usd !== null && b.override_limit_usd !== undefined
      const effectiveLimit = hasOverride ? (b.override_limit_usd ?? b.monthly_usd ?? 0) : (b.monthly_usd ?? 0)
      const projected = calculateProjectedSpend(b.current_spend_usd)
      return projected >= effectiveLimit
    }).length
  }, [configuredBudgets, calculateProjectedSpend])

  type FinopsDashboardRiskRow = {
    tenant_id: string
    current_spend_usd: number
    budget_usd: number
    utilization_pct: number
    status: string
    enforcement_mode: string | null
    warn_pct: number | null
    hard_pct: number | null
  }

  const finopsDashboardKpis = useMemo(() => {
    const overviewOk =
      dashboardBudgetOverviewQuery.isSuccess && dashboardOverviewRows.length > 0

    if (!overviewOk) {
      const totalSpendMonth = budgetsVisible.reduce((sum, b) => sum + Number(b.current_spend_usd ?? 0), 0)
      const totalBudgetUsd = configuredBudgets.reduce((sum, b) => {
        const hasOverride = b.override_limit_usd !== null && b.override_limit_usd !== undefined
        const effectiveLimit = hasOverride ? (b.override_limit_usd ?? b.monthly_usd ?? 0) : (b.monthly_usd ?? 0)
        return sum + effectiveLimit
      }, 0)
      const utilizationPct = totalBudgetUsd > 0 ? (totalSpendMonth / totalBudgetUsd) * 100 : 0
      const tenantsAtRisk = configuredBudgets.filter((b) => b.status === 'warning').length
      const tenantsOver = configuredBudgets.filter((b) => b.status === 'exceeded').length
      const spendByTenantRows = budgetsVisible.map((b) => ({
        tenant_id: b.tenant_id,
        spend: Number(b.current_spend_usd ?? 0),
      }))
      const riskRows: FinopsDashboardRiskRow[] = configuredBudgets.map((b) => {
        const budgetUsd = b.current_spend_usd + b.remaining_usd
        const utilization_pct = budgetUsd > 0 ? (b.current_spend_usd / budgetUsd) * 100 : 0
        return {
          tenant_id: b.tenant_id,
          current_spend_usd: b.current_spend_usd,
          budget_usd: budgetUsd,
          utilization_pct,
          status: b.status,
          enforcement_mode: b.enforcement_mode,
          warn_pct: b.warn_pct,
          hard_pct: b.hard_pct,
        }
      })
      return {
        totalSpendMonth,
        totalBudgetUsd,
        utilizationPct,
        tenantsAtRisk,
        tenantsOver,
        spendByTenantRows,
        riskRows,
      }
    }

    const totalSpendMonth = dashboardOverviewRows.reduce((s, r) => s + r.current_spend, 0)
    const totalBudgetUsd = dashboardOverviewRows.reduce((s, r) => s + Math.max(0, r.budget_limit), 0)
    const utilizationPct = totalBudgetUsd > 0 ? (totalSpendMonth / totalBudgetUsd) * 100 : 0
    const norm = (s: string) => s.toLowerCase()
    const tenantsAtRisk = dashboardOverviewRows.filter((r) => norm(r.status) === 'warning').length
    const tenantsOver = dashboardOverviewRows.filter((r) => norm(r.status) === 'exceeded').length
    const spendByTenantRows = dashboardOverviewRows.map((r) => ({
      tenant_id: r.tenant_id,
      spend: r.current_spend,
    }))
    const riskRows: FinopsDashboardRiskRow[] = dashboardOverviewRows.map((r) => {
      const budgetUsd =
        r.budget_limit > 0
          ? r.budget_limit
          : r.remaining_usd != null
            ? r.current_spend + r.remaining_usd
            : r.current_spend
      const utilization_pct =
        r.utilization_pct != null && Number.isFinite(r.utilization_pct)
          ? r.utilization_pct
          : budgetUsd > 0
            ? (r.current_spend / budgetUsd) * 100
            : 0
      return {
        tenant_id: r.tenant_id,
        current_spend_usd: r.current_spend,
        budget_usd: budgetUsd,
        utilization_pct,
        status: r.status,
        enforcement_mode: r.enforcement_mode,
        warn_pct: r.warn_pct,
        hard_pct: r.hard_pct,
      }
    })
    return {
      totalSpendMonth,
      totalBudgetUsd,
      utilizationPct,
      tenantsAtRisk,
      tenantsOver,
      spendByTenantRows,
      riskRows,
    }
  }, [dashboardBudgetOverviewQuery.isSuccess, dashboardOverviewRows, budgetsVisible, configuredBudgets])

  const dashboardViewLoading = isLoading || (viewMode === 'dashboard' && dashboardBudgetOverviewQuery.isLoading)

  const tenantRequestStatsMap = useMemo(() => {
    return new Map(tenantRequestStats.map((r) => [r.tenant_id, r]))
  }, [tenantRequestStats])

  const modelProviderMap = useMemo(() => {
    const map = new Map<string, string>()
    for (const m of modelPerf) {
      const model = String(m.model ?? '').trim().toLowerCase()
      const provider = String(m.provider ?? '').trim()
      if (model && provider && !map.has(model)) {
        map.set(model, provider)
      }
    }
    return map
  }, [modelPerf])

  const analyticsRows = useMemo<TenantAnalyticsRow[]>(() => {
    const usageByTenant = new Map(usageSummariesVisible.map((u) => [u.tenant_id, u]))
    const budgetByTenant = new Map(configuredBudgets.map((b) => [b.tenant_id, b]))
    const tenantIds = Array.from(
      new Set([...Array.from(usageByTenant.keys()), ...Array.from(budgetByTenant.keys())])
    )

    const rows = tenantIds.map((tenantId) => {
      const usage = usageByTenant.get(tenantId)
      const budget = budgetByTenant.get(tenantId)
      const stats = tenantRequestStatsMap.get(tenantId)
      const models = [...(usage?.models ?? [])].sort((a, b) => b.requests - a.requests)
      const topModel = models[0]?.model ?? null
      const hasOverride = budget?.override_limit_usd !== null && budget?.override_limit_usd !== undefined
      const budgetUsd = budget ? (hasOverride ? (budget.override_limit_usd ?? budget.monthly_usd ?? null) : (budget.monthly_usd ?? null)) : null
      const spendUsd = usage?.total_cost_usd ?? budget?.current_spend_usd ?? 0
      const remainingBudgetUsd = budgetUsd != null ? (budgetUsd - spendUsd) : null
      const util = budgetUsd != null && budgetUsd > 0 ? (spendUsd / budgetUsd) * 100 : null
      return {
        tenant_id: tenantId,
        requests: usage?.total_requests ?? 0,
        spend_usd: spendUsd,
        success_rate: stats?.success_rate ?? null,
        avg_latency_ms: stats?.avg_latency_ms ?? null,
        top_model: topModel,
        budget_usd: budgetUsd,
        remaining_budget_usd: remainingBudgetUsd,
        utilization_pct: util,
        status: budget?.status ?? 'healthy',
        models,
      }
    })

    const filtered = rows.filter((row) => {
      if (analyticsTenantFilter !== 'all' && row.tenant_id !== analyticsTenantFilter) return false
      if (analyticsStatusFilter !== 'all' && row.status !== analyticsStatusFilter) return false
      if (analyticsAnomalyFilter === 'with_alerts' && row.status === 'healthy') return false
      if (analyticsAnomalyFilter === 'no_alerts' && row.status !== 'healthy') return false
      if (analyticsModelFilter !== 'all' && !row.models.some((m) => m.model === analyticsModelFilter)) return false
      if (analyticsProviderFilter !== 'all') {
        const hasProviderModel = row.models.some((m) => {
          const provider = resolveProviderFromModel(String(m.model), modelProviderMap)
          return provider === analyticsProviderFilter
        })
        if (!hasProviderModel) return false
      }
      return true
    })

    return filtered.sort((a, b) => b.spend_usd - a.spend_usd)
  }, [
    usageSummariesVisible,
    configuredBudgets,
    tenantRequestStatsMap,
    analyticsTenantFilter,
    analyticsStatusFilter,
    analyticsAnomalyFilter,
    analyticsModelFilter,
    analyticsProviderFilter,
    modelProviderMap,
  ])

  const handleExportTenantAnalytics = () => {
    const generatedAt = new Date().toISOString()
    const headers = [
      'generated_at',
      'window',
      'tenant_id',
      'requests',
      'spend_usd',
      'success_rate',
      'avg_latency_ms',
      'top_model',
      'budget_usd',
      'remaining_budget_usd',
      'utilization_pct',
      'status',
    ]
    const lines = [headers.join(',')]
    for (const row of analyticsRows) {
      const line = [
        generatedAt,
        `${windowHours}h`,
        row.tenant_id,
        String(row.requests),
        String(row.spend_usd),
        row.success_rate == null ? '-' : String(row.success_rate),
        row.avg_latency_ms == null ? '-' : String(row.avg_latency_ms),
        row.top_model ?? '-',
        row.budget_usd == null ? '-' : String(row.budget_usd),
        row.remaining_budget_usd == null ? '-' : String(row.remaining_budget_usd),
        row.utilization_pct == null ? '-' : String(row.utilization_pct),
        row.status,
      ]
      lines.push(line.map((v) => (v.includes(',') ? `"${v}"` : v)).join(','))
    }
    const csv = lines.join('\n')
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `tenant_analytics_${selectedMonth}.csv`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  }

  if (error) {
    const s = error instanceof Error ? (error as Error & { status?: number }).status : undefined
    const isAccessDenied = s === 403 || s === 401
    return (
      <div>
        <PageHeader
          title="FinOps"
          description="Manage spending limits and cost controls"
        />
        <SectionCard title={isAccessDenied ? 'Access limited' : 'Error'}>
          {isAccessDenied ? (
            <EmptyState
              icon={ShieldAlert}
              title="Insufficient permissions"
              description="Your role cannot view this page. This page only shows data when the gateway allows access to this global area."
            />
          ) : (
            <div className="text-center py-8">
              <p className="text-destructive mb-2">Failed to load budgets</p>
              <p className="text-sm text-muted-foreground">{error.message}</p>
            </div>
          )}
        </SectionCard>
      </div>
    )
  }

  if (usageSummariesError) {
    const s =
      usageSummariesError instanceof Error
        ? (usageSummariesError as Error & { status?: number }).status
        : undefined
    const isAccessDenied = s === 403 || s === 401
    return (
      <div>
        <PageHeader
          title="FinOps"
          description="Manage spending limits and cost controls"
        />
        <SectionCard title={isAccessDenied ? 'Access limited' : 'Unable to load usage data'}>
          {isAccessDenied ? (
            <EmptyState
              icon={ShieldAlert}
              title="Insufficient permissions"
              description="Your role cannot view this page. This page only shows data when the gateway allows access to this global area."
            />
          ) : (
            <div className="text-center py-8">
              <p className="text-destructive mb-2">Failed to load usage summaries</p>
              <p className="text-sm text-muted-foreground">
                {usageSummariesError instanceof Error
                  ? usageSummariesError.message
                  : 'Failed to load usage summaries'}
              </p>
            </div>
          )}
        </SectionCard>
      </div>
    )
  }

  /** Tenant request-stats batch reported forbidden tenants — show only restricted state (no FinOps UI below). */
  if (forbiddenTenantIdsFromStats.length > 0 && !isLoadingTenantStats) {
    return (
      <div>
        <PageHeader
          title="FinOps"
          description="Manage spending limits and cost controls"
        />
        <SectionCard title="Access limited">
          <EmptyState
            icon={ShieldAlert}
            title="Insufficient permissions"
            description="Your role cannot view this page. This page only shows data when the gateway allows access to this global area."
          />
        </SectionCard>
      </div>
    )
  }

  return (
    <RequireAdminRole allowedRoles={['admin', 'finance']}>
    <div>
      <PageHeader
        title="FinOps"
        description={
          viewMode === 'budgets'
            ? 'Manage spending limits and cost controls per tenant'
            : viewMode === 'analytics'
            ? 'Tenant-first analytics using real backend endpoints'
            : viewMode === 'anomalies'
            ? 'Detect unusual spend, usage, and provider anomalies'
            : viewMode === 'models_monetization'
            ? 'Monetization and margin tracking for API keys and JWT subs'
            : viewMode === 'api_keys_usage'
            ? 'Per-key traffic, spend, reliability, and latency insights'
            : viewMode === 'jwt_sub_usage'
            ? 'Per-subject traffic, spend, and usage insights'
            : viewMode === 'api_keys_raw_usage'
            ? 'Raw per-request activity attributed to API keys'
            : viewMode === 'jwt_sub_raw_usage'
            ? 'Raw per-request activity attributed to JWT subjects'
            : 'Cost-first dashboard aggregated from real backend endpoints'
        }
      />

      {/* Navigation grid — uniform buttons, 2 rows, full width */}
      {(() => {
        const finopsNav = [
          { label: 'Dashboard', mode: 'dashboard' as FinOpsViewMode, icon: Gauge },
          { label: 'Budgets', mode: 'budgets' as FinOpsViewMode, icon: TableIcon },
          { label: 'Analytics', mode: 'analytics' as FinOpsViewMode, icon: TrendingUp },
          { label: 'Anomaly Detection', mode: 'anomalies' as FinOpsViewMode, icon: AlertTriangle },
          { label: 'Monetization', mode: 'models_monetization' as FinOpsViewMode, icon: DollarSign },
          { label: 'API Keys Usage', mode: 'api_keys_usage' as FinOpsViewMode, icon: KeyRound },
          { label: 'JWT-SUB Usage', mode: 'jwt_sub_usage' as FinOpsViewMode, icon: KeyRound },
          { label: 'API Keys Raw', mode: 'api_keys_raw_usage' as FinOpsViewMode, icon: Database },
          { label: 'JWT-SUB Raw', mode: 'jwt_sub_raw_usage' as FinOpsViewMode, icon: Database },
        ]
        const cols = Math.ceil(finopsNav.length / 2)
        return (
          <div
            className="mt-4 mb-2 grid gap-1.5"
            style={{ gridTemplateColumns: `repeat(${cols}, 1fr)` }}
          >
            {finopsNav.map((item) => (
              <Button
                key={item.label}
                variant={viewMode === item.mode ? 'default' : 'outline'}
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

      {/* Window / period controls row */}
      {(viewMode === 'budgets' || viewMode === 'analytics' || viewMode === 'anomalies' || viewMode === 'api_keys_usage' || viewMode === 'jwt_sub_usage' || viewMode === 'models_monetization') && (
        <div className="mb-6 flex items-center gap-3">
          <span className="text-sm text-muted-foreground">Window:</span>
          <select
            value={windowHours}
            onChange={(e) => setWindowHours(Number(e.target.value) as WindowHours)}
            className="rounded-md border border-input bg-background px-3 py-1.5 text-sm"
          >
            <option value={24}>24h</option>
            <option value={168}>7d</option>
            <option value={720}>30d</option>
          </select>
          {viewMode === 'analytics' && (
            <>
              <span className="text-sm text-muted-foreground">Month:</span>
              <input
                type="month"
                value={selectedMonth}
                onChange={(e) => setSelectedMonth(e.target.value)}
                className="rounded-md border border-input bg-background px-3 py-1.5 text-sm"
              />
            </>
          )}
        </div>
      )}
      {viewMode === 'dashboard' && (
        <div className="mb-6 flex items-center gap-3">
          <span className="text-sm text-muted-foreground">Month:</span>
          <input
            type="month"
            value={selectedMonth}
            onChange={(e) => setSelectedMonth(e.target.value)}
            className="rounded-md border border-input bg-background px-3 py-1.5 text-sm"
          />
        </div>
      )}
      {(viewMode === 'api_keys_raw_usage' || viewMode === 'jwt_sub_raw_usage') && (
        <div className="mb-6" />
      )}
      {viewMode === 'budgets' ? (
        <>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8 mb-6">
        {isLoading ? (
          <>
            <Skeleton className="h-32" />
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
            <StatCard
              title="Config Budgets"
              value={totalBudgets.toString()}
              icon={Wallet}
              description="Tenants with budgets"
            />
            <StatCard
              title="Total Spend"
              value={formatCurrency(totalSpend)}
              icon={DollarSign}
              description="Confirmed spend"
            />
            <StatCard
              title="Reserved"
              value={formatCurrency(totalReserved)}
              icon={DollarSign}
              description="In-flight requests"
            />
            <StatCard
              title="Remaining Budget"
              value={formatCurrency(totalRemaining)}
              icon={TrendingDown}
              description="After confirmed + reserved"
            />
            <StatCard
              title="Budgets Exceeded"
              value={budgetsExceeded.toString()}
              icon={AlertCircle}
              description="Over limit"
            />
            <StatCard
              title="In Warning"
              value={budgetsInWarning.toString()}
              icon={AlertCircle}
              description="Approaching limit"
            />
            <StatCard
              title="Active Overrides"
              value={activeOverrides.toString()}
              icon={AlertCircle}
              description="Paused or overridden"
            />
            <StatCard
              title="Proj Overruns"
              value={projectedOverruns.toString()}
              icon={AlertCircle}
              description="Will exceed"
            />
          </>
        )}
      </div>

      {isLoading ? (
        <div className="grid gap-6 mb-6 md:grid-cols-2">
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
        </div>
      ) : configuredBudgets.length > 0 ? (
        <div className="grid gap-6 mb-6 md:grid-cols-2">
          <BarChartSection
            title="Budget Consumption"
            budgets={configuredBudgets}
            valueAccessor={(b) => {
              const hasOverride = b.override_limit_usd !== null && b.override_limit_usd !== undefined
              const effectiveLimit = hasOverride ? (b.override_limit_usd ?? b.monthly_usd ?? 0) : (b.monthly_usd ?? 0)
              return effectiveLimit > 0 ? (b.current_spend_usd / effectiveLimit) * 100 : 0
            }}
            valueLabel={(v) => formatSmallPercent(v)}
            maxValue={100}
          />
          <BarChartSection
            title="Spend vs Limit"
            budgets={configuredBudgets}
            valueAccessor={(b) => b.current_spend_usd}
            valueLabel={(_v, budget?: TenantBudget) => {
              const hasOverride = budget?.override_limit_usd !== null && budget?.override_limit_usd !== undefined
              const effectiveLimit = hasOverride ? (budget?.override_limit_usd ?? budget?.monthly_usd ?? 0) : (budget?.monthly_usd ?? 0)
              return `${formatCurrency(budget?.current_spend_usd ?? 0)} / ${formatCurrency(effectiveLimit)}`
            }}
          />
        </div>
      ) : null}

      <SectionCard title="Budget Metrics Explain">
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setBudgetExplainMetric('projected_spend')
              setBudgetExplainOpen(true)
            }}
            className="gap-2"
          >
            <HelpCircle className="h-4 w-4" />
            Projected Spend
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setBudgetExplainMetric('in_warning')
              setBudgetExplainOpen(true)
            }}
            className="gap-2"
          >
            <HelpCircle className="h-4 w-4" />
            In Warning
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setBudgetExplainMetric('active_overrides')
              setBudgetExplainOpen(true)
            }}
            className="gap-2"
          >
            <HelpCircle className="h-4 w-4" />
            Active Overrides
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setBudgetExplainMetric('projected_overruns')
              setBudgetExplainOpen(true)
            }}
            className="gap-2"
          >
            <HelpCircle className="h-4 w-4" />
            Projected Overruns
          </Button>
        </div>
      </SectionCard>

      <SectionCard title="Budget Allocations">
        {isLoading ? (
          <div className="space-y-2">
            {[...Array(3)].map((_, i) => (
              <Skeleton key={i} className="h-16 w-full" />
            ))}
          </div>
        ) : budgetsVisible.length === 0 ? (
          <EmptyState
            icon={AlertCircle}
            title={budgets.length > 0 ? 'No accessible tenant budgets' : 'No tenants found'}
            description={
              budgets.length > 0 && forbiddenTenantIdsFromStats.length > 0
                ? 'Some tenants are hidden because request/traffic data is not accessible (access denied).'
                : 'No tenants available to configure budgets'
            }
          />
        ) : (
          <BudgetsTable
            budgets={budgetsVisible}
            onEdit={handleEdit}
            calculateProjectedSpend={calculateProjectedSpend}
            windowHours={windowHours}
          />
        )}
      </SectionCard>

      <EditBudgetDialog
        open={editDialogOpen}
        onOpenChange={setEditDialogOpen}
        budget={selectedBudget}
      />
      <Sheet open={budgetExplainOpen} onOpenChange={setBudgetExplainOpen}>
        <SheetContent className="w-full sm:max-w-xl overflow-y-auto">
          <SheetHeader>
            <SheetTitle>Budget Metric — Explanation</SheetTitle>
            <SheetDescription>
              How this metric is calculated and how to use it
            </SheetDescription>
          </SheetHeader>
          <div className="mt-6 text-sm text-muted-foreground space-y-4">
            {budgetExplainMetric === 'projected_spend' ? (
              <>
                <p className="font-medium text-foreground">Projected Spend</p>
                <p>
                  Projected spend extrapolates the current spending rate over the selected window.
                </p>
                <code className="block bg-muted rounded px-3 py-2 text-xs">
                  projected_spend = current_spend / elapsed_fraction
                </code>
                <p>
                  Example: if part of the window has elapsed and current spend is low, this helps estimate end-of-window spend early.
                </p>
              </>
            ) : null}
            {budgetExplainMetric === 'in_warning' ? (
              <>
                <p className="font-medium text-foreground">In Warning</p>
                <p>
                  Count of tenants currently marked as <strong>warning</strong> in budget status.
                </p>
                <p>
                  These tenants are approaching their configured limit and should be monitored.
                </p>
              </>
            ) : null}
            {budgetExplainMetric === 'active_overrides' ? (
              <>
                <p className="font-medium text-foreground">Active Overrides</p>
                <p>
                  Tenants with enforcement paused or with an active override limit configured.
                </p>
                <p>
                  This can temporarily change effective limits and affect status interpretation.
                </p>
              </>
            ) : null}
            {budgetExplainMetric === 'projected_overruns' ? (
              <>
                <p className="font-medium text-foreground">Projected Overruns</p>
                <p>
                  Count of tenants whose projected spend is greater than or equal to their effective limit.
                </p>
                <code className="block bg-muted rounded px-3 py-2 text-xs">
                  projected_spend &gt;= effective_limit
                </code>
                <p>
                  This is an early warning signal before budget is actually exceeded.
                </p>
              </>
            ) : null}
          </div>
        </SheetContent>
      </Sheet>
        </>
      ) : viewMode === 'analytics' ? (
        <>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-6 mb-6">
            <StatCard title="Total Tenants Active" value={analyticsRows.length.toString()} icon={Wallet} description="In selected month" />
            <StatCard title="Total Requests" value={analyticsRows.reduce((s, r) => s + r.requests, 0).toLocaleString()} icon={TableIcon} description="Aggregated" />
            <StatCard title="Total Spend" value={formatCurrency(analyticsRows.reduce((s, r) => s + r.spend_usd, 0))} icon={DollarSign} description="Aggregated" />
            <StatCard
              title="Avg Success Rate"
              value={
                analyticsRows.filter((r) => r.success_rate != null).length === 0
                  ? '-'
                  : `${(
                      analyticsRows
                        .filter((r) => r.success_rate != null)
                        .reduce((s, r) => s + (r.success_rate ?? 0), 0) /
                      Math.max(analyticsRows.filter((r) => r.success_rate != null).length, 1) *
                      100
                    ).toFixed(1)}%`
              }
              icon={CheckCircle}
              description={isLoadingTenantStats ? 'Loading stats...' : `${windowHours}h window`}
            />
            <StatCard title="Tenants in Warning" value={analyticsRows.filter((r) => r.status === 'warning').length.toString()} icon={AlertCircle} description="Budget status" />
            <StatCard title="Tenants Over Budget" value={analyticsRows.filter((r) => r.status === 'exceeded').length.toString()} icon={AlertCircle} description="Budget status" />
          </div>

          <SectionCard title="Filters">
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-6">
              <select value={analyticsTenantFilter} onChange={(e) => setAnalyticsTenantFilter(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Tenants</option>
                {Array.from(new Set(usageSummariesVisible.map((u) => u.tenant_id))).sort().map((tenantId) => (
                  <option key={tenantId} value={tenantId}>{tenantId}</option>
                ))}
              </select>
              <select value={analyticsProviderFilter} onChange={(e) => setAnalyticsProviderFilter(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Providers</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.provider ?? '')).filter(Boolean))).sort().map((provider) => (
                  <option key={provider} value={provider}>{provider}</option>
                ))}
              </select>
              <select value={analyticsModelFilter} onChange={(e) => setAnalyticsModelFilter(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Models</option>
                {Array.from(new Set(usageSummariesVisible.flatMap((u) => u.models.map((m) => m.model)))).sort().map((model) => (
                  <option key={model} value={model}>{model}</option>
                ))}
              </select>
              <select value={analyticsStatusFilter} onChange={(e) => setAnalyticsStatusFilter(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Statuses</option>
                <option value="healthy">healthy</option>
                <option value="warning">warning</option>
                <option value="exceeded">exceeded</option>
              </select>
              <select value={analyticsAnomalyFilter} onChange={(e) => setAnalyticsAnomalyFilter(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Alert States</option>
                <option value="with_alerts">With Alerts</option>
                <option value="no_alerts">No Alerts</option>
              </select>
              <Button variant="outline" onClick={() => {
                setAnalyticsTenantFilter('all')
                setAnalyticsProviderFilter('all')
                setAnalyticsModelFilter('all')
                setAnalyticsStatusFilter('all')
                setAnalyticsAnomalyFilter('all')
              }}>
                Reset Filters
              </Button>
            </div>
          </SectionCard>

          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 mb-6">
            <SectionCard title="Top Tenants by Requests (month)">
              {isLoadingUsage ? (
                <Skeleton className="h-48" />
              ) : analyticsRows.length === 0 ? (
                <EmptyState title="No data" description="No usage found for the selected month." />
              ) : (
                <div className="space-y-3">
                  {([...analyticsRows]
                    .sort((a, b) => b.requests - a.requests)
                    .slice(0, 5)
                  ).map((u) => {
                    const max = Math.max(...analyticsRows.map((x) => x.requests), 0) || 1
                    const width = Math.max((u.requests / max) * 100, 2)
                    return (
                      <div key={`req-${u.tenant_id}`} className="space-y-1">
                        <div className="flex items-center justify-between text-sm">
                          <span className="font-medium">{u.tenant_id}</span>
                          <span className="text-muted-foreground">{u.requests.toLocaleString()} reqs</span>
                        </div>

                  
                        <div className="h-2 w-full rounded-full bg-muted">
                          <div className="h-2 rounded-full bg-primary" style={{ width: `${width}%` }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </SectionCard>

            <SectionCard title="Top Tenants by Spend (month)">
              {isLoadingUsage ? (
                <Skeleton className="h-48" />
              ) : analyticsRows.length === 0 ? (
                <EmptyState title="No data" description="No spend found for the selected month." />
              ) : (
                <div className="space-y-3">
                  {([...analyticsRows]
                    .sort((a, b) => b.spend_usd - a.spend_usd)
                    .slice(0, 5)
                  ).map((u) => {
                    const max = Math.max(...analyticsRows.map((x) => x.spend_usd), 0) || 1
                    const width = Math.max((u.spend_usd / max) * 100, 2)
                    return (
                      <div key={`spend-${u.tenant_id}`} className="space-y-1">
                        <div className="flex items-center justify-between text-sm">
                          <span className="font-medium">{u.tenant_id}</span>
                          <span className="text-muted-foreground">{formatCurrency(u.spend_usd)}</span>
                        </div>
                        <div className="h-2 w-full rounded-full bg-muted">
                          <div className="h-2 rounded-full bg-primary" style={{ width: `${width}%` }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </SectionCard>

            <SectionCard title="Budget Utilization (current month)">
              {isLoading ? (
                <Skeleton className="h-48" />
              ) : analyticsRows.filter((r) => r.budget_usd != null).length === 0 ? (
                <EmptyState title="No budgets" description="No configured budgets found." />
              ) : (
                <div className="space-y-3">
                  {([...analyticsRows]
                    .filter((r) => r.utilization_pct != null)
                    .sort((a, b) => (b.utilization_pct ?? 0) - (a.utilization_pct ?? 0))
                    .slice(0, 5)
                  ).map((b) => {
                    const util = b.utilization_pct ?? 0
                    return (
                      <div key={`util-${b.tenant_id}`} className="space-y-1">
                        <div className="flex items-center justify-between text-sm">
                          <span className="font-medium">{b.tenant_id}</span>
                          <span className="text-muted-foreground">{util.toFixed(1)}%</span>
                        </div>
                        <div className="h-2 w-full rounded-full bg-muted">
                          <div className="h-2 rounded-full bg-primary" style={{ width: `${util}%` }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </SectionCard>
          </div>

          <div className="grid gap-4 md:grid-cols-2 mb-6">
            <SectionCard title="Success Rate by Tenant">
              {isLoadingTenantStats ? (
                <Skeleton className="h-48" />
              ) : analyticsRows.filter((r) => r.success_rate != null).length === 0 ? (
                <EmptyState title="No success rate data" description="No tenant request stats available in selected window." />
              ) : (
                <div className="space-y-3">
                  {([...analyticsRows]
                    .filter((r) => r.success_rate != null)
                    .sort((a, b) => (b.success_rate ?? 0) - (a.success_rate ?? 0))
                    .slice(0, 10)
                  ).map((row) => {
                    const rate = (row.success_rate ?? 0) * 100
                    return (
                      <div key={`sr-${row.tenant_id}`} className="space-y-1">
                        <div className="flex items-center justify-between text-sm">
                          <span className="font-medium">{row.tenant_id}</span>
                          <span className="text-muted-foreground">{rate.toFixed(1)}%</span>
                        </div>
                        <div className="h-2 w-full rounded-full bg-muted">
                          <div className="h-2 rounded-full bg-primary" style={{ width: `${Math.max(rate, 2)}%` }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </SectionCard>

            <SectionCard title="Budget Risk Map">
              <div className="grid gap-3 grid-cols-3">
                <div className="rounded-md border p-3 bg-green-50 border-green-200">
                  <div className="text-xs text-muted-foreground">healthy</div>
                  <div className="text-xl font-semibold text-green-700">{analyticsRows.filter((r) => r.status === 'healthy').length}</div>
                </div>
                <div className="rounded-md border p-3 bg-yellow-50 border-yellow-200">
                  <div className="text-xs text-muted-foreground">warning</div>
                  <div className="text-xl font-semibold text-yellow-700">{analyticsRows.filter((r) => r.status === 'warning').length}</div>
                </div>
                <div className="rounded-md border p-3 bg-red-50 border-red-200">
                  <div className="text-xs text-muted-foreground">exceeded</div>
                  <div className="text-xl font-semibold text-red-700">{analyticsRows.filter((r) => r.status === 'exceeded').length}</div>
                </div>
              </div>
            </SectionCard>
          </div>

          <SectionCard
            title="Tenant Analytics (month)"
            action={
              <Button variant="outline" size="sm" onClick={handleExportTenantAnalytics} className="gap-2">
                <Download className="h-4 w-4" />
                Export CSV
              </Button>
            }
          >
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-muted-foreground">
                  <tr>
                    <th className="py-2 pr-3">Tenant</th>
                    <th className="py-2 pr-3">Requests</th>
                    <th className="py-2 pr-3">Spend</th>
                    <th className="py-2 pr-3">Success Rate</th>
                    <th className="py-2 pr-3">Avg Latency</th>
                    <th className="py-2 pr-3">Top Model</th>
                    <th className="py-2 pr-3">Budget</th>
                    <th className="py-2 pr-3">Remaining</th>
                    <th className="py-2 pr-3">Utilization</th>
                    <th className="py-2 pr-3">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {analyticsRows.length === 0 ? (
                    <tr className="border-t">
                      <td className="py-6 text-center text-muted-foreground" colSpan={10}>
                        No tenant analytics data for selected filters.
                      </td>
                    </tr>
                  ) : (
                    analyticsRows.map((row) => (
                      <tr
                        key={row.tenant_id}
                        className="border-t cursor-pointer hover:bg-muted/40"
                        onClick={() => {
                          setSelectedTenantRow(row)
                          setTenantDrilldownOpen(true)
                        }}
                      >
                        <td className="py-2 pr-3 font-medium">{row.tenant_id}</td>
                        <td className="py-2 pr-3 tabular-nums">{row.requests.toLocaleString()}</td>
                        <td className="py-2 pr-3 tabular-nums">{formatCurrency(row.spend_usd)}</td>
                        <td className={`py-2 pr-3 tabular-nums ${getSuccessRateClass(row.success_rate)}`}>
                          {row.success_rate == null ? '-' : `${(row.success_rate * 100).toFixed(1)}%`}
                        </td>
                        <td className="py-2 pr-3 tabular-nums">{row.avg_latency_ms == null ? '-' : `${row.avg_latency_ms.toFixed(1)} ms`}</td>
                        <td className="py-2 pr-3">{row.top_model ?? '-'}</td>
                        <td className="py-2 pr-3 tabular-nums">{row.budget_usd == null ? '-' : formatCurrency(row.budget_usd)}</td>
                        <td className="py-2 pr-3 tabular-nums">{row.remaining_budget_usd == null ? '-' : formatCurrencyFloorCents(row.remaining_budget_usd)}</td>
                        <td className="py-2 pr-3 tabular-nums">{formatSmallPercent(row.utilization_pct)}</td>
                        <td className="py-2 pr-3">
                          <Badge variant="outline" className={row.status === 'exceeded' ? 'border-red-300 text-red-700 bg-red-50' : row.status === 'warning' ? 'border-yellow-300 text-yellow-700 bg-yellow-50' : 'border-green-300 text-green-700 bg-green-50'}>
                            {row.status}
                          </Badge>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </SectionCard>

          <Sheet open={tenantDrilldownOpen} onOpenChange={setTenantDrilldownOpen}>
            <SheetContent className="w-full sm:max-w-2xl overflow-y-auto">
              <SheetHeader>
                <SheetTitle>Tenant Drilldown</SheetTitle>
                <SheetDescription>
                  {selectedTenantRow?.tenant_id ?? 'Tenant details'}
                </SheetDescription>
              </SheetHeader>
              {selectedTenantRow ? (
                <div className="mt-6 space-y-6">
                  <div>
                    <h3 className="font-semibold mb-2">Tenant Overview</h3>
                    <div className="grid grid-cols-2 gap-3 text-sm">
                      <div><span className="text-muted-foreground">Requests:</span> {selectedTenantRow.requests.toLocaleString()}</div>
                      <div><span className="text-muted-foreground">Total Cost:</span> {formatCurrency(selectedTenantRow.spend_usd)}</div>
                      <div>
                        <span className="text-muted-foreground">Success:</span>{' '}
                        <span className={getSuccessRateClass(selectedTenantRow.success_rate)}>
                          {selectedTenantRow.success_rate == null ? '-' : `${(selectedTenantRow.success_rate * 100).toFixed(1)}%`}
                        </span>
                      </div>
                      <div><span className="text-muted-foreground">Avg Latency:</span> {selectedTenantRow.avg_latency_ms == null ? '-' : `${selectedTenantRow.avg_latency_ms.toFixed(1)} ms`}</div>
                    </div>
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Budget Details</h3>
                    <div className="grid grid-cols-2 gap-3 text-sm">
                      <div><span className="text-muted-foreground">Budget:</span> {selectedTenantRow.budget_usd == null ? '-' : formatCurrency(selectedTenantRow.budget_usd)}</div>
                      <div><span className="text-muted-foreground">Remaining:</span> {selectedTenantRow.remaining_budget_usd == null ? '-' : formatCurrencyFloorCents(selectedTenantRow.remaining_budget_usd)}</div>
                      <div><span className="text-muted-foreground">Projected:</span> {formatCurrency(calculateProjectedSpend(selectedTenantRow.spend_usd))}</div>
                      <div><span className="text-muted-foreground">Status:</span> {selectedTenantRow.status}</div>
                    </div>
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Requests by Model</h3>
                    <div className="space-y-2">
                      {selectedTenantRow.models.slice(0, 10).map((m) => {
                        const max = Math.max(...selectedTenantRow.models.map((x) => x.requests), 0) || 1
                        const width = Math.max((m.requests / max) * 100, 2)
                        return (
                          <div key={`req-model-${m.model}`} className="space-y-1">
                            <div className="flex justify-between text-sm">
                              <span>{m.model}</span>
                              <span className="text-muted-foreground">{m.requests.toLocaleString()}</span>
                            </div>
                            <div className="h-2 rounded bg-muted">
                              <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Spend by Model</h3>
                    <div className="space-y-2">
                      {[...selectedTenantRow.models].sort((a, b) => b.cost_usd - a.cost_usd).slice(0, 10).map((m) => {
                        const max = Math.max(...selectedTenantRow.models.map((x) => x.cost_usd), 0) || 1
                        const width = Math.max((m.cost_usd / max) * 100, 2)
                        return (
                          <div key={`cost-model-${m.model}`} className="space-y-1">
                            <div className="flex justify-between text-sm">
                              <span>{m.model}</span>
                              <span className="text-muted-foreground">{formatCurrency(m.cost_usd)}</span>
                            </div>
                            <div className="h-2 rounded bg-muted">
                              <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Requests by Provider</h3>
                    <div className="space-y-2">
                      {(() => {
                        const byProvider = new Map<string, number>()
                        for (const modelRow of selectedTenantRow.models) {
                          const provider = resolveProviderFromModel(modelRow.model, modelProviderMap)
                          byProvider.set(provider, (byProvider.get(provider) || 0) + modelRow.requests)
                        }
                        const rows = Array.from(byProvider.entries()).sort((a, b) => b[1] - a[1])
                        const max = Math.max(...rows.map((r) => r[1]), 0) || 1
                        return rows.map(([provider, count]) => {
                          const width = Math.max((count / max) * 100, 2)
                          return (
                            <div key={`provider-${provider}`} className="space-y-1">
                              <div className="flex justify-between text-sm">
                                <span>{provider}</span>
                                <span className="text-muted-foreground">{count.toLocaleString()}</span>
                              </div>
                              <div className="h-2 rounded bg-muted">
                                <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                              </div>
                            </div>
                          )
                        })
                      })()}
                    </div>
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Recent Alerts</h3>
                    <p className="text-sm text-muted-foreground">
                      {selectedTenantRow.status === 'exceeded'
                        ? 'Budget exceeded for this tenant in current period.'
                        : selectedTenantRow.status === 'warning'
                        ? 'Tenant is approaching budget limit.'
                        : 'No active alerts in available analytics sources.'}
                    </p>
                  </div>
                </div>
              ) : null}
            </SheetContent>
          </Sheet>
        </>
      ) : viewMode === 'models_monetization' ? (
        <div className="space-y-6">
          <SectionCard
            title="Models Monetization"
            description="Chargeback view for API keys and JWT subs with pricing and margin."
          >
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant={monetizationTab === 'api_keys' ? 'default' : 'outline'}
                  onClick={() => {
                    setMonetizationTab('api_keys')
                    setMonetizationApiKeyPage(0)
                  }}
                >
                  API Keys
                </Button>
                <Button
                  variant={monetizationTab === 'jwt_subs' ? 'default' : 'outline'}
                  onClick={() => {
                    setMonetizationTab('jwt_subs')
                    setMonetizationJwtPage(0)
                  }}
                >
                  JWT Subs
                </Button>
              </div>
              <Button
                variant="outline"
                size="sm"
                className="ml-auto gap-2"
                onClick={handleDownloadBillingReport}
                disabled={isDownloadingBilling}
              >
                <Download className="h-4 w-4" />
                {isDownloadingBilling ? 'Downloading...' : 'Download Billing CSV'}
              </Button>
            </div>
          </SectionCard>

          {monetizationTab === 'api_keys' ? (
            <SectionCard title="API Keys" description="Monetization summary by API key">
              {isLoadingMonetizationApiKeys ? (
                <Skeleton className="h-64" />
              ) : monetizationApiKeysError ? (
                <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
                  {monetizationApiKeysError instanceof Error ? monetizationApiKeysError.message : 'Failed to load API key monetization'}
                </div>
              ) : monetizationApiKeys.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  No API key monetization data found for the selected period.
                  <div className="text-xs text-muted-foreground">Try expanding the date range.</div>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead className="text-left text-muted-foreground">
                      <tr>
                        <th className="py-2 pr-3">API Key</th>
                        <th className="py-2 pr-3">Tenant</th>
                        <th className="py-2 pr-3">Requests</th>
                        <th className="py-2 pr-3">Spend</th>
                        <th className="py-2 pr-3">Avg Cost / Request</th>
                        <th className="py-2 pr-3">Avg Price / Request</th>
                        <th className="py-2 pr-3">Total Price</th>
                        <th className="py-2 pr-3">Margin</th>
                        <th className="py-2 pr-3">Margin %</th>
                        <th className="py-2 pr-3">Success Rate</th>
                        <th className="py-2 pr-3">Avg Latency</th>
                        <th className="py-2 pr-3">Top Model</th>
                        <th className="py-2 pr-3">Last Seen</th>
                        <th className="py-2 pr-3">Details</th>
                      </tr>
                    </thead>
                    <tbody>
                      {monetizationApiKeys.map((row) => (
                        <tr key={row.api_key_id} className="border-t">
                          <td className="py-2 pr-3 font-medium">{row.api_key_name || row.api_key_id}</td>
                          <td className="py-2 pr-3">{row.tenant_id}</td>
                          <td className="py-2 pr-3 tabular-nums">{row.requests.toLocaleString()}</td>
                          <td className="py-2 pr-3 tabular-nums">{formatCurrency(row.spend)}</td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.avg_cost_per_request_effective == null ? '-' : formatAvgCostUsd(row.avg_cost_per_request_effective)}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.avg_price_per_request == null ? '-' : formatAvgCostUsd(row.avg_price_per_request)}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.total_price == null ? '-' : formatCurrency(row.total_price)}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.margin == null ? '-' : formatCurrency(row.margin)}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.margin_pct == null ? '-' : `${(row.margin_pct * 100).toFixed(2)}%`}
                          </td>
                          <td className={`py-2 pr-3 tabular-nums ${getSuccessRateClass(row.success_rate)}`}>
                            {row.success_rate == null ? '-' : `${(row.success_rate * 100).toFixed(1)}%`}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.avg_latency_ms == null ? '-' : `${row.avg_latency_ms.toFixed(1)} ms`}
                          </td>
                          <td className="py-2 pr-3">{row.top_model ?? '-'}</td>
                          <td className="py-2 pr-3">{row.last_seen ? new Date(row.last_seen).toLocaleString() : '-'}</td>
                          <td className="py-2 pr-3">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => {
                                setSelectedMonetizationApiKey(row)
                                setMonetizationApiKeyDrawerOpen(true)
                              }}
                            >
                              Details
                            </Button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              {monetizationApiKeysPagination.total > 0 && (
                <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
                  <div>
                    Showing {monetizationApiKeysPagination.offset + 1} to {Math.min(monetizationApiKeysPagination.offset + monetizationApiKeysPagination.returned, monetizationApiKeysPagination.total)} of {monetizationApiKeysPagination.total} API keys
                  </div>
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" onClick={() => setMonetizationApiKeyPage(Math.max(0, monetizationApiKeyPage - 1))} disabled={monetizationApiKeyPage === 0}>
                      Previous
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setMonetizationApiKeyPage(monetizationApiKeyPage + 1)}
                      disabled={monetizationApiKeysPagination.offset + monetizationApiKeysPagination.returned >= monetizationApiKeysPagination.total}
                    >
                      Next
                    </Button>
                  </div>
                </div>
              )}
            </SectionCard>
          ) : (
            <SectionCard title="JWT Subs" description="Monetization summary by JWT subject">
              {isLoadingMonetizationJwt ? (
                <Skeleton className="h-64" />
              ) : monetizationJwtError ? (
                <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
                  {monetizationJwtError instanceof Error ? monetizationJwtError.message : 'Failed to load JWT monetization'}
                </div>
              ) : monetizationJwtRows.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  No JWT monetization data found for the selected period.
                  <div className="text-xs text-muted-foreground">Try expanding the date range.</div>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead className="text-left text-muted-foreground">
                      <tr>
                        <th className="py-2 pr-3">JWT Sub</th>
                        <th className="py-2 pr-3">Tenant</th>
                        <th className="py-2 pr-3">Requests</th>
                        <th className="py-2 pr-3">Total Tokens</th>
                        <th className="py-2 pr-3">Spend</th>
                        <th className="py-2 pr-3">Avg Cost / Request</th>
                        <th className="py-2 pr-3">Avg Price / Request</th>
                        <th className="py-2 pr-3">Total Price</th>
                        <th className="py-2 pr-3">Margin</th>
                        <th className="py-2 pr-3">Margin %</th>
                        <th className="py-2 pr-3">First Seen</th>
                        <th className="py-2 pr-3">Last Seen</th>
                      </tr>
                    </thead>
                    <tbody>
                      {monetizationJwtRows.map((row) => (
                        <tr key={row.jwt_sub} className="border-t">
                          <td className="py-2 pr-3 font-medium">{row.jwt_sub || '-'}</td>
                          <td className="py-2 pr-3">{row.tenant_id || '-'}</td>
                          <td className="py-2 pr-3 tabular-nums">{row.requests.toLocaleString()}</td>
                          <td className="py-2 pr-3 tabular-nums">{row.total_tokens.toLocaleString()}</td>
                          <td className="py-2 pr-3 tabular-nums">{formatCurrency(row.total_cost_usd)}</td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.avg_cost_per_request_effective == null ? '-' : formatAvgCostUsd(row.avg_cost_per_request_effective)}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.avg_price_per_request == null ? '-' : formatAvgCostUsd(row.avg_price_per_request)}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.total_price == null ? '-' : formatCurrency(row.total_price)}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.margin == null ? '-' : formatCurrency(row.margin)}
                          </td>
                          <td className="py-2 pr-3 tabular-nums">
                            {row.margin_pct == null ? '-' : `${(row.margin_pct * 100).toFixed(2)}%`}
                          </td>
                          <td className="py-2 pr-3">{row.first_seen ? new Date(row.first_seen).toLocaleString() : '-'}</td>
                          <td className="py-2 pr-3">{row.last_seen ? new Date(row.last_seen).toLocaleString() : '-'}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              {monetizationJwtPagination.total > 0 && (
                <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
                  <div>
                    Showing {monetizationJwtPagination.offset + 1} to {Math.min(monetizationJwtPagination.offset + monetizationJwtPagination.returned, monetizationJwtPagination.total)} of {monetizationJwtPagination.total} JWT subs
                  </div>
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" onClick={() => setMonetizationJwtPage(Math.max(0, monetizationJwtPage - 1))} disabled={monetizationJwtPage === 0}>
                      Previous
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setMonetizationJwtPage(monetizationJwtPage + 1)}
                      disabled={monetizationJwtPagination.offset + monetizationJwtPagination.returned >= monetizationJwtPagination.total}
                    >
                      Next
                    </Button>
                  </div>
                </div>
              )}
            </SectionCard>
          )}

          <Sheet open={monetizationApiKeyDrawerOpen} onOpenChange={setMonetizationApiKeyDrawerOpen}>
            <SheetContent className="w-full sm:max-w-3xl overflow-y-auto">
              <SheetHeader>
                <SheetTitle>API Key Monetization Details</SheetTitle>
                <SheetDescription>
                  {selectedMonetizationApiKey?.api_key_name || selectedMonetizationApiKey?.api_key_id || 'API key details'}
                </SheetDescription>
              </SheetHeader>
              {!selectedMonetizationApiKey ? null : (
                <div className="mt-6 space-y-6">
                  {monetizationDrilldownError ? (
                    <div className="border-t pt-4 text-sm text-destructive">
                      {monetizationDrilldownError instanceof Error ? monetizationDrilldownError.message : 'Failed to load drilldown'}
                    </div>
                  ) : isLoadingMonetizationDrilldown ? (
                    <div className="space-y-3 border-t pt-4">
                      <Skeleton className="h-20" />
                      <Skeleton className="h-20" />
                      <Skeleton className="h-20" />
                    </div>
                  ) : !monetizationApiKeyDrilldown ? (
                    <div className="border-t pt-4 text-sm text-muted-foreground">Detailed monetization data is not available yet for this API key.</div>
                  ) : (
                    <>
                      <div className="grid grid-cols-2 gap-3 text-sm">
                        <div><span className="text-muted-foreground">Requests:</span> {monetizationApiKeyDrilldown.summary.requests.toLocaleString()}</div>
                        <div><span className="text-muted-foreground">Spend:</span> {formatCurrency(monetizationApiKeyDrilldown.summary.spend)}</div>
                        <div><span className="text-muted-foreground">Avg Cost / Request:</span> {monetizationApiKeyDrilldown.summary.avg_cost_per_request_effective == null ? '-' : formatAvgCostUsd(monetizationApiKeyDrilldown.summary.avg_cost_per_request_effective)}</div>
                        <div><span className="text-muted-foreground">Avg Price / Request:</span> {monetizationApiKeyDrilldown.summary.avg_price_per_request == null ? '-' : formatAvgCostUsd(monetizationApiKeyDrilldown.summary.avg_price_per_request)}</div>
                        <div><span className="text-muted-foreground">Total Price:</span> {monetizationApiKeyDrilldown.summary.total_price == null ? '-' : formatCurrency(monetizationApiKeyDrilldown.summary.total_price)}</div>
                        <div><span className="text-muted-foreground">Margin:</span> {monetizationApiKeyDrilldown.summary.margin == null ? '-' : formatCurrency(monetizationApiKeyDrilldown.summary.margin)}</div>
                        <div><span className="text-muted-foreground">Margin %:</span> {monetizationApiKeyDrilldown.summary.margin_pct == null ? '-' : `${(monetizationApiKeyDrilldown.summary.margin_pct * 100).toFixed(2)}%`}</div>
                        <div><span className="text-muted-foreground">Success Rate:</span> {monetizationApiKeyDrilldown.summary.success_rate == null ? '-' : `${(monetizationApiKeyDrilldown.summary.success_rate * 100).toFixed(1)}%`}</div>
                        <div><span className="text-muted-foreground">Avg Latency:</span> {monetizationApiKeyDrilldown.summary.avg_latency_ms == null ? '-' : `${monetizationApiKeyDrilldown.summary.avg_latency_ms.toFixed(1)} ms`}</div>
                        <div><span className="text-muted-foreground">Top Model:</span> {monetizationApiKeyDrilldown.summary.top_model ?? '-'}</div>
                        <div><span className="text-muted-foreground">Last Seen:</span> {monetizationApiKeyDrilldown.summary.last_seen ? new Date(monetizationApiKeyDrilldown.summary.last_seen).toLocaleString() : '-'}</div>
                      </div>

                      <div className="border-t pt-4">
                        <h3 className="font-semibold mb-2">Requests by Model</h3>
                        {(monetizationApiKeyDrilldown.requests_by_model ?? []).length === 0 ? (
                          <p className="text-sm text-muted-foreground">No per-model data available.</p>
                        ) : (
                          <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                              <thead className="text-left text-muted-foreground">
                                <tr>
                                  <th className="py-2 pr-3">Model</th>
                                  <th className="py-2 pr-3">Requests</th>
                                  <th className="py-2 pr-3">Spend</th>
                                  <th className="py-2 pr-3">Avg Price / Request</th>
                                  <th className="py-2 pr-3">Total Price</th>
                                  <th className="py-2 pr-3">Margin</th>
                                  <th className="py-2 pr-3">Margin %</th>
                                </tr>
                              </thead>
                              <tbody>
                                {monetizationApiKeyDrilldown.requests_by_model.map((row) => (
                                  <tr key={row.model} className="border-t">
                                    <td className="py-2 pr-3 font-medium">{row.model}</td>
                                    <td className="py-2 pr-3 tabular-nums">{row.requests.toLocaleString()}</td>
                                    <td className="py-2 pr-3 tabular-nums">{formatCurrency(row.spend)}</td>
                                    <td className="py-2 pr-3 tabular-nums">
                                      {row.avg_price_per_request == null ? '-' : formatAvgCostUsd(row.avg_price_per_request)}
                                    </td>
                                    <td className="py-2 pr-3 tabular-nums">
                                      {row.total_price == null ? '-' : formatCurrency(row.total_price)}
                                    </td>
                                    <td className="py-2 pr-3 tabular-nums">
                                      {row.margin == null ? '-' : formatCurrency(row.margin)}
                                    </td>
                                    <td className="py-2 pr-3 tabular-nums">
                                      {row.margin_pct == null ? '-' : `${(row.margin_pct * 100).toFixed(2)}%`}
                                    </td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        )}
                      </div>

                      <div className="border-t pt-4">
                        <h3 className="font-semibold mb-2">Recent Requests</h3>
                        {(monetizationApiKeyDrilldown.recent_requests ?? []).length === 0 ? (
                          <p className="text-sm text-muted-foreground">No recent requests available.</p>
                        ) : (
                          <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                              <thead className="text-left text-muted-foreground">
                                <tr>
                                  <th className="py-2 pr-3">Timestamp</th>
                                  <th className="py-2 pr-3">Request ID</th>
                                  <th className="py-2 pr-3">Model</th>
                                  <th className="py-2 pr-3">Provider</th>
                                  <th className="py-2 pr-3">Status</th>
                                  <th className="py-2 pr-3">Latency</th>
                                  <th className="py-2 pr-3">Cost USD</th>
                                </tr>
                              </thead>
                              <tbody>
                                {monetizationApiKeyDrilldown.recent_requests.map((row) => (
                                  <tr key={row.request_id} className="border-t">
                                    <td className="py-2 pr-3">{row.timestamp ? new Date(row.timestamp).toLocaleString() : '-'}</td>
                                    <td className="py-2 pr-3 font-mono text-xs">{row.request_id}</td>
                                    <td className="py-2 pr-3">{row.model}</td>
                                    <td className="py-2 pr-3">{row.provider}</td>
                                    <td className="py-2 pr-3">{row.status}</td>
                                    <td className="py-2 pr-3 tabular-nums">{row.latency_ms == null ? '-' : `${row.latency_ms.toFixed(1)} ms`}</td>
                                    <td className="py-2 pr-3 tabular-nums">{formatCurrency(row.cost_usd)}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        )}
                      </div>
                    </>
                  )}
                </div>
              )}
            </SheetContent>
          </Sheet>
        </div>
      ) : viewMode === 'api_keys_usage' ? (
        apiKeysUsageError ? (
          <SectionCard title="Unable to load API key usage">
            <p className="text-sm text-destructive">
              {apiKeysUsageError instanceof Error ? apiKeysUsageError.message : 'Request failed'}
            </p>
          </SectionCard>
        ) : (
        <>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-6 mb-6">
            {isLoadingApiKeysUsage ? (
              <>
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
              </>
            ) : (
              (() => {
                const rows = apiKeyUsageRows
                const totalRequests = apiKeyUsageSummary?.total_requests ?? rows.reduce((sum, r) => sum + Number(r.requests ?? 0), 0)
                const totalSpend = apiKeyUsageSummary?.total_spend ?? rows.reduce((sum, r) => sum + Number(r.total_cost_usd ?? 0), 0)
                const successRows = rows.filter((r) => r.success_rate != null)
                const avgSuccess = apiKeyUsageSummary?.avg_success_rate != null
                  ? Number(apiKeyUsageSummary.avg_success_rate) * 100
                  : successRows.length > 0
                  ? (successRows.reduce((sum, r) => sum + Number(r.success_rate ?? 0), 0) / successRows.length) * 100
                  : null
                const highestSpendKey = [...rows].sort((a, b) => Number(b.total_cost_usd ?? 0) - Number(a.total_cost_usd ?? 0))[0]
                const mostActiveKey = [...rows].sort((a, b) => Number(b.requests ?? 0) - Number(a.requests ?? 0))[0]
                return (
                  <>
                    <StatCard title="Total Active API Keys" value={String(apiKeyUsageSummary?.total_active_api_keys ?? apiKeyPagination.total ?? rows.length)} icon={KeyRound} description={`${windowHours}h window`} />
                    <StatCard title="Total Requests" value={totalRequests.toLocaleString()} icon={TableIcon} description="Visible leaderboard" />
                    <StatCard title="Total Spend" value={formatCurrency(totalSpend)} icon={DollarSign} description="Visible leaderboard" />
                    <StatCard title="Avg Success Rate" value={avgSuccess == null ? '-' : `${avgSuccess.toFixed(1)}%`} icon={CheckCircle} description="Visible leaderboard" />
                    <StatCard title="Highest Spend Key" value={apiKeyUsageSummary?.highest_spend_key || highestSpendKey?.api_key_name || '-'} icon={TrendingUp} description={highestSpendKey ? formatCurrency(highestSpendKey.total_cost_usd) : 'No data'} />
                    <StatCard title="Most Active Key" value={apiKeyUsageSummary?.most_active_key || mostActiveKey?.api_key_name || '-'} icon={Gauge} description={mostActiveKey ? `${Number(mostActiveKey.requests).toLocaleString()} reqs` : 'No data'} />
                  </>
                )
              })()
            )}
          </div>

          <SectionCard title="Filters">
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-6">
              <select
                value={apiKeyTenant}
                onChange={(e) => {
                  setApiKeyTenant(e.target.value)
                  setApiKeyPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Tenants</option>
                {Array.from(new Set(usageSummariesVisible.map((u) => u.tenant_id))).sort().map((tenantId) => (
                  <option key={tenantId} value={tenantId}>{tenantId}</option>
                ))}
              </select>
              <select
                value={apiKeyProvider}
                onChange={(e) => {
                  setApiKeyProvider(e.target.value)
                  setApiKeyPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Providers</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.provider ?? '')).filter(Boolean))).sort().map((provider) => (
                  <option key={provider} value={provider}>{provider}</option>
                ))}
              </select>
              <select
                value={apiKeyModel}
                onChange={(e) => {
                  setApiKeyModel(e.target.value)
                  setApiKeyPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Models</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.model ?? '')).filter(Boolean))).sort().map((model) => (
                  <option key={model} value={model}>{model}</option>
                ))}
              </select>
              <select
                value={apiKeyStatus}
                onChange={(e) => {
                  setApiKeyStatus(e.target.value)
                  setApiKeyPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Status</option>
                <option value="ok">ok</option>
                <option value="error">error</option>
              </select>
              <select
                value={apiKeyNameFilter}
                onChange={(e) => {
                  setApiKeyNameFilter(e.target.value)
                  setApiKeyPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="">All API Keys</option>
                {apiKeyNameOptions.map((apiKeyName) => (
                  <option key={apiKeyName} value={apiKeyName}>{apiKeyName}</option>
                ))}
              </select>
              <Button
                variant="outline"
                onClick={() => {
                  setApiKeyTenant('all')
                  setApiKeyProvider('all')
                  setApiKeyModel('all')
                  setApiKeyStatus('all')
                  setApiKeyNameFilter('')
                  setApiKeyPage(0)
                }}
              >
                Reset
              </Button>
            </div>
          </SectionCard>

          <div className="grid gap-4 md:grid-cols-3 mb-6">
            <SectionCard title="Top API Keys by Requests">
              {isLoadingApiKeysUsage ? (
                <Skeleton className="h-48" />
              ) : apiKeyUsageRows.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No API key usage data available</div>
              ) : (
                <div className="space-y-3">
                  {[...apiKeyUsageRows].sort((a, b) => b.requests - a.requests).slice(0, 8).map((row) => {
                    const max = Math.max(...apiKeyUsageRows.map((r) => r.requests), 0) || 1
                    const width = Math.max((row.requests / max) * 100, 2)
                    return (
                      <div key={`api-req-${row.api_key_id}`} className="space-y-1">
                        <div className="flex justify-between text-sm">
                          <span className="font-medium truncate pr-2">{row.api_key_name || row.api_key_id}</span>
                          <span className="text-muted-foreground">{row.requests.toLocaleString()}</span>
                        </div>
                        <div className="h-2 rounded bg-muted">
                          <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </SectionCard>
            <SectionCard title="Top API Keys by Spend">
              {isLoadingApiKeysUsage ? (
                <Skeleton className="h-48" />
              ) : apiKeyUsageRows.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No API key spend data available</div>
              ) : (
                <div className="space-y-3">
                  {[...apiKeyUsageRows].sort((a, b) => b.total_cost_usd - a.total_cost_usd).slice(0, 8).map((row) => {
                    const max = Math.max(...apiKeyUsageRows.map((r) => r.total_cost_usd), 0) || 1
                    const width = Math.max((row.total_cost_usd / max) * 100, 2)
                    return (
                      <div key={`api-spend-${row.api_key_id}`} className="space-y-1">
                        <div className="flex justify-between text-sm">
                          <span className="font-medium truncate pr-2">{row.api_key_name || row.api_key_id}</span>
                          <span className="text-muted-foreground">{formatCurrency(row.total_cost_usd)}</span>
                        </div>
                        <div className="h-2 rounded bg-muted">
                          <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </SectionCard>
            <SectionCard title="Success Rate by API Key">
              {isLoadingApiKeysUsage ? (
                <Skeleton className="h-48" />
              ) : apiKeyUsageRows.filter((r) => r.success_rate != null).length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No success rate data available</div>
              ) : (
                <div className="space-y-3">
                  {[...apiKeyUsageRows]
                    .filter((r) => r.success_rate != null)
                    .sort((a, b) => (b.success_rate ?? 0) - (a.success_rate ?? 0))
                    .slice(0, 8)
                    .map((row) => {
                      const rate = Number(row.success_rate ?? 0) * 100
                      return (
                        <div key={`api-sr-${row.api_key_id}`} className="space-y-1">
                          <div className="flex justify-between text-sm">
                            <span className="font-medium truncate pr-2">{row.api_key_name || row.api_key_id}</span>
                            <span className={`text-muted-foreground ${getSuccessRateClass(row.success_rate)}`}>{rate.toFixed(1)}%</span>
                          </div>
                          <div className="h-2 rounded bg-muted">
                            <div className="h-2 rounded bg-primary" style={{ width: `${Math.max(rate, 2)}%` }} />
                          </div>
                        </div>
                      )
                    })}
                </div>
              )}
            </SectionCard>
          </div>

          <SectionCard
            title="API Keys Leaderboard"
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const now = new Date()
                  const pad = (n: number) => String(n).padStart(2, '0')
                  const ts = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}`
                  const headers = ['generated_at', 'window', 'api_key_id', 'api_key_name', 'tenant_id', 'requests', 'total_cost_usd', 'avg_cost_per_request', 'success_rate', 'avg_latency_ms', 'top_model', 'top_provider', 'last_seen_at']
                  const lines = [headers.join(',')]
                  for (const row of apiKeyUsageRows) {
                    const avgCost = Number(row.requests ?? 0) > 0 ? Number(row.total_cost_usd ?? 0) / Number(row.requests ?? 0) : 0
                    const data = [
                      now.toISOString(),
                      `${windowHours}h`,
                      row.api_key_id,
                      row.api_key_name || '-',
                      row.tenant_id,
                      String(row.requests),
                      String(row.total_cost_usd),
                      String(avgCost),
                      row.success_rate == null ? '-' : String(row.success_rate),
                      row.avg_latency_ms == null ? '-' : String(row.avg_latency_ms),
                      row.top_model || '-',
                      row.top_provider || '-',
                      row.last_seen_at || '-',
                    ]
                    lines.push(data.map((v) => (String(v).includes(',') ? `"${String(v).replace(/\"/g, '\"')}"` : String(v))).join(','))
                  }
                  const csv = lines.join('\n')
                  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
                  const url = URL.createObjectURL(blob)
                  const link = document.createElement('a')
                  link.href = url
                  link.download = `api_key_usage_${ts}.csv`
                  document.body.appendChild(link)
                  link.click()
                  document.body.removeChild(link)
                  URL.revokeObjectURL(url)
                }}
                className="gap-2"
              >
                <Download className="h-4 w-4" />
                Export CSV
              </Button>
            }
          >
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-muted-foreground">
                  <tr>
                    <th className="py-2 pr-3">API Key</th>
                    <th className="py-2 pr-3">Tenant</th>
                    <th className="py-2 pr-3">Requests</th>
                    <th className="py-2 pr-3">Spend</th>
                    <th className="py-2 pr-3">Avg Cost / Request</th>
                    <th className="py-2 pr-3">Success Rate</th>
                    <th className="py-2 pr-3">Avg Latency</th>
                    <th className="py-2 pr-3">Top Model</th>
                    <th className="py-2 pr-3">Model Type</th>
                    <th className="py-2 pr-3">Top Provider</th>
                    <th className="py-2 pr-3">Last Seen</th>
                  </tr>
                </thead>
                <tbody>
                  {isLoadingApiKeysUsage ? (
                    <tr className="border-t"><td className="py-6 text-center" colSpan={11}><Skeleton className="h-6" /></td></tr>
                  ) : apiKeyUsageRows.length === 0 ? (
                    <tr className="border-t"><td className="py-6 text-center text-muted-foreground" colSpan={11}>No API key usage data in the selected window</td></tr>
                  ) : (
                    [...apiKeyUsageRows]
                      .sort((a, b) => Number(b.total_cost_usd ?? 0) - Number(a.total_cost_usd ?? 0))
                      .map((row) => (
                        <tr
                          key={row.api_key_id}
                          className="border-t cursor-pointer hover:bg-muted/40"
                          onClick={() => {
                            setSelectedApiKey(row)
                            setApiKeyDrawerOpen(true)
                          }}
                        >
                          <td className="py-2 pr-3 font-medium">{row.api_key_name || row.api_key_id}</td>
                          <td className="py-2 pr-3">{row.tenant_id}</td>
                          <td className="py-2 pr-3 tabular-nums">{Number(row.requests ?? 0).toLocaleString()}</td>
                          <td className="py-2 pr-3 tabular-nums">{formatCurrency(Number(row.total_cost_usd ?? 0))}</td>
                          <td className="py-2 pr-3 tabular-nums">
                            {formatAvgCostUsd(
                              row.avg_cost_per_request_effective == null
                                ? Number(row.total_cost_usd ?? 0) / Math.max(1, Number(row.requests ?? 0))
                                : Number(row.avg_cost_per_request_effective)
                            )}
                          </td>
                          <td className={`py-2 pr-3 tabular-nums ${getSuccessRateClass(row.success_rate)}`}>{row.success_rate == null ? '-' : `${(row.success_rate * 100).toFixed(1)}%`}</td>
                          <td className="py-2 pr-3 tabular-nums">{row.avg_latency_ms == null ? '-' : `${row.avg_latency_ms.toFixed(1)} ms`}</td>
                          <td className="py-2 pr-3">{row.top_model ?? '-'}</td>
                          <td className="py-2 pr-3">
                            {row.top_model
                              ? (() => {
                                  const t = modelTypeMap.get(row.top_model)
                                  if (t === undefined) return 'unknown'
                                  if (t === 'ml') return 'ml'
                                  if (t === 'embedding') return 'embedding'
                                  return 'llm'
                                })()
                              : '-'}
                          </td>
                          <td className="py-2 pr-3">{row.top_provider ?? '-'}</td>
                          <td className="py-2 pr-3">{row.last_seen_at ? new Date(row.last_seen_at).toLocaleString() : '-'}</td>
                        </tr>
                      ))
                  )}
                </tbody>
              </table>
            </div>
            {apiKeyPagination.total > 0 && (
              <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
                <div>
                  Showing {apiKeyPagination.offset + 1} to {Math.min(apiKeyPagination.offset + apiKeyPagination.returned, apiKeyPagination.total)} of {apiKeyPagination.total} API keys
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => setApiKeyPage(Math.max(0, apiKeyPage - 1))} disabled={apiKeyPage === 0}>
                    Previous
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setApiKeyPage(apiKeyPage + 1)} disabled={apiKeyPagination.offset + apiKeyPagination.returned >= apiKeyPagination.total}>
                    Next
                  </Button>
                </div>
              </div>
            )}
          </SectionCard>

          <Sheet open={apiKeyDrawerOpen} onOpenChange={setApiKeyDrawerOpen}>
            <SheetContent className="w-full sm:max-w-2xl overflow-y-auto">
              <SheetHeader>
                <SheetTitle>API Key Drilldown</SheetTitle>
                <SheetDescription>
                  {selectedApiKey?.api_key_name || selectedApiKey?.api_key_id || 'API key details'}
                </SheetDescription>
              </SheetHeader>
              {!selectedApiKey ? null : (
                <div className="mt-6 space-y-6">
                  <div className="grid grid-cols-2 gap-3 text-sm">
                    <div><span className="text-muted-foreground">API Key Name:</span> {selectedApiKey.api_key_name || '-'}</div>
                    <div><span className="text-muted-foreground">API Key ID:</span> {selectedApiKey.api_key_id || '-'}</div>
                    <div><span className="text-muted-foreground">Tenant:</span> {selectedApiKey.tenant_id || '-'}</div>
                    <div><span className="text-muted-foreground">Requests:</span> {Number(selectedApiKey.requests ?? 0).toLocaleString()}</div>
                    <div><span className="text-muted-foreground">Spend:</span> {formatCurrency(Number(selectedApiKey.total_cost_usd ?? 0))}</div>
                    <div>
                      <span className="text-muted-foreground">Avg Cost / Request:</span>{' '}
                      {formatAvgCostUsd(
                        selectedApiKey.avg_cost_per_request_effective == null
                          ? Number(selectedApiKey.total_cost_usd ?? 0) / Math.max(1, Number(selectedApiKey.requests ?? 0))
                          : Number(selectedApiKey.avg_cost_per_request_effective)
                      )}
                    </div>
                    <div><span className="text-muted-foreground">Success Rate:</span> {selectedApiKey.success_rate == null ? '-' : `${(selectedApiKey.success_rate * 100).toFixed(1)}%`}</div>
                    <div><span className="text-muted-foreground">Avg Latency:</span> {selectedApiKey.avg_latency_ms == null ? '-' : `${selectedApiKey.avg_latency_ms.toFixed(1)} ms`}</div>
                    <div><span className="text-muted-foreground">Top Model:</span> {selectedApiKey.top_model ?? '-'}</div>
                    <div><span className="text-muted-foreground">Top Provider:</span> {selectedApiKey.top_provider ?? '-'}</div>
                    <div><span className="text-muted-foreground">Last Seen:</span> {selectedApiKey.last_seen_at ? new Date(selectedApiKey.last_seen_at).toLocaleString() : '-'}</div>
                  </div>

                  {apiKeyDrilldownError ? (
                    <div className="border-t pt-4 text-sm text-destructive">
                      {apiKeyDrilldownError instanceof Error ? apiKeyDrilldownError.message : 'Failed to load drilldown'}
                    </div>
                  ) : isLoadingApiKeyDrilldown ? (
                    <div className="space-y-3 border-t pt-4">
                      <Skeleton className="h-20" />
                      <Skeleton className="h-20" />
                      <Skeleton className="h-20" />
                    </div>
                  ) : !apiKeyDrilldown ? (
                    <div className="border-t pt-4 text-sm text-muted-foreground">Detailed drilldown data is not available yet for this API key.</div>
                  ) : (
                    <>
                  <div className="grid grid-cols-2 gap-3 text-sm">
                    <div><span className="text-muted-foreground">API Key:</span> {apiKeyDrilldown.api_key_name || '-'}</div>
                    <div><span className="text-muted-foreground">Tenant:</span> {apiKeyDrilldown.tenant_id || '-'}</div>
                    <div><span className="text-muted-foreground">Requests:</span> {apiKeyDrilldown.summary.requests.toLocaleString()}</div>
                    <div><span className="text-muted-foreground">Total Cost:</span> {formatCurrency(apiKeyDrilldown.summary.total_cost_usd)}</div>
                    <div>
                      <span className="text-muted-foreground">Avg Cost / Request:</span>{' '}
                      {formatAvgCostUsd(
                        apiKeyDrilldown.summary.avg_cost_per_request_effective == null
                          ? apiKeyDrilldown.summary.total_cost_usd / Math.max(1, apiKeyDrilldown.summary.requests)
                          : apiKeyDrilldown.summary.avg_cost_per_request_effective
                      )}
                    </div>
                    <div><span className="text-muted-foreground">Success Rate:</span> {apiKeyDrilldown.summary.success_rate == null ? '-' : `${(apiKeyDrilldown.summary.success_rate * 100).toFixed(1)}%`}</div>
                    <div><span className="text-muted-foreground">Avg Latency:</span> {apiKeyDrilldown.summary.avg_latency_ms == null ? '-' : `${apiKeyDrilldown.summary.avg_latency_ms.toFixed(1)} ms`}</div>
                    <div><span className="text-muted-foreground">Top Model:</span> {apiKeyDrilldown.summary.top_model ?? '-'}</div>
                    <div><span className="text-muted-foreground">Top Provider:</span> {apiKeyDrilldown.summary.top_provider ?? '-'}</div>
                    <div><span className="text-muted-foreground">Last Seen:</span> {apiKeyDrilldown.summary.last_seen_at ? new Date(apiKeyDrilldown.summary.last_seen_at).toLocaleString() : '-'}</div>
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Latency Percentiles</h3>
                    {(() => {
                      const p = apiKeyDrilldown.latency_percentiles || { p50: 0, p95: 0, max: 0 }
                      const allZero = [p.p50, p.p95, p.max].every((v) => !Number.isFinite(v) || Number(v) === 0)
                      if (allZero) {
                        return <p className="text-sm text-muted-foreground">No latency percentile data available</p>
                      }
                      return (
                        <div className="grid grid-cols-3 gap-3">
                          <div className="rounded-md border p-3">
                            <div className="text-xs text-muted-foreground">p50</div>
                            <div className="text-lg font-semibold">{formatLatencyMs(p.p50)}</div>
                          </div>
                          <div className="rounded-md border p-3">
                            <div className="text-xs text-muted-foreground">p95</div>
                            <div className="text-lg font-semibold">{formatLatencyMs(p.p95)}</div>
                          </div>
                          <div className="rounded-md border p-3">
                            <div className="text-xs text-muted-foreground">max</div>
                            <div className="text-lg font-semibold">{formatLatencyMs(p.max)}</div>
                          </div>
                        </div>
                      )
                    })()}
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Cost Distribution by Model</h3>
                    {(apiKeyDrilldown.requests_by_model ?? []).length === 0 ? (
                      <p className="text-sm text-muted-foreground">No model breakdown available.</p>
                    ) : (
                      <div className="space-y-2">
                        {(() => {
                          const rows = [...(apiKeyDrilldown.requests_by_model ?? [])].sort((a, b) => Number(b.cost_usd ?? 0) - Number(a.cost_usd ?? 0)).slice(0, 10)
                          const max = Math.max(...rows.map((r) => Number(r.cost_usd ?? 0)), 0) || 1
                          const total = Number(apiKeyDrilldown.summary.total_cost_usd ?? 0) || rows.reduce((s, r) => s + Number(r.cost_usd ?? 0), 0)
                          return rows.map((row) => {
                            const cost = Number(row.cost_usd ?? 0)
                            const width = Math.max((cost / max) * 100, 2)
                            const pct = total > 0 ? (cost / total) * 100 : 0
                            return (
                              <div key={`ak-model-${row.model}`} className="space-y-1">
                                <div className="flex justify-between text-sm">
                                  <span>{row.model || '-'}</span>
                                  <span className="text-muted-foreground">{Number(row.requests ?? 0).toLocaleString()} req • {formatCurrency(cost)} • {pct.toFixed(1)}%</span>
                                </div>
                                <div className="h-2 rounded bg-muted">
                                  <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                                </div>
                              </div>
                            )
                          })
                        })()}
                      </div>
                    )}
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Requests by Provider</h3>
                    {(apiKeyDrilldown.requests_by_provider ?? []).length === 0 ? (
                      <p className="text-sm text-muted-foreground">No provider breakdown available.</p>
                    ) : (
                      <div className="space-y-2">
                        {[...(apiKeyDrilldown.requests_by_provider ?? [])].sort((a, b) => Number(b.requests ?? 0) - Number(a.requests ?? 0)).map((row) => (
                          <div key={`ak-provider-${row.provider}`} className="flex items-center justify-between text-sm">
                            <span>{row.provider || '-'}</span>
                            <span className="text-muted-foreground">{Number(row.requests ?? 0).toLocaleString()} req</span>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Error Distribution</h3>
                    {(() => {
                      const recents = apiKeyDrilldown.recent_requests ?? []
                      const counts = new Map<string, number>()
                      for (const r of recents) {
                        const isError = String(r.status || '').toLowerCase() !== 'ok'
                        if (!isError) continue
                        const key = (r.error_type && r.error_type.trim().length > 0) ? r.error_type.trim() : 'unknown'
                        counts.set(key, (counts.get(key) || 0) + 1)
                      }
                      const rows = Array.from(counts.entries()).map(([error_type, count]) => ({ error_type, count })).sort((a, b) => b.count - a.count)
                      if (rows.length === 0) {
                        return <p className="text-sm text-muted-foreground">No errors recorded in the selected window.</p>
                      }
                      const top = rows.slice(0, 10)
                      const max = Math.max(...top.map(r => r.count), 1)
                      return (
                        <div className="space-y-2">
                          {top.map((r) => {
                            const width = Math.max((r.count / max) * 100, 2)
                            return (
                              <div key={`ak-error-${r.error_type}`} className="space-y-1">
                                <div className="flex justify-between text-sm">
                                  <span>{r.error_type || 'unknown'}</span>
                                  <span className="text-muted-foreground">{r.count.toLocaleString()}</span>
                                </div>
                                <div className="h-2 rounded bg-muted">
                                  <div className="h-2 rounded bg-destructive/80" style={{ width: `${width}%` }} />
                                </div>
                              </div>
                            )
                          })}
                        </div>
                      )
                    })()}
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Traffic Over Time</h3>
                    {(() => {
                      const builtin = apiKeyDrilldown.traffic_over_time ?? []
                      if (builtin.length > 0) {
                        return (
                          <div className="space-y-2">
                            {builtin.slice(-12).map((row) => (
                              <div key={`ak-traffic-${row.bucket}`} className="flex items-center justify-between text-sm">
                                <span className="text-muted-foreground">{new Date(row.bucket).toLocaleString()}</span>
                                <span>{Number(row.requests ?? 0).toLocaleString()} req / {Number(row.errors ?? 0).toLocaleString()} err</span>
                              </div>
                            ))}
                          </div>
                        )
                      }
                      const recent = apiKeyDrilldown.recent_requests ?? []
                      if ((recent ?? []).length === 0) {
                        return <p className="text-sm text-muted-foreground">No traffic timeline available.</p>
                      }
                      const times = recent.map(r => new Date(r.timestamp).getTime()).filter(n => Number.isFinite(n))
                      const minTs = Math.min(...times)
                      const maxTs = Math.max(...times)
                      const rangeMs = Math.max(0, maxTs - minTs)
                      const minute = 60 * 1000
                      const hour = 60 * minute
                      const bucketMs = (rangeMs <= 2 * hour && recent.length <= 200) ? minute : hour
                      const buckets = new Map<number, { ts: number; count: number; spend: number }>()
                      for (const r of recent) {
                        const t = new Date(r.timestamp).getTime()
                        if (!Number.isFinite(t)) continue
                        const b = Math.floor(t / bucketMs) * bucketMs
                        const cur = buckets.get(b) || { ts: b, count: 0, spend: 0 }
                        cur.count += 1
                        cur.spend += Number(r.cost_usd ?? 0)
                        buckets.set(b, cur)
                      }
                      const rows = Array.from(buckets.values()).sort((a, b) => a.ts - b.ts)
                      const last = rows.slice(-24) // limit items
                      const maxCount = Math.max(...last.map(r => r.count), 1)
                      return (
                        <div className="space-y-2">
                          {last.map((r) => {
                            const width = Math.max((r.count / maxCount) * 100, 2)
                            const label = new Date(r.ts).toLocaleString()
                            return (
                              <div key={`ak-traffic-derived-${r.ts}`} className="space-y-1">
                                <div className="flex justify-between text-sm">
                                  <span className="text-muted-foreground">{label}</span>
                                  <span>{r.count.toLocaleString()} req{r.spend > 0 ? ` • ${formatCurrency(r.spend)}` : ''}</span>
                                </div>
                                <div className="h-2 rounded bg-muted">
                                  <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                                </div>
                              </div>
                            )
                          })}
                        </div>
                      )
                    })()}
                  </div>

                  <div className="border-t pt-4">
                    <h3 className="font-semibold mb-2">Recent Requests</h3>
                    {(apiKeyDrilldown.recent_requests ?? []).length === 0 ? (
                      <p className="text-sm text-muted-foreground">No recent requests available.</p>
                    ) : (
                      <div className="space-y-2">
                        {(apiKeyDrilldown.recent_requests ?? []).slice(0, 10).map((row) => (
                          <div key={row.request_id} className="rounded-md border p-2 text-sm">
                            <div className="flex items-center justify-between">
                              <span className="font-medium">
                                {row.model || '-'}
                                {row.model && (
                                  <span className="ml-1 font-normal text-muted-foreground">
                                    ({(() => {
                                      const t = modelTypeMap.get(row.model)
                                      if (t === undefined) return 'unknown'
                                      if (t === 'ml') return 'ml'
                                      if (t === 'embedding') return 'embedding'
                                      return 'llm'
                                    })()})
                                  </span>
                                )}
                              </span>
                              <span className="text-muted-foreground">{new Date(row.timestamp).toLocaleString()}</span>
                            </div>
                            <div className="mt-1 text-muted-foreground">
                              {row.provider || '-'} • {row.latency_ms} ms • {row.status} • {formatCurrency(Number(row.cost_usd ?? 0))} {row.fallback_used ? '• fallback' : ''}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                    </>
                  )}
                </div>
              )}
            </SheetContent>
          </Sheet>
        </>
        )
      ) : viewMode === 'jwt_sub_usage' ? (
        jwtSubUsageError ? (
          <SectionCard title="Unable to load JWT sub usage">
            <p className="text-sm text-destructive">
              {jwtSubUsageError instanceof Error ? jwtSubUsageError.message : 'Request failed'}
            </p>
          </SectionCard>
        ) : (
        <>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-6 mb-6">
            {isLoadingJwtSubUsage ? (
              <>
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
              </>
            ) : (
              (() => {
                const rows = jwtSubUsageRows
                const totalRequests = jwtSubUsageSummary?.total_requests ?? rows.reduce((sum, r) => sum + Number(r.requests ?? 0), 0)
                const totalSpend = jwtSubUsageSummary?.total_spend ?? rows.reduce((sum, r) => sum + Number(r.total_cost_usd ?? 0), 0)
                const successRows = rows.filter((r) => r.success_rate != null)
                const avgSuccess = jwtSubUsageSummary?.avg_success_rate != null
                  ? Number(jwtSubUsageSummary.avg_success_rate) * 100
                  : successRows.length > 0
                  ? (successRows.reduce((sum, r) => sum + Number(r.success_rate ?? 0), 0) / successRows.length) * 100
                  : null
                const highestSpendSub = [...rows].sort((a, b) => Number(b.total_cost_usd ?? 0) - Number(a.total_cost_usd ?? 0))[0]
                const mostActiveSub = [...rows].sort((a, b) => Number(b.requests ?? 0) - Number(a.requests ?? 0))[0]
                return (
                  <>
                    <StatCard title="Total Active JWT Subs" value={String(jwtSubUsageSummary?.total_active_jwt_subs ?? jwtSubPagination.total ?? rows.length)} icon={KeyRound} description={`${windowHours}h window`} />
                    <StatCard title="Total Requests" value={totalRequests.toLocaleString()} icon={TableIcon} description="Visible leaderboard" />
                    <StatCard title="Total Spend" value={formatCurrency(totalSpend)} icon={DollarSign} description="Visible leaderboard" />
                    <StatCard title="Avg Success Rate" value={avgSuccess == null ? '-' : `${avgSuccess.toFixed(1)}%`} icon={CheckCircle} description="Visible leaderboard" />
                    <StatCard title="Highest Spend Sub" value={jwtSubUsageSummary?.highest_spend_sub || highestSpendSub?.jwt_sub || '-'} icon={TrendingUp} description={highestSpendSub ? formatCurrency(highestSpendSub.total_cost_usd) : 'No data'} />
                    <StatCard title="Most Active Sub" value={jwtSubUsageSummary?.most_active_sub || mostActiveSub?.jwt_sub || '-'} icon={Gauge} description={mostActiveSub ? `${Number(mostActiveSub.requests).toLocaleString()} reqs` : 'No data'} />
                  </>
                )
              })()
            )}
          </div>

          <SectionCard title="Filters">
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-6">
              <select
                value={jwtSubTenant}
                onChange={(e) => {
                  setJwtSubTenant(e.target.value)
                  setJwtSubPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Tenants</option>
                {Array.from(new Set(usageSummariesVisible.map((u) => u.tenant_id))).sort().map((tenantId) => (
                  <option key={tenantId} value={tenantId}>{tenantId}</option>
                ))}
              </select>
              <select
                value={jwtSubProvider}
                onChange={(e) => {
                  setJwtSubProvider(e.target.value)
                  setJwtSubPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Providers</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.provider ?? '')).filter(Boolean))).sort().map((provider) => (
                  <option key={provider} value={provider}>{provider}</option>
                ))}
              </select>
              <select
                value={jwtSubModel}
                onChange={(e) => {
                  setJwtSubModel(e.target.value)
                  setJwtSubPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Models</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.model ?? '')).filter(Boolean))).sort().map((model) => (
                  <option key={model} value={model}>{model}</option>
                ))}
              </select>
              <select
                value={jwtSubStatus}
                onChange={(e) => {
                  setJwtSubStatus(e.target.value)
                  setJwtSubPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Status</option>
                <option value="ok">ok</option>
                <option value="error">error</option>
              </select>
              <select
                value={jwtSubFilter}
                onChange={(e) => {
                  setJwtSubFilter(e.target.value)
                  setJwtSubPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="">All JWT Subs</option>
                {jwtSubOptions.map((jwtSub) => (
                  <option key={jwtSub} value={jwtSub}>{jwtSub}</option>
                ))}
              </select>
              <Button
                variant="outline"
                onClick={() => {
                  setJwtSubTenant('all')
                  setJwtSubProvider('all')
                  setJwtSubModel('all')
                  setJwtSubStatus('all')
                  setJwtSubFilter('')
                  setJwtSubPage(0)
                }}
              >
                Reset
              </Button>
            </div>
          </SectionCard>

          <div className="grid gap-4 md:grid-cols-3 mb-6">
            <SectionCard title="Top JWT Subs by Requests">
              {isLoadingJwtSubUsage ? (
                <Skeleton className="h-48" />
              ) : jwtSubUsageRows.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No JWT sub usage data available</div>
              ) : (
                <div className="space-y-3">
                  {[...jwtSubUsageRows].sort((a, b) => b.requests - a.requests).slice(0, 8).map((row) => {
                    const max = Math.max(...jwtSubUsageRows.map((r) => r.requests), 0) || 1
                    const width = Math.max((row.requests / max) * 100, 2)
                    return (
                      <div key={`jwt-req-${row.jwt_sub}`} className="space-y-1">
                        <div className="flex justify-between text-sm">
                          <span className="font-medium truncate pr-2">{row.jwt_sub || '-'}</span>
                          <span className="text-muted-foreground">{row.requests.toLocaleString()}</span>
                        </div>
                        <div className="h-2 rounded bg-muted">
                          <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </SectionCard>
            <SectionCard title="Top JWT Subs by Spend">
              {isLoadingJwtSubUsage ? (
                <Skeleton className="h-48" />
              ) : jwtSubUsageRows.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No JWT sub spend data available</div>
              ) : (
                <div className="space-y-3">
                  {[...jwtSubUsageRows].sort((a, b) => b.total_cost_usd - a.total_cost_usd).slice(0, 8).map((row) => {
                    const max = Math.max(...jwtSubUsageRows.map((r) => r.total_cost_usd), 0) || 1
                    const width = Math.max((row.total_cost_usd / max) * 100, 2)
                    return (
                      <div key={`jwt-spend-${row.jwt_sub}`} className="space-y-1">
                        <div className="flex justify-between text-sm">
                          <span className="font-medium truncate pr-2">{row.jwt_sub || '-'}</span>
                          <span className="text-muted-foreground">{formatCurrency(row.total_cost_usd)}</span>
                        </div>
                        <div className="h-2 rounded bg-muted">
                          <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </SectionCard>
            <SectionCard title="Success Rate by JWT Sub">
              {isLoadingJwtSubUsage ? (
                <Skeleton className="h-48" />
              ) : jwtSubUsageRows.filter((r) => r.success_rate != null).length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No success rate data available</div>
              ) : (
                <div className="space-y-3">
                  {[...jwtSubUsageRows]
                    .filter((r) => r.success_rate != null)
                    .sort((a, b) => (b.success_rate ?? 0) - (a.success_rate ?? 0))
                    .slice(0, 8)
                    .map((row) => {
                      const rate = Number(row.success_rate ?? 0) * 100
                      return (
                        <div key={`jwt-sr-${row.jwt_sub}`} className="space-y-1">
                          <div className="flex justify-between text-sm">
                            <span className="font-medium truncate pr-2">{row.jwt_sub || '-'}</span>
                            <span className={`text-muted-foreground ${getSuccessRateClass(row.success_rate ?? null)}`}>{rate.toFixed(1)}%</span>
                          </div>
                          <div className="h-2 rounded bg-muted">
                            <div className="h-2 rounded bg-primary" style={{ width: `${Math.max(rate, 2)}%` }} />
                          </div>
                        </div>
                      )
                    })}
                </div>
              )}
            </SectionCard>
          </div>

          <SectionCard
            title="JWT Subs Leaderboard"
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const now = new Date()
                  const pad = (n: number) => String(n).padStart(2, '0')
                  const ts = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}`
                  const headers = ['generated_at', 'window', 'jwt_sub', 'tenant_id', 'requests', 'prompt_tokens', 'completion_tokens', 'total_tokens', 'total_cost_usd', 'avg_cost_per_request', 'first_seen', 'last_seen']
                  const lines = [headers.join(',')]
                  for (const row of jwtSubUsageRows) {
                    const avgCost = Number(row.requests ?? 0) > 0 ? Number(row.total_cost_usd ?? 0) / Number(row.requests ?? 0) : 0
                    const data = [
                      now.toISOString(),
                      `${windowHours}h`,
                      row.jwt_sub,
                      row.tenant_id || '-',
                      String(row.requests),
                      String(row.prompt_tokens ?? 0),
                      String(row.completion_tokens ?? 0),
                      String(row.total_tokens ?? 0),
                      String(row.total_cost_usd),
                      String(avgCost),
                      row.first_seen || '-',
                      row.last_seen || '-',
                    ]
                    lines.push(data.map((v) => (String(v).includes(',') ? `"${String(v).replace(/\"/g, '\"')}"` : String(v))).join(','))
                  }
                  const csv = lines.join('\n')
                  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
                  const url = URL.createObjectURL(blob)
                  const link = document.createElement('a')
                  link.href = url
                  link.download = `jwt_sub_usage_${ts}.csv`
                  document.body.appendChild(link)
                  link.click()
                  document.body.removeChild(link)
                  URL.revokeObjectURL(url)
                }}
                className="gap-2"
              >
                <Download className="h-4 w-4" />
                Export CSV
              </Button>
            }
          >
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-muted-foreground">
                  <tr>
                    <th className="py-2 pr-3">JWT Sub</th>
                    <th className="py-2 pr-3">Tenant</th>
                    <th className="py-2 pr-3">Requests</th>
                    <th className="py-2 pr-3">Prompt</th>
                    <th className="py-2 pr-3">Completion</th>
                    <th className="py-2 pr-3">Total Tokens</th>
                    <th className="py-2 pr-3">Spend</th>
                    <th className="py-2 pr-3">Avg Cost / Request</th>
                    <th className="py-2 pr-3">First Seen</th>
                    <th className="py-2 pr-3">Last Seen</th>
                  </tr>
                </thead>
                <tbody>
                  {isLoadingJwtSubUsage ? (
                    <tr className="border-t"><td className="py-6 text-center" colSpan={10}><Skeleton className="h-6" /></td></tr>
                  ) : jwtSubUsageRows.length === 0 ? (
                    <tr className="border-t"><td className="py-6 text-center text-muted-foreground" colSpan={10}>No JWT sub usage data in the selected window</td></tr>
                  ) : (
                    [...jwtSubUsageRows]
                      .sort((a, b) => Number(b.total_cost_usd ?? 0) - Number(a.total_cost_usd ?? 0))
                      .map((row) => (
                        <tr
                          key={row.jwt_sub}
                          className="border-t cursor-pointer hover:bg-muted/40"
                          onClick={() => {
                            setSelectedJwtSub(row)
                            setJwtSubDrawerOpen(true)
                          }}
                        >
                          <td className="py-2 pr-3 font-medium">{row.jwt_sub || '-'}</td>
                          <td className="py-2 pr-3">{row.tenant_id || '-'}</td>
                          <td className="py-2 pr-3 tabular-nums">{Number(row.requests ?? 0).toLocaleString()}</td>
                          <td className="py-2 pr-3 tabular-nums">{Number(row.prompt_tokens ?? 0).toLocaleString()}</td>
                          <td className="py-2 pr-3 tabular-nums">{Number(row.completion_tokens ?? 0).toLocaleString()}</td>
                          <td className="py-2 pr-3 tabular-nums">{Number(row.total_tokens ?? 0).toLocaleString()}</td>
                          <td className="py-2 pr-3 tabular-nums">{formatCurrency(Number(row.total_cost_usd ?? 0))}</td>
                          <td className="py-2 pr-3 tabular-nums">
                            {formatAvgCostUsd(
                              row.avg_cost_per_request_effective == null
                                ? Number(row.total_cost_usd ?? 0) / Math.max(1, Number(row.requests ?? 0))
                                : Number(row.avg_cost_per_request_effective)
                            )}
                          </td>
                          <td className="py-2 pr-3">{row.first_seen ? new Date(row.first_seen).toLocaleString() : '-'}</td>
                          <td className="py-2 pr-3">{row.last_seen ? new Date(row.last_seen).toLocaleString() : '-'}</td>
                        </tr>
                      ))
                  )}
                </tbody>
              </table>
            </div>
            {jwtSubPagination.total > 0 && (
              <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
                <div>
                  Showing {jwtSubPagination.offset + 1} to {Math.min(jwtSubPagination.offset + jwtSubPagination.returned, jwtSubPagination.total)} of {jwtSubPagination.total} JWT subs
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => setJwtSubPage(Math.max(0, jwtSubPage - 1))} disabled={jwtSubPage === 0}>
                    Previous
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setJwtSubPage(jwtSubPage + 1)} disabled={jwtSubPagination.offset + jwtSubPagination.returned >= jwtSubPagination.total}>
                    Next
                  </Button>
                </div>
              </div>
            )}
          </SectionCard>

          <Sheet open={jwtSubDrawerOpen} onOpenChange={setJwtSubDrawerOpen}>
            <SheetContent className="w-full sm:max-w-2xl overflow-y-auto">
              <SheetHeader>
                <SheetTitle>JWT Sub Drilldown</SheetTitle>
                <SheetDescription>
                  {selectedJwtSub?.jwt_sub || 'JWT subject details'}
                </SheetDescription>
              </SheetHeader>
              {!selectedJwtSub ? null : (
                <div className="mt-6 space-y-6">
                  <div className="grid grid-cols-2 gap-3 text-sm">
                    <div><span className="text-muted-foreground">JWT Sub:</span> {selectedJwtSub.jwt_sub || '-'}</div>
                    <div><span className="text-muted-foreground">Tenant:</span> {selectedJwtSub.tenant_id || '-'}</div>
                    <div><span className="text-muted-foreground">Requests:</span> {Number(selectedJwtSub.requests ?? 0).toLocaleString()}</div>
                    <div><span className="text-muted-foreground">Spend:</span> {formatCurrency(Number(selectedJwtSub.total_cost_usd ?? 0))}</div>
                    <div><span className="text-muted-foreground">Prompt Tokens:</span> {Number(selectedJwtSub.prompt_tokens ?? 0).toLocaleString()}</div>
                    <div><span className="text-muted-foreground">Completion Tokens:</span> {Number(selectedJwtSub.completion_tokens ?? 0).toLocaleString()}</div>
                    <div><span className="text-muted-foreground">Total Tokens:</span> {Number(selectedJwtSub.total_tokens ?? 0).toLocaleString()}</div>
                    <div><span className="text-muted-foreground">First Seen:</span> {selectedJwtSub.first_seen ? new Date(selectedJwtSub.first_seen).toLocaleString() : '-'}</div>
                    <div><span className="text-muted-foreground">Last Seen:</span> {selectedJwtSub.last_seen ? new Date(selectedJwtSub.last_seen).toLocaleString() : '-'}</div>
                  </div>

                  {jwtSubDrilldownError ? (
                    <div className="border-t pt-4 text-sm text-destructive">
                      {jwtSubDrilldownError instanceof Error ? jwtSubDrilldownError.message : 'Failed to load drilldown'}
                    </div>
                  ) : isLoadingJwtSubDrilldown ? (
                    <div className="space-y-3 border-t pt-4">
                      <Skeleton className="h-20" />
                      <Skeleton className="h-20" />
                      <Skeleton className="h-20" />
                    </div>
                  ) : !jwtSubDrilldown ? (
                    <div className="border-t pt-4 text-sm text-muted-foreground">Detailed drilldown data is not available yet for this JWT subject.</div>
                  ) : (
                    <>
                      <div className="grid grid-cols-2 gap-3 text-sm">
                        <div><span className="text-muted-foreground">Requests:</span> {jwtSubDrilldown.summary.requests.toLocaleString()}</div>
                        <div><span className="text-muted-foreground">Total Cost:</span> {formatCurrency(jwtSubDrilldown.summary.total_cost_usd)}</div>
                        <div><span className="text-muted-foreground">Prompt Tokens:</span> {jwtSubDrilldown.summary.prompt_tokens.toLocaleString()}</div>
                        <div><span className="text-muted-foreground">Completion Tokens:</span> {jwtSubDrilldown.summary.completion_tokens.toLocaleString()}</div>
                        <div><span className="text-muted-foreground">Total Tokens:</span> {jwtSubDrilldown.summary.total_tokens.toLocaleString()}</div>
                      </div>

                      <div className="border-t pt-4">
                        <h3 className="font-semibold mb-2">Cost Distribution by Model</h3>
                        {(jwtSubDrilldown.requests_by_model ?? []).length === 0 ? (
                          <p className="text-sm text-muted-foreground">No model breakdown available.</p>
                        ) : (
                          <div className="space-y-2">
                            {jwtSubDrilldown.requests_by_model.slice(0, 10).map((row) => {
                              const max = Math.max(...jwtSubDrilldown.requests_by_model.map((x) => x.cost_usd), 0) || 1
                              const width = Math.max((row.cost_usd / max) * 100, 2)
                              return (
                                <div key={`jwt-model-${row.model}`} className="space-y-1">
                                  <div className="flex justify-between text-sm">
                                    <span>{row.model}</span>
                                    <span className="text-muted-foreground">{formatCurrency(row.cost_usd)}</span>
                                  </div>
                                  <div className="h-2 rounded bg-muted">
                                    <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                                  </div>
                                </div>
                              )
                            })}
                          </div>
                        )}
                      </div>

                      <div className="border-t pt-4">
                        <h3 className="font-semibold mb-2">Requests by Provider</h3>
                        {(jwtSubDrilldown.requests_by_provider ?? []).length === 0 ? (
                          <p className="text-sm text-muted-foreground">No provider breakdown available.</p>
                        ) : (
                          <div className="space-y-2">
                            {jwtSubDrilldown.requests_by_provider.slice(0, 10).map((row) => {
                              const max = Math.max(...jwtSubDrilldown.requests_by_provider.map((x) => x.requests), 0) || 1
                              const width = Math.max((row.requests / max) * 100, 2)
                              return (
                                <div key={`jwt-provider-${row.provider}`} className="space-y-1">
                                  <div className="flex justify-between text-sm">
                                    <span>{row.provider}</span>
                                    <span className="text-muted-foreground">{row.requests.toLocaleString()}</span>
                                  </div>
                                  <div className="h-2 rounded bg-muted">
                                    <div className="h-2 rounded bg-primary" style={{ width: `${width}%` }} />
                                  </div>
                                </div>
                              )
                            })}
                          </div>
                        )}
                      </div>

                      <div className="border-t pt-4">
                        <h3 className="font-semibold mb-2">Usage Over Time</h3>
                        {(jwtSubDrilldown.traffic_over_time ?? []).length === 0 ? (
                          <p className="text-sm text-muted-foreground">No usage timeline available.</p>
                        ) : (
                          <div className="space-y-2">
                            {jwtSubDrilldown.traffic_over_time.slice(-12).map((row) => (
                              <div key={`jwt-day-${row.bucket}`} className="flex items-center justify-between text-sm">
                                <span className="text-muted-foreground">{new Date(row.bucket).toLocaleDateString()}</span>
                                <span>{row.requests.toLocaleString()} req{row.total_cost_usd ? ` • ${formatCurrency(row.total_cost_usd)}` : ''}</span>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    </>
                  )}
                </div>
              )}
            </SheetContent>
          </Sheet>
        </>
        )
      ) : viewMode === 'api_keys_raw_usage' ? (
        apiKeyRawUsageError ? (
          <SectionCard title="Unable to load raw API key usage">
            <p className="text-sm text-destructive">
              {apiKeyRawUsageError instanceof Error ? apiKeyRawUsageError.message : 'Request failed'}
            </p>
          </SectionCard>
        ) : (
        <>
          <SectionCard
            title="API Keys Raw Usage"
            description="Raw per-request activity attributed to API keys"
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const now = new Date()
                  const pad = (n: number) => String(n).padStart(2, '0')
                  const ts = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}`
                  const headers = ['timestamp', 'tenant_id', 'api_key_name', 'api_key_id', 'request_id', 'model', 'provider', 'status', 'latency_ms', 'cost_usd', 'prompt_tokens', 'cached_tokens', 'completion_tokens', 'total_tokens']
                  const lines = [headers.join(',')]
                  for (const row of apiKeyRawRows) {
                    const data = [
                      row.timestamp || '-',
                      row.tenant_id || '-',
                      row.api_key_name || '-',
                      row.api_key_id || '-',
                      row.request_id || '-',
                      row.model || '-',
                      row.provider || '-',
                      row.status || '-',
                      String(Number(row.latency_ms ?? 0)),
                      String(Number(row.cost_usd ?? 0)),
                      String(Number(row.prompt_tokens ?? 0)),
                      String(Number(row.cached_tokens ?? 0)),
                      String(Number(row.completion_tokens ?? 0)),
                      String(Number(row.total_tokens ?? 0)),
                    ]
                    lines.push(data.map((v) => (String(v).includes(',') ? `"${String(v).replace(/\"/g, '\"')}"` : String(v))).join(','))
                  }
                  const csv = lines.join('\n')
                  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
                  const url = URL.createObjectURL(blob)
                  const link = document.createElement('a')
                  link.href = url
                  link.download = `api_key_raw_usage_${ts}.csv`
                  document.body.appendChild(link)
                  link.click()
                  document.body.removeChild(link)
                  URL.revokeObjectURL(url)
                }}
                className="gap-2"
              >
                <Download className="h-4 w-4" />
                Export CSV
              </Button>
            }
          >
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-8 mb-4">
              <input
                type="datetime-local"
                value={rawFrom}
                onChange={(e) => {
                  setRawFrom(e.target.value)
                  setRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              />
              <input
                type="datetime-local"
                value={rawTo}
                onChange={(e) => {
                  setRawTo(e.target.value)
                  setRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              />
              <select
                value={rawTenant}
                onChange={(e) => {
                  setRawTenant(e.target.value)
                  setRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Tenants</option>
                {Array.from(new Set(usageSummariesVisible.map((u) => u.tenant_id))).sort().map((tenantId) => (
                  <option key={tenantId} value={tenantId}>{tenantId}</option>
                ))}
              </select>
              <select
                value={rawApiKeyName}
                onChange={(e) => {
                  setRawApiKeyName(e.target.value)
                  setRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All API Keys</option>
                {apiKeyRawNameOptions.map((apiKeyName) => (
                  <option key={apiKeyName} value={apiKeyName}>{apiKeyName}</option>
                ))}
              </select>
              <select
                value={rawModel}
                onChange={(e) => {
                  setRawModel(e.target.value)
                  setRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Models</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.model ?? '')).filter(Boolean))).sort().map((model) => (
                  <option key={model} value={model}>{model}</option>
                ))}
              </select>
              <select
                value={rawProvider}
                onChange={(e) => {
                  setRawProvider(e.target.value)
                  setRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Providers</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.provider ?? '')).filter(Boolean))).sort().map((provider) => (
                  <option key={provider} value={provider}>{provider}</option>
                ))}
              </select>
              <select
                value={rawStatus}
                onChange={(e) => {
                  setRawStatus(e.target.value)
                  setRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Status</option>
                <option value="ok">ok</option>
                <option value="error">error</option>
              </select>
              <Button
                variant="outline"
                onClick={() => {
                  setRawFrom(defaultRawFrom)
                  setRawTo(defaultRawTo)
                  setRawTenant('all')
                  setRawApiKeyName('all')
                  setRawModel('all')
                  setRawProvider('all')
                  setRawStatus('all')
                  setRawPage(0)
                }}
              >
                Reset
              </Button>
            </div>

            {!apiKeyRawUsageResponse?.endpoint_available && apiKeyRawUsageResponse?.error_message ? (
              <div className="mb-4 rounded-md border border-yellow-300 bg-yellow-50 px-3 py-2 text-sm text-yellow-800">
                Raw usage endpoint not available yet: {apiKeyRawUsageResponse.error_message}
              </div>
            ) : null}

            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-muted-foreground">
                  <tr>
                    <th className="py-2 pr-3">Timestamp</th>
                    <th className="py-2 pr-3">Tenant</th>
                    <th className="py-2 pr-3">API Key Name</th>
                    <th className="py-2 pr-3">API Key ID</th>
                    <th className="py-2 pr-3">Request ID</th>
                    <th className="py-2 pr-3">Model</th>
                    <th className="py-2 pr-3">Provider</th>
                    <th className="py-2 pr-3">Status</th>
                    <th className="py-2 pr-3">Latency</th>
                    <th className="py-2 pr-3">Cost</th>
                    <th className="py-2 pr-3">Prompt</th>
                    <th className="py-2 pr-3">Cached</th>
                    <th className="py-2 pr-3">Completion</th>
                    <th className="py-2 pr-3">Total</th>
                  </tr>
                </thead>
                <tbody>
                  {isLoadingApiKeyRawUsage ? (
                    <tr className="border-t"><td className="py-6 text-center" colSpan={14}><Skeleton className="h-6" /></td></tr>
                  ) : apiKeyRawRows.length === 0 ? (
                    <tr className="border-t"><td className="py-6 text-center text-muted-foreground" colSpan={14}>No API key request activity found in the selected range</td></tr>
                  ) : (
                    apiKeyRawRows.map((row) => (
                      <tr key={`${row.request_id}-${row.timestamp}`} className="border-t">
                        <td className="py-2 pr-3">{row.timestamp ? new Date(row.timestamp).toLocaleString() : '-'}</td>
                        <td className="py-2 pr-3">{row.tenant_id || '-'}</td>
                        <td className="py-2 pr-3">{row.api_key_name || '-'}</td>
                        <td className="py-2 pr-3">{row.api_key_id || '-'}</td>
                        <td className="py-2 pr-3">{row.request_id || '-'}</td>
                        <td className="py-2 pr-3">{row.model || '-'}</td>
                        <td className="py-2 pr-3">{row.provider || '-'}</td>
                        <td className="py-2 pr-3">
                          <Badge variant="outline" className={row.status === 'ok' ? 'border-green-300 text-green-700 bg-green-50' : 'border-red-300 text-red-700 bg-red-50'}>
                            {row.status || '-'}
                          </Badge>
                        </td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.latency_ms ?? 0).toLocaleString()} ms</td>
                        <td className="py-2 pr-3 tabular-nums">{formatCurrency(Number(row.cost_usd ?? 0))}</td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.prompt_tokens ?? 0).toLocaleString()}</td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.cached_tokens ?? 0).toLocaleString()}</td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.completion_tokens ?? 0).toLocaleString()}</td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.total_tokens ?? 0).toLocaleString()}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            {apiKeyRawPagination.total > 0 && (
              <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
                <div>
                  Showing {apiKeyRawPagination.offset + 1} to {Math.min(apiKeyRawPagination.offset + apiKeyRawPagination.returned, apiKeyRawPagination.total)} of {apiKeyRawPagination.total} requests
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => setRawPage(Math.max(0, rawPage - 1))} disabled={rawPage === 0}>
                    Previous
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setRawPage(rawPage + 1)} disabled={apiKeyRawPagination.offset + apiKeyRawPagination.returned >= apiKeyRawPagination.total}>
                    Next
                  </Button>
                </div>
              </div>
            )}
          </SectionCard>
        </>
        )
      ) : viewMode === 'jwt_sub_raw_usage' ? (
        jwtSubRawUsageError ? (
          <SectionCard title="Unable to load raw JWT subject usage">
            <p className="text-sm text-destructive">
              {jwtSubRawUsageError instanceof Error ? jwtSubRawUsageError.message : 'Request failed'}
            </p>
          </SectionCard>
        ) : (
        <>
          <SectionCard
            title="JWT Sub Raw Usage"
            description="Raw per-request activity attributed to JWT subjects"
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const now = new Date()
                  const pad = (n: number) => String(n).padStart(2, '0')
                  const ts = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}`
                  const headers = ['timestamp', 'tenant_id', 'jwt_sub', 'request_id', 'model', 'provider', 'status', 'latency_ms', 'cost_usd', 'prompt_tokens', 'cached_tokens', 'completion_tokens', 'total_tokens']
                  const lines = [headers.join(',')]
                  for (const row of jwtSubRawRows) {
                    const data = [
                      row.timestamp || '-',
                      row.tenant_id || '-',
                      row.jwt_sub || '-',
                      row.request_id || '-',
                      row.model || '-',
                      row.provider || '-',
                      row.status || '-',
                      String(Number(row.latency_ms ?? 0)),
                      String(Number(row.cost_usd ?? 0)),
                      String(Number(row.prompt_tokens ?? 0)),
                      String(Number(row.cached_tokens ?? 0)),
                      String(Number(row.completion_tokens ?? 0)),
                      String(Number(row.total_tokens ?? 0)),
                    ]
                    lines.push(data.map((v) => (String(v).includes(',') ? `"${String(v).replace(/\"/g, '\"')}"` : String(v))).join(','))
                  }
                  const csv = lines.join('\n')
                  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
                  const url = URL.createObjectURL(blob)
                  const link = document.createElement('a')
                  link.href = url
                  link.download = `jwt_sub_raw_usage_${ts}.csv`
                  document.body.appendChild(link)
                  link.click()
                  document.body.removeChild(link)
                  URL.revokeObjectURL(url)
                }}
                className="gap-2"
              >
                <Download className="h-4 w-4" />
                Export CSV
              </Button>
            }
          >
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-8 mb-4">
              <input
                type="datetime-local"
                value={jwtRawFrom}
                onChange={(e) => {
                  setJwtRawFrom(e.target.value)
                  setJwtRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              />
              <input
                type="datetime-local"
                value={jwtRawTo}
                onChange={(e) => {
                  setJwtRawTo(e.target.value)
                  setJwtRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              />
              <select
                value={jwtRawTenant}
                onChange={(e) => {
                  setJwtRawTenant(e.target.value)
                  setJwtRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Tenants</option>
                {Array.from(new Set(usageSummariesVisible.map((u) => u.tenant_id))).sort().map((tenantId) => (
                  <option key={tenantId} value={tenantId}>{tenantId}</option>
                ))}
              </select>
              <select
                value={jwtRawSub}
                onChange={(e) => {
                  setJwtRawSub(e.target.value)
                  setJwtRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All JWT Subs</option>
                {jwtSubRawOptions.map((jwtSub) => (
                  <option key={jwtSub} value={jwtSub}>{jwtSub}</option>
                ))}
              </select>
              <select
                value={jwtRawModel}
                onChange={(e) => {
                  setJwtRawModel(e.target.value)
                  setJwtRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Models</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.model ?? '')).filter(Boolean))).sort().map((model) => (
                  <option key={model} value={model}>{model}</option>
                ))}
              </select>
              <select
                value={jwtRawProvider}
                onChange={(e) => {
                  setJwtRawProvider(e.target.value)
                  setJwtRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Providers</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.provider ?? '')).filter(Boolean))).sort().map((provider) => (
                  <option key={provider} value={provider}>{provider}</option>
                ))}
              </select>
              <select
                value={jwtRawStatus}
                onChange={(e) => {
                  setJwtRawStatus(e.target.value)
                  setJwtRawPage(0)
                }}
                className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="all">All Status</option>
                <option value="ok">ok</option>
                <option value="error">error</option>
              </select>
              <Button
                variant="outline"
                onClick={() => {
                  setJwtRawFrom(defaultRawFrom)
                  setJwtRawTo(defaultRawTo)
                  setJwtRawTenant('all')
                  setJwtRawSub('all')
                  setJwtRawModel('all')
                  setJwtRawProvider('all')
                  setJwtRawStatus('all')
                  setJwtRawPage(0)
                }}
              >
                Reset
              </Button>
            </div>

            {!jwtSubRawUsageResponse?.endpoint_available && jwtSubRawUsageResponse?.error_message ? (
              <div className="mb-4 rounded-md border border-yellow-300 bg-yellow-50 px-3 py-2 text-sm text-yellow-800">
                Raw usage endpoint not available yet: {jwtSubRawUsageResponse.error_message}
              </div>
            ) : null}

            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-muted-foreground">
                  <tr>
                    <th className="py-2 pr-3">Timestamp</th>
                    <th className="py-2 pr-3">Tenant</th>
                    <th className="py-2 pr-3">JWT Sub</th>
                    <th className="py-2 pr-3">Request ID</th>
                    <th className="py-2 pr-3">Model</th>
                    <th className="py-2 pr-3">Provider</th>
                    <th className="py-2 pr-3">Status</th>
                    <th className="py-2 pr-3">Latency</th>
                    <th className="py-2 pr-3">Cost</th>
                    <th className="py-2 pr-3">Prompt</th>
                    <th className="py-2 pr-3">Cached</th>
                    <th className="py-2 pr-3">Completion</th>
                    <th className="py-2 pr-3">Total</th>
                  </tr>
                </thead>
                <tbody>
                  {isLoadingJwtSubRawUsage ? (
                    <tr className="border-t"><td className="py-6 text-center" colSpan={13}><Skeleton className="h-6" /></td></tr>
                  ) : jwtSubRawRows.length === 0 ? (
                    <tr className="border-t"><td className="py-6 text-center text-muted-foreground" colSpan={13}>No JWT subject request activity found in the selected range</td></tr>
                  ) : (
                    jwtSubRawRows.map((row) => (
                      <tr key={`${row.request_id}-${row.timestamp}`} className="border-t">
                        <td className="py-2 pr-3">{row.timestamp ? new Date(row.timestamp).toLocaleString() : '-'}</td>
                        <td className="py-2 pr-3">{row.tenant_id || '-'}</td>
                        <td className="py-2 pr-3">{row.jwt_sub || '-'}</td>
                        <td className="py-2 pr-3">{row.request_id || '-'}</td>
                        <td className="py-2 pr-3">{row.model || '-'}</td>
                        <td className="py-2 pr-3">{row.provider || '-'}</td>
                        <td className="py-2 pr-3">
                          <Badge variant="outline" className={row.status === 'ok' ? 'border-green-300 text-green-700 bg-green-50' : 'border-red-300 text-red-700 bg-red-50'}>
                            {row.status || '-'}
                          </Badge>
                        </td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.latency_ms ?? 0).toLocaleString()} ms</td>
                        <td className="py-2 pr-3 tabular-nums">{formatCurrency(Number(row.cost_usd ?? 0))}</td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.prompt_tokens ?? 0).toLocaleString()}</td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.cached_tokens ?? 0).toLocaleString()}</td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.completion_tokens ?? 0).toLocaleString()}</td>
                        <td className="py-2 pr-3 tabular-nums">{Number(row.total_tokens ?? 0).toLocaleString()}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            {jwtSubRawPagination.total > 0 && (
              <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
                <div>
                  Showing {jwtSubRawPagination.offset + 1} to {Math.min(jwtSubRawPagination.offset + jwtSubRawPagination.returned, jwtSubRawPagination.total)} of {jwtSubRawPagination.total} requests
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => setJwtRawPage(Math.max(0, jwtRawPage - 1))} disabled={jwtRawPage === 0}>
                    Previous
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setJwtRawPage(jwtRawPage + 1)} disabled={jwtSubRawPagination.offset + jwtSubRawPagination.returned >= jwtSubRawPagination.total}>
                    Next
                  </Button>
                </div>
              </div>
            )}
          </SectionCard>
        </>
        )
      ) : viewMode === 'anomalies' ? (
        anomaliesListError || anomalyStatsError ? (
          <SectionCard title="Unable to load anomalies">
            <p className="text-sm text-destructive">
              {(anomaliesListError instanceof Error ? anomaliesListError.message : null) ||
                (anomalyStatsError instanceof Error ? anomalyStatsError.message : null) ||
                'Request failed'}
            </p>
          </SectionCard>
        ) : (
        <>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-6">
            {isLoadingAnomStats ? (
              <>
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
              </>
            ) : (
              <>
                <StatCard title="Active Anomalies" value={anomalyStats?.summary?.active_anomalies ?? 0} icon={AlertTriangle} description={`${windowHours}h window`} />
                <StatCard title="Cost Spike (24h)" value={formatCurrency(anomalyStats?.summary?.cost_spike_24h_usd ?? 0)} icon={DollarSign} description="Above expected" />
                <StatCard title="Affected Tenants" value={anomalyStats?.summary?.affected_tenants ?? 0} icon={Wallet} description="Distinct tenants" />
                <StatCard title="Affected Models" value={anomalyStats?.summary?.affected_models ?? 0} icon={TableIcon} description="Distinct models" />
              </>
            )}
          </div>

          <SectionCard title="Filters">
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-6">
              <select value={anomalyTenant} onChange={(e) => setAnomalyTenant(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Tenants</option>
                {Array.from(new Set(usageSummariesVisible.map((u) => u.tenant_id))).sort().map((tenantId) => (
                  <option key={tenantId} value={tenantId}>{tenantId}</option>
                ))}
              </select>
              <select value={anomalyProvider} onChange={(e) => setAnomalyProvider(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Providers</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.provider ?? '')).filter(Boolean))).sort().map((provider) => (
                  <option key={provider} value={provider}>{provider}</option>
                ))}
              </select>
              <select value={anomalyModel} onChange={(e) => setAnomalyModel(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Models</option>
                {Array.from(new Set(modelPerf.map((m) => String(m.model ?? '')).filter(Boolean))).sort().map((model) => (
                  <option key={model} value={model}>{model}</option>
                ))}
              </select>
              <select value={anomalyStatus} onChange={(e) => setAnomalyStatus(e.target.value)} className="rounded-md border border-input bg-background px-3 py-2 text-sm">
                <option value="all">All Status</option>
                <option value="open">open</option>
                <option value="closed">closed</option>
              </select>
              <Button variant="outline" onClick={() => { setAnomalyTenant('all'); setAnomalyProvider('all'); setAnomalyModel('all'); setAnomalyStatus('all'); setAnomalyPage(0) }}>Reset</Button>
              <div />
            </div>
          </SectionCard>

          <div className="grid gap-4 md:grid-cols-2 mb-6">
            <SectionCard title="Cost Anomaly Timeline">
              {isLoadingAnomStats ? (
                <Skeleton className="h-48" />
              ) : (anomalyStats?.timeline ?? []).length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No timeline data available</div>
              ) : (
                <div className="space-y-3">
                  {(anomalyStats?.timeline ?? []).map((t) => (
                    <div key={t.bucket} className="flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">{new Date(t.bucket).toLocaleString()}</span>
                      <span className="tabular-nums">{t.anomalies} anomalies</span>
                    </div>
                  ))}
                </div>
              )}
            </SectionCard>
            <SectionCard title="Top Anomalous Tenants">
              {isLoadingAnomStats ? (
                <Skeleton className="h-48" />
              ) : (anomalyStats?.top_tenants ?? []).length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No tenant data available</div>
              ) : (
                <div className="space-y-3">
                  {(anomalyStats?.top_tenants ?? []).map((x) => (
                    <div key={x.tenant_id} className="flex items-center justify-between text-sm">
                      <span className="font-medium">{x.tenant_id}</span>
                      <span className="tabular-nums text-muted-foreground">{x.anomalies}</span>
                    </div>
                  ))}
                </div>
              )}
            </SectionCard>
          </div>

          <SectionCard
            title="Anomalies"
            action={
              <Button variant="outline" size="sm" onClick={() => {
                const now = new Date()
                const pad = (n: number) => String(n).padStart(2, '0')
                const ts = `${now.getFullYear()}${pad(now.getMonth()+1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}`
                const headers = ['timestamp','tenant_id','model','provider','expected_cost_usd','observed_cost_usd','deviation_pct','anomaly_type','status']
                const lines = [headers.join(',')]
                for (const a of anomalies) {
                  const row = [a.timestamp, a.tenant_id, a.model || '-', a.provider || '-', a.expected_cost_usd.toFixed(6), a.observed_cost_usd.toFixed(6), a.deviation_pct.toFixed(2), a.anomaly_type, a.status]
                  lines.push(row.map((v) => (String(v).includes(',') ? `"${String(v).replace(/\"/g,'\"')}"` : String(v))).join(','))
                }
                const csv = lines.join('\n')
                const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
                const url = URL.createObjectURL(blob)
                const link = document.createElement('a')
                link.href = url
                link.download = `anomaly_detection_${ts}.csv`
                document.body.appendChild(link)
                link.click()
                document.body.removeChild(link)
                URL.revokeObjectURL(url)
              }} className="gap-2">
                <Download className="h-4 w-4" />
                Export CSV
              </Button>
            }
          >
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-muted-foreground">
                  <tr>
                    <th className="py-2 pr-3">Timestamp</th>
                    <th className="py-2 pr-3">Tenant</th>
                    <th className="py-2 pr-3">Model</th>
                    <th className="py-2 pr-3">Provider</th>
                    <th className="py-2 pr-3">Expected</th>
                    <th className="py-2 pr-3">Observed</th>
                    <th className="py-2 pr-3">Deviation</th>
                    <th className="py-2 pr-3">Type</th>
                    <th className="py-2 pr-3">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {isLoadingAnoms ? (
                    <tr className="border-t"><td className="py-6 text-center" colSpan={9}><Skeleton className="h-6" /></td></tr>
                  ) : !anomalies || anomalies.length === 0 ? (
                    <tr className="border-t"><td className="py-6 text-center text-muted-foreground" colSpan={9}>No anomalies detected in the selected window</td></tr>
                  ) : (
                    anomalies.slice().sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()).map((a, idx) => (
                      <tr key={a.anomaly_id ?? `anomaly-${idx}`} className="border-t cursor-pointer hover:bg-muted/40" onClick={() => { setSelectedAnomaly(a); setAnomalyDrawerOpen(true) }}>
                        <td className="py-2 pr-3">{new Date(a.timestamp).toLocaleString()}</td>
                        <td className="py-2 pr-3">{a.tenant_id}</td>
                        <td className="py-2 pr-3">{a.model || '-'}</td>
                        <td className="py-2 pr-3">{a.provider || '-'}</td>
                        <td className="py-2 pr-3 tabular-nums">{formatCurrency(a.expected_cost_usd)}</td>
                        <td className="py-2 pr-3 tabular-nums">{formatCurrency(a.observed_cost_usd)}</td>
                        <td className="py-2 pr-3 tabular-nums" title="Observed vs expected">{a.deviation_pct.toFixed(2)}%</td>
                        <td className="py-2 pr-3"><Badge variant="secondary">{a.anomaly_type}</Badge></td>
                        <td className="py-2 pr-3"><Badge variant={a.status === 'open' ? 'destructive' : a.status === 'resolved' ? 'default' : 'secondary'}>{a.status}</Badge></td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            {anomalyPagination.total > 0 && (
              <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
                <div>
                  Showing {anomalyPagination.offset + 1} to {Math.min(anomalyPagination.offset + anomalyPagination.returned, anomalyPagination.total)} of {anomalyPagination.total} anomalies
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setAnomalyPage(Math.max(0, anomalyPage - 1))}
                    disabled={anomalyPage === 0}
                  >
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setAnomalyPage(anomalyPage + 1)}
                    disabled={anomalyPagination.offset + anomalyPagination.returned >= anomalyPagination.total}
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </SectionCard>

          <Sheet open={anomalyDrawerOpen} onOpenChange={setAnomalyDrawerOpen}>
            <SheetContent className="w-full sm:max-w-xl overflow-y-auto">
              <SheetHeader>
                <SheetTitle>Anomaly Details</SheetTitle>
                <SheetDescription>
                  {selectedAnomaly?.anomaly_id ?? 'Anomaly details'}
                </SheetDescription>
              </SheetHeader>
              {selectedAnomaly ? (
                <div className="mt-6 space-y-4 text-sm">
                  {selectedAnomaly.anomaly_id && (
                    <div>
                      <span className="text-muted-foreground">Anomaly ID:</span>
                      <div className="mt-1 font-mono text-xs">{selectedAnomaly.anomaly_id}</div>
                    </div>
                  )}
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <span className="text-muted-foreground block mb-1">Timestamp</span>
                      <span className="font-medium">{new Date(selectedAnomaly.timestamp).toLocaleString()}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground block mb-1">Tenant</span>
                      <span className="font-medium">{selectedAnomaly.tenant_id}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground block mb-1">Model</span>
                      <span className="font-medium">{selectedAnomaly.model || '-'}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground block mb-1">Provider</span>
                      <span className="font-medium">{selectedAnomaly.provider || '-'}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground block mb-1">Expected Cost</span>
                      <span className="font-medium tabular-nums">{formatCurrency(selectedAnomaly.expected_cost_usd)}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground block mb-1">Observed Cost</span>
                      <span className="font-medium tabular-nums">{formatCurrency(selectedAnomaly.observed_cost_usd)}</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground block mb-1">Deviation</span>
                      <span className="font-medium tabular-nums">{selectedAnomaly.deviation_pct.toFixed(2)}%</span>
                    </div>
                    <div>
                      <span className="text-muted-foreground block mb-1">Type</span>
                      <Badge variant="secondary">{selectedAnomaly.anomaly_type}</Badge>
                    </div>
                    <div>
                      <span className="text-muted-foreground block mb-1">Status</span>
                      <Badge variant={selectedAnomaly.status === 'open' ? 'destructive' : selectedAnomaly.status === 'resolved' ? 'default' : 'secondary'}>{selectedAnomaly.status}</Badge>
                    </div>
                  </div>

                  {/* Root Cause Analysis */}
                  <div className="mt-6 pt-6 border-t">
                    <h3 className="text-sm font-semibold mb-3">Root Cause Analysis</h3>
                    {isLoadingExplain ? (
                      <div className="text-sm text-muted-foreground">Loading analysis...</div>
                    ) : (() => {
                      const matchedExplain = anomalyExplainData?.data?.find(
                        (e) => e.tenant_id === selectedAnomaly.tenant_id
                      )
                      if (!matchedExplain || !matchedExplain.top_drivers) {
                        return <div className="text-sm text-muted-foreground">No driver breakdown available</div>
                      }
                      const { models, providers, api_keys } = matchedExplain.top_drivers
                      const formatDelta = (v: number) =>
                        new Intl.NumberFormat(undefined, {
                          style: 'currency',
                          currency: 'USD',
                          minimumFractionDigits: 4,
                          maximumFractionDigits: 6,
                        }).format(v)
                      const hasData = (models?.length ?? 0) > 0 || (providers?.length ?? 0) > 0 || (api_keys?.length ?? 0) > 0
                      if (!hasData) {
                        return <div className="text-sm text-muted-foreground">No driver breakdown available</div>
                      }
                      return (
                        <div className="space-y-4 text-sm">
                          {models && models.length > 0 && (
                            <div>
                              <div className="text-muted-foreground font-medium mb-2">Top Models</div>
                              <ul className="space-y-1">
                                {models.map((m, i) => (
                                  <li key={i} className="flex items-center justify-between">
                                    <span>{m.label || (m as { model?: string }).model || 'unknown'}</span>
                                    <span className="font-mono text-xs text-green-600">+{formatDelta(m.delta_spend)}</span>
                                  </li>
                                ))}
                              </ul>
                            </div>
                          )}
                          {providers && providers.length > 0 && (
                            <div>
                              <div className="text-muted-foreground font-medium mb-2">Top Providers</div>
                              <ul className="space-y-1">
                                {providers.map((p, i) => (
                                  <li key={i} className="flex items-center justify-between">
                                    <span>{p.label || (p as { provider?: string }).provider || 'unknown'}</span>
                                    <span className="font-mono text-xs text-green-600">+{formatDelta(p.delta_spend)}</span>
                                  </li>
                                ))}
                              </ul>
                            </div>
                          )}
                          {api_keys && api_keys.length > 0 && (
                            <div>
                              <div className="text-muted-foreground font-medium mb-2">Top API Keys</div>
                              <ul className="space-y-1">
                                {api_keys.map((k, i) => (
                                  <li key={i} className="flex items-center justify-between">
                                    <span>{(
                                      (k.label && k.label.trim()) ||
                                      (k as { api_key_name?: string }).api_key_name ||
                                      'unknown'
                                    )}</span>
                                    <span className="font-mono text-xs text-green-600">+{formatDelta(k.delta_spend)}</span>
                                  </li>
                                ))}
                              </ul>
                            </div>
                          )}
                        </div>
                      )
                    })()}
                  </div>
                </div>
              ) : null}
            </SheetContent>
          </Sheet>
        </>
        )
      ) : (
        <>
          {/* FinOps Dashboard View */}
          {tenantStatsQuery.isError && tenantRequestStatsError ? (
            <SectionCard title="Tenant activity">
              <p className="text-sm text-destructive">
                {tenantRequestStatsError instanceof Error
                  ? tenantRequestStatsError.message
                  : 'Failed to load per-tenant request stats'}
              </p>
            </SectionCard>
          ) : null}
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-6">
            {dashboardViewLoading ? (
              <>
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
                <Skeleton className="h-32" />
              </>
            ) : (
              <>
                <StatCard
                  title="Total Spend (month)"
                  value={formatCurrency(finopsDashboardKpis.totalSpendMonth)}
                  icon={DollarSign}
                  description={selectedMonth}
                />
                <StatCard
                  title="Total Budget"
                  value={formatCurrency(finopsDashboardKpis.totalBudgetUsd)}
                  icon={Wallet}
                  description="Configured budgets"
                />
                <StatCard
                  title="Budget Utilization"
                  value={formatSmallPercent(finopsDashboardKpis.utilizationPct)}
                  icon={TrendingDown}
                  description="Spend / Budget"
                />
                <StatCard
                  title="Tenants At Risk / Over"
                  value={`${finopsDashboardKpis.tenantsAtRisk} / ${finopsDashboardKpis.tenantsOver}`}
                  icon={AlertCircle}
                  description="warning / exceeded"
                />
              </>
            )}
          </div>

          <div className="grid gap-6 md:grid-cols-2 mb-6">
            <SectionCard title="Spend by Tenant (month)">
              {dashboardViewLoading ? (
                <Skeleton className="h-48" />
              ) : finopsDashboardKpis.spendByTenantRows.length === 0 ? (
                <div className="py-8 text-center text-sm text-muted-foreground">No tenant spend data available</div>
              ) : (
                <div className="space-y-3">
                  {(() => {
                    const rows = finopsDashboardKpis.spendByTenantRows
                    const max = Math.max(...rows.map((x) => x.spend), 0) || 1
                    return rows.map((row) => {
                      const width = Math.max((row.spend / max) * 100, 2)
                      return (
                        <div key={row.tenant_id} className="space-y-1">
                          <div className="flex items-center justify-between text-sm">
                            <span className="font-medium">{row.tenant_id}</span>
                            <span className="text-muted-foreground">{formatCurrency(row.spend)}</span>
                          </div>
                          <div className="h-2 w-full rounded-full bg-muted">
                            <div className="h-2 rounded-full bg-primary" style={{ width: `${width}%` }} />
                          </div>
                        </div>
                      )
                    })
                  })()}
                </div>
              )}
            </SectionCard>

            <SectionCard title="Spend by Model (month)">
              {dashboardModelAggregationQuery.isLoading ? (
                <Skeleton className="h-48" />
              ) : dashboardModelAggregationQuery.isError ? (
                <div className="text-sm text-destructive">Failed to load model spend data.</div>
              ) : dashboardModelRows.length === 0 ? (
                <div className="text-sm text-muted-foreground">No model cost data available</div>
              ) : (
                <div className="space-y-3">
                  {(() => {
                    const rows = [...dashboardModelRows].sort((a, b) => b.effective_spend - a.effective_spend).slice(0, 20)
                    const max = Math.max(...rows.map((r) => r.effective_spend), 0) || 1
                    return rows.map((row) => {
                      const width = Math.max((row.effective_spend / max) * 100, 2)
                      return (
                        <div key={row.model} className="space-y-1">
                          <div className="flex items-center justify-between text-sm">
                            <span className="font-medium">{row.model}</span>
                            <span className="text-muted-foreground">{formatCurrency(row.effective_spend)}</span>
                          </div>
                          <div className="h-2 w-full rounded-full bg-muted">
                            <div className="h-2 rounded-full bg-primary" style={{ width: `${width}%` }} />
                          </div>
                        </div>
                      )
                    })
                  })()}
                </div>
              )}
            </SectionCard>
          </div>

          <div className="mb-6">
            <SectionCard
              title="Model Cost Efficiency (month)"
              action={
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleExportModelEfficiency}
                    className="gap-2"
                  >
                    <Download className="h-4 w-4" />
                    Export CSV
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setExplainOpen(true)}
                    className="gap-2"
                  >
                    <HelpCircle className="h-4 w-4" />
                    Explain
                  </Button>
                </div>
              }
            >
              {dashboardModelAggregationQuery.isLoading ? (
                <Skeleton className="h-64" />
              ) : dashboardModelAggregationQuery.isError ? (
                <p className="text-sm text-destructive">Failed to load model cost efficiency.</p>
              ) : dashboardModelRows.length === 0 ? (
                <p className="text-sm text-muted-foreground">No model cost data available</p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b">
                        <th className="py-2 px-3 text-left font-medium">Model</th>
                        <th className="py-2 px-3 text-right font-medium">Requests</th>
                        <th className="py-2 px-3 text-right font-medium">Effective Spend</th>
                        <th className="py-2 px-3 text-right font-medium">Avg Cost / Request</th>
                        <th className="py-2 px-3 text-left font-medium">Provider</th>
                        <th className="py-2 px-3 text-left font-medium">Model Type</th>
                      </tr>
                    </thead>
                    <tbody>
                      {[...dashboardModelRows]
                        .sort((a, b) => b.effective_spend - a.effective_spend)
                        .slice(0, 15)
                        .map((row) => (
                          <tr key={row.model} className="border-b last:border-0">
                            <td className="py-2 px-3">{row.model}</td>
                            <td className="py-2 px-3 text-right tabular-nums">{row.requests.toLocaleString()}</td>
                            <td className="py-2 px-3 text-right tabular-nums">{formatCurrency(row.effective_spend)}</td>
                            <td className="py-2 px-3 text-right tabular-nums">{formatAvgCostUsd(row.avg_cost_per_request)}</td>
                            <td className="py-2 px-3">{row.provider ?? '-'}</td>
                            <td className="py-2 px-3">{row.model_type ?? '-'}</td>
                          </tr>
                        ))}
                    </tbody>
                  </table>
                </div>
              )}
            </SectionCard>
          </div>

          <Sheet open={explainOpen} onOpenChange={setExplainOpen}>
            <SheetContent className="w-full sm:max-w-xl overflow-y-auto">
              <SheetHeader>
                <SheetTitle>Model Cost Efficiency — Explanation</SheetTitle>
                <SheetDescription>
                  Understanding how this table is built and what the data means
                </SheetDescription>
              </SheetHeader>

              <div className="mt-6 space-y-6">
                <div>
                  <div className="flex items-center gap-2 mb-3">
                    <Database className="h-5 w-5 text-primary" />
                    <h3 className="font-semibold text-base">What this table shows</h3>
                  </div>
                  <div className="text-sm text-muted-foreground space-y-2">
                    <p>This table aggregates model usage for the selected month using <strong>effective spend</strong> from monetization data.</p>
                    <ul className="ml-6 list-disc space-y-1">
                      <li>Rows are grouped by model and summed across API keys</li>
                      <li>Effective spend and requests come from API key monetization drilldowns</li>
                      <li>Avg cost per request = effective spend / requests</li>
                    </ul>
                    <p className="mt-3">Provider and model type are added from the model catalog when available.</p>
                  </div>
                </div>

                <div className="border-t pt-6">
                  <div className="flex items-center gap-2 mb-3">
                    <Zap className="h-5 w-5 text-primary" />
                    <h3 className="font-semibold text-base">Data Sources</h3>
                  </div>
                  <div className="text-sm text-muted-foreground space-y-4">
                    <div>
                      <p className="font-medium text-foreground mb-2">Budget overview (effective tenant spend):</p>
                      <code className="block bg-muted px-3 py-2 rounded text-xs mb-2">
                        GET /api/budgets/overview
                      </code>
                      <p className="mb-1">
                        Powers Total Spend, Spend by Tenant, and Budget Risk when the gateway returns overview data; otherwise the UI falls back to per-tenant budget status.
                      </p>
                    </div>
                    <div>
                      <p className="font-medium text-foreground mb-2">API key monetization list:</p>
                      <code className="block bg-muted px-3 py-2 rounded text-xs mb-2">
                        GET /api/finops/api-keys/usage?window_hours=&#123;window&#125;
                      </code>
                      <p className="mb-1">Used to discover API keys for the selected window.</p>
                    </div>
                    <div>
                      <p className="font-medium text-foreground mb-2">API key drilldown by model:</p>
                      <code className="block bg-muted px-3 py-2 rounded text-xs mb-2">
                        GET /api/finops/api-keys/&#123;api_key_id&#125;/usage?window_hours=&#123;window&#125;
                      </code>
                      <p className="mb-1">
                        Each row includes <code className="text-xs">effective_spend</code> (summed per model); token-only{' '}
                        <code className="text-xs">spend</code> is not used for these charts.
                      </p>
                    </div>
                    <div>
                      <p className="font-medium text-foreground mb-2">Model catalog:</p>
                      <code className="block bg-muted px-3 py-2 rounded text-xs mb-2">
                        GET /api/models
                      </code>
                      <p className="mb-1">Provides provider and model type metadata.</p>
                    </div>
                  </div>
                </div>

                <div className="border-t pt-6">
                  <div className="flex items-center gap-2 mb-3">
                    <Info className="h-5 w-5 text-primary" />
                    <h3 className="font-semibold text-base">Why some fields may appear empty</h3>
                  </div>
                  <div className="text-sm text-muted-foreground space-y-2">
                    <p>
                      If a model appears in monetization data but <strong>does not exist in the model catalog</strong>, the UI still shows the model row.
                    </p>
                    <p>In that case the following fields will appear as &quot;-&quot;:</p>
                    <ul className="ml-6 list-disc space-y-1">
                      <li>provider</li>
                      <li>model type</li>
                    </ul>
                    <p className="mt-3">
                      This means the model <strong>has usage but is not registered in the model catalog</strong>.
                    </p>
                  </div>
                </div>
              </div>
            </SheetContent>
          </Sheet>

          <SectionCard title="Budget Risk Table (month)">
            {dashboardViewLoading ? (
              <Skeleton className="h-64" />
            ) : (
              (() => {
                const sorted = [...finopsDashboardKpis.riskRows].sort((a, b) => b.utilization_pct - a.utilization_pct)

                const exportCsv = () => {
                  const headers = ['generated_at','month','tenant_id','current_spend_usd','budget_usd','utilization_pct','status','enforcement_mode','warn_pct','hard_pct']
                  const now = new Date().toISOString()
                  const escapeCell = (v: string) => '"' + v.replace(/"/g, '""') + '"'
                  const body = sorted.map((r) => [now, selectedMonth, r.tenant_id, String(r.current_spend_usd), String(r.budget_usd), String(r.utilization_pct), r.status, String(r.enforcement_mode ?? ''), String(r.warn_pct ?? ''), String(r.hard_pct ?? '')])
                  const csv = [headers.map(escapeCell).join(','), ...body.map((line) => line.map((c) => escapeCell(c)).join(','))].join('\n')
                  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
                  const url = URL.createObjectURL(blob)
                  const link = document.createElement('a')
                  link.href = url
                  link.download = `finops_budget_risk_${selectedMonth}.csv`
                  link.click()
                  URL.revokeObjectURL(url)
                }

                return (
                  <div className="space-y-3">
                    <div className="flex justify-end">
                      <Button variant="outline" size="sm" onClick={exportCsv}>
                        <Download className="mr-2 h-4 w-4" /> Export CSV
                      </Button>
                    </div>
                    <div className="border rounded-md overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead className="text-left text-muted-foreground">
                          <tr>
                            <th className="py-2 px-3">Tenant</th>
                            <th className="py-2 px-3">Current Spend</th>
                            <th className="py-2 px-3">Budget</th>
                            <th className="py-2 px-3">Utilization %</th>
                            <th className="py-2 px-3">Status</th>
                            <th className="py-2 px-3">Mode</th>
                            <th className="py-2 px-3">Warn Pct</th>
                            <th className="py-2 px-3">Hard Pct</th>
                          </tr>
                        </thead>
                        <tbody>
                          {sorted.map((r) => (
                            <tr key={r.tenant_id} className="border-t">
                              <td className="py-2 px-3 font-medium">{r.tenant_id}</td>
                              <td className="py-2 px-3 tabular-nums">{formatCurrency(r.current_spend_usd)}</td>
                              <td className="py-2 px-3 tabular-nums">{formatCurrency(r.budget_usd)}</td>
                              <td className="py-2 px-3 tabular-nums">{r.utilization_pct.toFixed(1)}%</td>
                              <td className="py-2 px-3">{r.status}</td>
                              <td className="py-2 px-3">{r.enforcement_mode ?? '—'}</td>
                              <td className="py-2 px-3">{r.warn_pct ?? '—'}</td>
                              <td className="py-2 px-3">{r.hard_pct ?? '—'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )
              })()
            )}
          </SectionCard>
        </>
      )}

    </div>
    </RequireAdminRole>
  )
}
