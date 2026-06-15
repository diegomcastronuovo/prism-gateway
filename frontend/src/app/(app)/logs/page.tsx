'use client'

import { Fragment, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { Download, ShieldAlert } from 'lucide-react'
import { useAuth } from '@/hooks/use-auth'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { useToast } from '@/hooks/use-toast'
import { useTenants } from '@/features/tenants/api/use-tenants'

type LogsTab = 'audit' | 'compliance' | 'conversation' | 'ml'

interface PaginationState {
  limit: number
  offset: number
  returned: number
  total: number
}

interface LogsResponse<T> {
  data: T[]
  pagination?: Partial<PaginationState>
}

interface AuditLogEntry {
  ts?: string
  timestamp?: string
  tenant_id?: string
  model?: string
  provider?: string
  strategy?: string
  status?: string
  latency_ms?: number
  request_id?: string
  actor?: string
  api_key_id?: string
  api_key_name?: string
  jwt_sub?: string
  decision?: string
  decision_reason?: string
  error_type?: string
  error?: string
  fallback_used?: boolean
  routing_snapshot?: Record<string, unknown> | null
  decision_snapshot?: Record<string, unknown> | null
}

interface ComplianceEventEntry {
  id?: string
  tenant_id?: string
  request_id?: string
  event_type?: string
  action_taken?: string
  metadata?: Record<string, unknown> | null
  created_at?: string
}

interface ConversationLogEntry {
  id?: string
  request_id?: string
  tenant_id?: string
  jwt_sub?: string
  workflow_id?: string
  conversation_id?: string
  customer_id?: string | null
  prompt_preview?: string
  response_preview?: string
  prompt_redacted?: string | null
  response_redacted?: string | null
  prompt_full?: string | null
  response_full?: string | null
  pii_detected?: boolean
  logging_mode?: string
  created_at?: string
}

interface MlLogEntry {
  timestamp?: string
  tenant_id?: string
  model?: string
  provider?: string
  strategy?: string
  status?: string
  latency_ms?: number
  request_id?: string
  api_key_name?: string
  jwt_sub?: string
  metadata?: {
    observable?: Record<string, unknown>
  } | null
}

interface AuditFilters {
  from: string
  to: string
  tenant_id: string
  jwt_sub: string
  status: string
  limit: number
  offset: number
}

interface ComplianceFilters {
  from: string
  to: string
  limit: number
  offset: number
}

interface ConversationFilters {
  from: string
  to: string
  tenant_id: string
  jwt_sub: string
  limit: number
  offset: number
}

interface MlLogFilters {
  from: string
  to: string
  tenant_id: string
  model: string
  status: string
  limit: number
  offset: number
}

const complianceEventTypes = [
  'pii_detected',
  'pii_blocked',
  'regex_block',
  'budget_applied',
  'fallback_triggered',
  'model_changed',
  'logs_accessed',
  'csv_exported',
]

function toInputValue(date: Date) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return local.toISOString().slice(0, 16)
}

function toRfc3339(value: string) {
  if (!value) return ''
  const date = new Date(value)
  return date.toISOString()
}

function formatTimestamp(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return date.toLocaleString()
}

function truncateText(value: string | undefined | null, max = 120) {
  if (!value) return '—'
  if (value.length <= max) return value
  return `${value.slice(0, max - 3)}...`
}

function previewJson(value: Record<string, unknown> | null | undefined) {
  if (!value || Object.keys(value).length === 0) return '—'
  return truncateText(JSON.stringify(value), 160)
}

function formatLatencyMs(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return `${Math.round(value)} ms`
}

function previewObservable(observable?: Record<string, unknown> | null) {
  if (!observable || Object.keys(observable).length === 0) return '—'
  const entries = Object.entries(observable)
  if (entries.length > 2) {
    return `${entries.length} fields`
  }
  return entries
    .map(([key, value]) => {
      if (value && typeof value === 'object') return `${key}={...}`
      return `${key}=${String(value)}`
    })
    .join(', ')
}

function getStatusBadgeClass(status?: string) {
  const normalized = (status || '').toLowerCase()
  if (normalized === 'ok' || normalized === 'success') {
    return 'bg-green-100 text-green-800 border-green-200 hover:bg-green-100'
  }
  if (normalized === 'error') {
    return 'bg-red-100 text-red-800 border-red-200 hover:bg-red-100'
  }
  return ''
}

function buildPagination<T>(data: LogsResponse<T> | undefined, fallback: { limit: number; offset: number }) {
  const rows = data?.data ?? []
  const pagination = data?.pagination ?? {}
  const limit = Number(pagination.limit ?? fallback.limit)
  const offset = Number(pagination.offset ?? fallback.offset)
  const returned = Number(pagination.returned ?? rows.length)
  const total = Number(pagination.total ?? offset + returned)
  return { limit, offset, returned, total }
}

function resolveExportFilename(contentDisposition: string | null): string | null {
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

function getExportUrl(tab: LogsTab, params: URLSearchParams) {
  switch (tab) {
    case 'audit':
      return `/api/logs/audit/export?${params.toString()}`
    case 'compliance':
      return `/api/logs/compliance/export?${params.toString()}`
    case 'conversation':
      return `/api/logs/conversations/export?${params.toString()}`
    case 'ml':
      return `/api/logs/ml/export?${params.toString()}`
    default: {
      const neverTab: never = tab
      throw new Error(`Unknown tab: ${neverTab}`)
    }
  }
}

export default function LogsPage() {
  const { user, isRefreshingSession, session, logout } = useAuth()
  const { toast } = useToast()
  const tenantsQuery = useTenants()
  const tenants = useMemo(() => {
    const items = tenantsQuery.data ?? []
    return [...items].sort((a, b) => a.tenant_id.localeCompare(b.tenant_id))
  }, [tenantsQuery.data])
  const [activeTab, setActiveTab] = useState<LogsTab>('audit')

  const now = useMemo(() => new Date(), [])
  const defaultTo = useMemo(() => toInputValue(now), [now])
  const defaultFrom = useMemo(() => toInputValue(new Date(now.getTime() - 24 * 60 * 60 * 1000)), [now])

  const [auditFilters, setAuditFilters] = useState<AuditFilters>({
    from: defaultFrom,
    to: defaultTo,
    tenant_id: '',
    jwt_sub: '',
    status: 'ok',
    limit: 50,
    offset: 0,
  })
  const [complianceFilters, setComplianceFilters] = useState<ComplianceFilters>({
    from: defaultFrom,
    to: defaultTo,
    limit: 50,
    offset: 0,
  })
  const [conversationFilters, setConversationFilters] = useState<ConversationFilters>({
    from: defaultFrom,
    to: defaultTo,
    tenant_id: '',
    jwt_sub: '',
    limit: 50,
    offset: 0,
  })
  const [mlFilters, setMlFilters] = useState<MlLogFilters>({
    from: defaultFrom,
    to: defaultTo,
    tenant_id: '',
    model: '',
    status: '',
    limit: 50,
    offset: 0,
  })

  const [expandedAuditId, setExpandedAuditId] = useState<string | null>(null)
  const [expandedComplianceId, setExpandedComplianceId] = useState<string | null>(null)
  const [expandedConversationId, setExpandedConversationId] = useState<string | null>(null)
  const [selectedMlLog, setSelectedMlLog] = useState<MlLogEntry | null>(null)
  const [isDownloading, setIsDownloading] = useState(false)

  const canFetch = Boolean(user && !isRefreshingSession)

  const auditReady = Boolean(
    auditFilters.from && auditFilters.to && auditFilters.tenant_id && auditFilters.status
  )
  const complianceReady = Boolean(
    complianceFilters.from && complianceFilters.to
  )
  const conversationReady = Boolean(
    conversationFilters.from && conversationFilters.to && conversationFilters.tenant_id
  )
  const mlReady = Boolean(mlFilters.from && mlFilters.to && mlFilters.tenant_id)

  const handleDownloadCsv = async (tab: LogsTab) => {
    if (!session?.accessToken) {
      await logout()
      return
    }

    const params = new URLSearchParams()
    if (tab === 'audit') {
      params.set('from', toRfc3339(auditFilters.from))
      params.set('to', toRfc3339(auditFilters.to))
      if (auditFilters.tenant_id) params.set('tenant_id', auditFilters.tenant_id)
    }
    if (tab === 'compliance') {
      params.set('from', toRfc3339(complianceFilters.from))
      params.set('to', toRfc3339(complianceFilters.to))
    }
    if (tab === 'conversation') {
      params.set('from', toRfc3339(conversationFilters.from))
      params.set('to', toRfc3339(conversationFilters.to))
      if (conversationFilters.tenant_id) params.set('tenant_id', conversationFilters.tenant_id)
    }
    if (tab === 'ml') {
      params.set('from', toRfc3339(mlFilters.from))
      params.set('to', toRfc3339(mlFilters.to))
      if (mlFilters.tenant_id) params.set('tenant_id', mlFilters.tenant_id)
    }

    const url = getExportUrl(tab, params)

    try {
      setIsDownloading(true)
      const res = await fetch(url, {
        headers: {
          Authorization: `Bearer ${session.accessToken}`,
        },
        credentials: 'include',
      })
      if (res.status === 401) {
        await logout()
        return
      }
      if (res.status === 403) {
        toast({
          title: 'Error',
          description: 'You do not have permission to export data',
          variant: 'destructive',
        })
        return
      }
      if (!res.ok) {
        throw new Error('Failed to download CSV')
      }

      const blob = await res.blob()
      const filename = resolveExportFilename(res.headers.get('content-disposition')) ?? 'export.csv'
      const downloadUrl = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = downloadUrl
      link.download = filename
      link.click()
      URL.revokeObjectURL(downloadUrl)
    } catch (error) {
      console.error('Failed to download logs CSV', error)
      toast({
        title: 'Error',
        description: 'Failed to download CSV',
        variant: 'destructive',
      })
    } finally {
      setIsDownloading(false)
    }
  }

  const auditQuery = useQuery<LogsResponse<AuditLogEntry>>({
    queryKey: ['logs', 'audit', auditFilters],
    enabled: canFetch && activeTab === 'audit' && auditReady,
    queryFn: async () => {
      const params = new URLSearchParams()
      params.set('from', toRfc3339(auditFilters.from))
      params.set('to', toRfc3339(auditFilters.to))
      params.set('tenant_id', auditFilters.tenant_id)
      params.set('status', auditFilters.status)
      if (auditFilters.jwt_sub) params.set('jwt_sub', auditFilters.jwt_sub)
      params.set('limit', String(auditFilters.limit))
      params.set('offset', String(auditFilters.offset))

      const res = await fetch(`/api/logs/audit?${params.toString()}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        const error = new Error(payload?.error || 'Failed to load audit logs') as Error & { status?: number }
        error.status = res.status
        throw error
      }
      return res.json()
    },
    retry: (failureCount, error) => {
      const status = (error as Error & { status?: number }).status
      if (status === 401 || status === 403) return false
      return failureCount < 2
    },
  })

  const complianceQuery = useQuery<LogsResponse<ComplianceEventEntry>>({
    queryKey: ['logs', 'compliance', complianceFilters],
    enabled: canFetch && activeTab === 'compliance' && complianceReady,
    queryFn: async () => {
      const params = new URLSearchParams()
      params.set('from', toRfc3339(complianceFilters.from))
      params.set('to', toRfc3339(complianceFilters.to))
      params.set('limit', String(complianceFilters.limit))
      params.set('offset', String(complianceFilters.offset))

      const res = await fetch(`/api/logs/compliance?${params.toString()}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        const error = new Error(payload?.error || 'Failed to load compliance events') as Error & { status?: number }
        error.status = res.status
        throw error
      }
      return res.json()
    },
    retry: (failureCount, error) => {
      const status = (error as Error & { status?: number }).status
      if (status === 401 || status === 403) return false
      return failureCount < 2
    },
  })

  const conversationQuery = useQuery<LogsResponse<ConversationLogEntry>>({
    queryKey: ['logs', 'conversations', conversationFilters],
    enabled: canFetch && activeTab === 'conversation' && conversationReady,
    queryFn: async () => {
      const params = new URLSearchParams()
      params.set('from', toRfc3339(conversationFilters.from))
      params.set('to', toRfc3339(conversationFilters.to))
      params.set('tenant_id', conversationFilters.tenant_id)
      if (conversationFilters.jwt_sub) params.set('jwt_sub', conversationFilters.jwt_sub)
      params.set('limit', String(conversationFilters.limit))
      params.set('offset', String(conversationFilters.offset))

      const res = await fetch(`/api/logs/conversations?${params.toString()}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        const error = new Error(payload?.error || 'Failed to load conversation logs') as Error & { status?: number }
        error.status = res.status
        throw error
      }
      return res.json()
    },
    retry: (failureCount, error) => {
      const status = (error as Error & { status?: number }).status
      if (status === 401 || status === 403) return false
      return failureCount < 2
    },
  })

  const mlLogsQuery = useQuery<LogsResponse<MlLogEntry>>({
    queryKey: ['logs', 'ml', mlFilters],
    enabled: canFetch && activeTab === 'ml' && mlReady,
    queryFn: async () => {
      const params = new URLSearchParams()
      params.set('from', toRfc3339(mlFilters.from))
      params.set('to', toRfc3339(mlFilters.to))
      params.set('tenant_id', mlFilters.tenant_id)
      if (mlFilters.model) params.set('model', mlFilters.model)
      if (mlFilters.status) params.set('status', mlFilters.status)
      params.set('limit', String(mlFilters.limit))
      params.set('offset', String(mlFilters.offset))

      const res = await fetch(`/api/logs/ml?${params.toString()}`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        const error = new Error(payload?.error || 'Failed to load ML logs') as Error & { status?: number }
        error.status = res.status
        throw error
      }
      return res.json()
    },
    retry: (failureCount, error) => {
      const status = (error as Error & { status?: number }).status
      if (status === 401 || status === 403) return false
      return failureCount < 2
    },
  })

  const auditRows = auditQuery.data?.data ?? []
  const complianceRows = complianceQuery.data?.data ?? []
  const conversationRows = conversationQuery.data?.data ?? []
  const mlRows = mlLogsQuery.data?.data ?? []

  const auditPagination = buildPagination(auditQuery.data, {
    limit: auditFilters.limit,
    offset: auditFilters.offset,
  })
  const compliancePagination = buildPagination(complianceQuery.data, {
    limit: complianceFilters.limit,
    offset: complianceFilters.offset,
  })
  const conversationPagination = buildPagination(conversationQuery.data, {
    limit: conversationFilters.limit,
    offset: conversationFilters.offset,
  })
  const mlPagination = buildPagination(mlLogsQuery.data, {
    limit: mlFilters.limit,
    offset: mlFilters.offset,
  })

  const auditHasNext = auditRows.length === auditPagination.limit
  const complianceHasNext = complianceRows.length === compliancePagination.limit
  const conversationHasNext = conversationRows.length === conversationPagination.limit
  const mlHasNext = mlRows.length === mlPagination.limit

  const auditError = auditQuery.error as Error & { status?: number }
  const complianceError = complianceQuery.error as Error & { status?: number }
  const conversationError = conversationQuery.error as Error & { status?: number }
  const mlError = mlLogsQuery.error as Error & { status?: number }

  const renderRestricted = () => (
    <SectionCard title="Access limited" className="border-t-4 border-t-rose-500">
      <EmptyState
        icon={ShieldAlert}
        title="You do not have access to Logs"
        description="Your role does not have permission to view audit, compliance, or conversation logs."
      />
    </SectionCard>
  )

  const renderPagination = (
    pagination: PaginationState,
    rowsLength: number,
    onPrev: () => void,
    onNext: () => void,
    canNext: boolean
  ) => (
    <div className="mt-4 flex items-center justify-between">
      <div className="text-sm text-muted-foreground">
        Showing {rowsLength === 0 ? 0 : pagination.offset + 1} - {pagination.offset + rowsLength} of {pagination.total}
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={onPrev}
          disabled={pagination.offset === 0}
        >
          Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onNext}
          disabled={!canNext}
        >
          Next
        </Button>
      </div>
    </div>
  )

  const renderAuditTab = () => {
    if (auditError?.status === 403) {
      return renderRestricted()
    }

    return (
      <SectionCard title="Audit Logs" description="Request-level audit trail" className="border-t-4 border-t-pink-500">
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            <div className="space-y-2">
              <Label htmlFor="audit-from">From</Label>
              <Input
                id="audit-from"
                type="datetime-local"
                value={auditFilters.from}
                onChange={(e) => setAuditFilters((prev) => ({ ...prev, from: e.target.value, offset: 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="audit-to">To</Label>
              <Input
                id="audit-to"
                type="datetime-local"
                value={auditFilters.to}
                onChange={(e) => setAuditFilters((prev) => ({ ...prev, to: e.target.value, offset: 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="audit-tenant">Tenant ID</Label>
              <Select
                value={auditFilters.tenant_id}
                onValueChange={(value) => setAuditFilters((prev) => ({ ...prev, tenant_id: value, offset: 0 }))}
                disabled={tenantsQuery.isLoading || tenants.length === 0}
              >
                <SelectTrigger id="audit-tenant">
                  <SelectValue placeholder={tenantsQuery.isLoading ? 'Loading tenants...' : 'Select tenant'} />
                </SelectTrigger>
                <SelectContent>
                  {tenants.map((tenant) => (
                    <SelectItem key={tenant.tenant_id} value={tenant.tenant_id}>
                      {tenant.tenant_id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Status</Label>
              <Select
                value={auditFilters.status}
                onValueChange={(value) => setAuditFilters((prev) => ({ ...prev, status: value, offset: 0 }))}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="ok">OK</SelectItem>
                  <SelectItem value="error">Error</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2 md:col-span-2 lg:col-span-2">
              <Label htmlFor="audit-jwt-sub">JWT Sub (optional)</Label>
              <Input
                id="audit-jwt-sub"
                placeholder="user-id"
                value={auditFilters.jwt_sub}
                onChange={(e) => setAuditFilters((prev) => ({ ...prev, jwt_sub: e.target.value, offset: 0 }))}
              />
            </div>
          </div>

          <div className="flex items-center justify-end">
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleDownloadCsv('audit')}
              disabled={auditRows.length === 0 || isDownloading}
            >
              <Download className="mr-2 h-4 w-4" />
              Download CSV
            </Button>
          </div>

          {auditQuery.isLoading ? (
            <Skeleton className="h-64" />
          ) : auditQuery.isError ? (
            <div className="flex items-center justify-between rounded-md border p-4">
              <div>
                <p className="text-sm text-destructive">Failed to load audit logs</p>
                <p className="text-xs text-muted-foreground">{auditError?.message || 'Unknown error'}</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => auditQuery.refetch()}>
                Retry
              </Button>
            </div>
          ) : (
            <>
              <div className="border rounded-md overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Time</TableHead>
                      <TableHead>Tenant</TableHead>
                      <TableHead>Model</TableHead>
                      <TableHead>Provider</TableHead>
                      <TableHead>Strategy</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="text-right">Latency (ms)</TableHead>
                      <TableHead>Actor</TableHead>
                      <TableHead>Request ID</TableHead>
                      <TableHead>Decision</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {auditRows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={10} className="text-center text-muted-foreground">
                          No audit logs found for the selected filters
                        </TableCell>
                      </TableRow>
                    ) : (
                      auditRows.map((row, index) => {
                        const rowId = row.request_id || row.ts || row.timestamp || `row-${index}`
                        const isExpanded = expandedAuditId === rowId
                        const actor = row.actor || row.jwt_sub || row.api_key_name || row.api_key_id || '—'
                        const hasDetails = !!(
                          row.error_type || row.error || row.fallback_used !== undefined ||
                          row.routing_snapshot || row.decision_snapshot ||
                          row.api_key_id || row.api_key_name || row.jwt_sub
                        )
                        return (
                          <Fragment key={`${rowId}-${index}`}>
                            <TableRow>
                              <TableCell className="font-mono text-xs whitespace-nowrap">
                                {formatTimestamp(row.ts || row.timestamp)}
                              </TableCell>
                              <TableCell className="font-medium">{row.tenant_id || '—'}</TableCell>
                              <TableCell>{row.model || '—'}</TableCell>
                              <TableCell>{row.provider || '—'}</TableCell>
                              <TableCell>{row.strategy || '—'}</TableCell>
                              <TableCell>
                                <Badge className={getStatusBadgeClass(row.status)} variant="outline">
                                  {row.status || '—'}
                                </Badge>
                              </TableCell>
                              <TableCell className="text-right tabular-nums">
                                {row.latency_ms ?? '—'}
                              </TableCell>
                              <TableCell className="font-mono text-xs max-w-[140px] truncate" title={actor}>
                                {actor}
                              </TableCell>
                              <TableCell className="font-mono text-xs max-w-[120px] truncate" title={row.request_id || ''}>
                                {row.request_id ? row.request_id.slice(0, 8) + '…' : '—'}
                              </TableCell>
                              <TableCell>
                                <div className="flex items-center justify-between gap-2 max-w-[220px]">
                                  <span
                                    className="text-xs truncate"
                                    title={row.decision || row.decision_reason || ''}
                                  >
                                    {row.decision || row.decision_reason || '—'}
                                  </span>
                                  {hasDetails && (
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      className="shrink-0"
                                      onClick={() => setExpandedAuditId(isExpanded ? null : rowId)}
                                    >
                                      {isExpanded ? 'Hide' : 'View'}
                                    </Button>
                                  )}
                                </div>
                              </TableCell>
                            </TableRow>
                            {isExpanded && (
                              <TableRow key={`${rowId}-${index}-details`}>
                                <TableCell colSpan={10} className="bg-muted/30">
                                  <div className="grid gap-3 text-xs p-1">
                                    <div className="grid grid-cols-2 gap-x-8 gap-y-1 md:grid-cols-3">
                                      {row.request_id && (
                                        <div>
                                          <span className="text-muted-foreground">Request ID: </span>
                                          <span className="font-mono">{row.request_id}</span>
                                        </div>
                                      )}
                                      {row.error_type && (
                                        <div>
                                          <span className="text-muted-foreground">Error Type: </span>
                                          <span className="text-destructive">{row.error_type}</span>
                                        </div>
                                      )}
                                      {row.error && (
                                        <div className="col-span-2 md:col-span-3">
                                          <span className="text-muted-foreground">Error: </span>
                                          <span className="text-destructive">{row.error}</span>
                                        </div>
                                      )}
                                      {row.fallback_used !== undefined && (
                                        <div>
                                          <span className="text-muted-foreground">Fallback Used: </span>
                                          <span>{row.fallback_used ? 'Yes' : 'No'}</span>
                                        </div>
                                      )}
                                      {row.jwt_sub && (
                                        <div>
                                          <span className="text-muted-foreground">JWT Sub: </span>
                                          <span className="font-mono">{row.jwt_sub}</span>
                                        </div>
                                      )}
                                      {row.api_key_name && (
                                        <div>
                                          <span className="text-muted-foreground">API Key Name: </span>
                                          <span className="font-mono">{row.api_key_name}</span>
                                        </div>
                                      )}
                                      {row.api_key_id && (
                                        <div>
                                          <span className="text-muted-foreground">API Key ID: </span>
                                          <span className="font-mono">{row.api_key_id}</span>
                                        </div>
                                      )}
                                    </div>
                                    {row.routing_snapshot && (
                                      <div>
                                        <p className="text-muted-foreground font-medium mb-1">Routing Snapshot</p>
                                        <pre className="whitespace-pre-wrap font-mono bg-background rounded border p-2">
                                          {JSON.stringify(row.routing_snapshot, null, 2)}
                                        </pre>
                                      </div>
                                    )}
                                    {row.decision_snapshot && (
                                      <div>
                                        <p className="text-muted-foreground font-medium mb-1">Decision Snapshot</p>
                                        <pre className="whitespace-pre-wrap font-mono bg-background rounded border p-2">
                                          {JSON.stringify(row.decision_snapshot, null, 2)}
                                        </pre>
                                      </div>
                                    )}
                                  </div>
                                </TableCell>
                              </TableRow>
                            )}
                          </Fragment>
                        )
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
              {renderPagination(
                auditPagination,
                auditRows.length,
                () => setAuditFilters((prev) => ({ ...prev, offset: Math.max(0, prev.offset - prev.limit) })),
                () => setAuditFilters((prev) => ({ ...prev, offset: prev.offset + prev.limit })),
                auditHasNext
              )}
            </>
          )}
        </div>
      </SectionCard>
    )
  }

  const renderComplianceTab = () => {
    if (complianceError?.status === 403) {
      return renderRestricted()
    }

    return (
      <SectionCard title="Compliance Events" description="Policy enforcement and governance events" className="border-t-4 border-t-amber-400">
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            <div className="space-y-2">
              <Label htmlFor="compliance-from">From</Label>
              <Input
                id="compliance-from"
                type="datetime-local"
                value={complianceFilters.from}
                onChange={(e) => setComplianceFilters((prev) => ({ ...prev, from: e.target.value, offset: 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="compliance-to">To</Label>
              <Input
                id="compliance-to"
                type="datetime-local"
                value={complianceFilters.to}
                onChange={(e) => setComplianceFilters((prev) => ({ ...prev, to: e.target.value, offset: 0 }))}
              />
            </div>
          </div>

          <div className="flex items-center justify-end">
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleDownloadCsv('compliance')}
              disabled={complianceRows.length === 0 || isDownloading}
            >
              <Download className="mr-2 h-4 w-4" />
              Download CSV
            </Button>
          </div>

          {complianceQuery.isLoading ? (
            <Skeleton className="h-64" />
          ) : complianceQuery.isError ? (
            <div className="flex items-center justify-between rounded-md border p-4">
              <div>
                <p className="text-sm text-destructive">Failed to load compliance events</p>
                <p className="text-xs text-muted-foreground">{complianceError?.message || 'Unknown error'}</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => complianceQuery.refetch()}>
                Retry
              </Button>
            </div>
          ) : (
            <>
              <div className="border rounded-md">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Created At</TableHead>
                      <TableHead>Tenant</TableHead>
                      <TableHead>Request ID</TableHead>
                      <TableHead>Event Type</TableHead>
                      <TableHead>Action Taken</TableHead>
                      <TableHead>Metadata</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {complianceRows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center text-muted-foreground">
                          No compliance events found for the selected filters
                        </TableCell>
                      </TableRow>
                    ) : (
                      complianceRows.map((row, index) => {
                        const rowId = row.id || `row-${index}`
                        const isExpanded = expandedComplianceId === rowId
                        return (
                          <Fragment key={rowId}>
                            <TableRow key={rowId}>
                              <TableCell className="font-mono text-xs">{formatTimestamp(row.created_at)}</TableCell>
                              <TableCell className="font-medium">{row.tenant_id || '—'}</TableCell>
                              <TableCell className="font-mono text-xs">{row.request_id || '—'}</TableCell>
                              <TableCell>{row.event_type || '—'}</TableCell>
                              <TableCell>{row.action_taken || '—'}</TableCell>
                              <TableCell>
                                <div className="flex items-center justify-between gap-2">
                                  <span className="text-xs text-muted-foreground">{previewJson(row.metadata)}</span>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() =>
                                      setExpandedComplianceId(isExpanded ? null : rowId)
                                    }
                                  >
                                    {isExpanded ? 'Hide' : 'View'}
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                            {isExpanded && (
                              <TableRow key={`${rowId}-details`}>
                                <TableCell colSpan={6} className="bg-muted/30">
                                  <pre className="text-xs whitespace-pre-wrap font-mono">
                                    {JSON.stringify(row.metadata ?? {}, null, 2)}
                                  </pre>
                                </TableCell>
                              </TableRow>
                            )}
                          </Fragment>
                        )
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
              {renderPagination(
                compliancePagination,
                complianceRows.length,
                () => setComplianceFilters((prev) => ({ ...prev, offset: Math.max(0, prev.offset - prev.limit) })),
                () => setComplianceFilters((prev) => ({ ...prev, offset: prev.offset + prev.limit })),
                complianceHasNext
              )}
            </>
          )}
        </div>
      </SectionCard>
    )
  }

  const renderConversationTab = () => {
    if (conversationError?.status === 403) {
      return renderRestricted()
    }

    return (
      <SectionCard title="Conversation Logs" description="Conversation-level auditing and review" className="border-t-4 border-t-purple-500">
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            <div className="space-y-2">
              <Label htmlFor="conversation-from">From</Label>
              <Input
                id="conversation-from"
                type="datetime-local"
                value={conversationFilters.from}
                onChange={(e) => setConversationFilters((prev) => ({ ...prev, from: e.target.value, offset: 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="conversation-to">To</Label>
              <Input
                id="conversation-to"
                type="datetime-local"
                value={conversationFilters.to}
                onChange={(e) => setConversationFilters((prev) => ({ ...prev, to: e.target.value, offset: 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="conversation-tenant">Tenant ID</Label>
              <Select
                value={conversationFilters.tenant_id}
                onValueChange={(value) => setConversationFilters((prev) => ({ ...prev, tenant_id: value, offset: 0 }))}
                disabled={tenantsQuery.isLoading || tenants.length === 0}
              >
                <SelectTrigger id="conversation-tenant">
                  <SelectValue placeholder={tenantsQuery.isLoading ? 'Loading tenants...' : 'Select tenant'} />
                </SelectTrigger>
                <SelectContent>
                  {tenants.map((tenant) => (
                    <SelectItem key={tenant.tenant_id} value={tenant.tenant_id}>
                      {tenant.tenant_id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="conversation-jwt">JWT Sub (optional)</Label>
              <Input
                id="conversation-jwt"
                placeholder="user-id"
                value={conversationFilters.jwt_sub}
                onChange={(e) => setConversationFilters((prev) => ({ ...prev, jwt_sub: e.target.value, offset: 0 }))}
              />
            </div>
          </div>

          <div className="flex items-center justify-end">
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleDownloadCsv('conversation')}
              disabled={conversationRows.length === 0 || isDownloading}
            >
              <Download className="mr-2 h-4 w-4" />
              Download CSV
            </Button>
          </div>

          {conversationQuery.isLoading ? (
            <Skeleton className="h-64" />
          ) : conversationQuery.isError ? (
            <div className="flex items-center justify-between rounded-md border p-4">
              <div>
                <p className="text-sm text-destructive">Failed to load conversation logs</p>
                <p className="text-xs text-muted-foreground">{conversationError?.message || 'Unknown error'}</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => conversationQuery.refetch()}>
                Retry
              </Button>
            </div>
          ) : (
            <>
              <div className="border rounded-md">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Created At</TableHead>
                      <TableHead>Tenant</TableHead>
                      <TableHead>JWT Sub</TableHead>
                      <TableHead>Workflow</TableHead>
                      <TableHead>Logging Mode</TableHead>
                      <TableHead>PII Detected</TableHead>
                      <TableHead>Prompt Preview</TableHead>
                      <TableHead>Response Preview</TableHead>
                      <TableHead></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {conversationRows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={9} className="text-center text-muted-foreground">
                          No conversation logs found for the selected filters
                        </TableCell>
                      </TableRow>
                    ) : (
                      conversationRows.map((row, index) => {
                        const rowId = row.id || `row-${index}`
                        const isExpanded = expandedConversationId === rowId
                        return (
                          <Fragment key={rowId}>
                            <TableRow key={rowId}>
                              <TableCell className="font-mono text-xs">{formatTimestamp(row.created_at)}</TableCell>
                              <TableCell className="font-medium">{row.tenant_id || '—'}</TableCell>
                              <TableCell className="font-mono text-xs">{row.jwt_sub || '—'}</TableCell>
                              <TableCell>
                                {row.workflow_id ? (
                                  <Badge variant="outline" className="font-mono text-xs max-w-[120px] truncate">
                                    {row.workflow_id}
                                  </Badge>
                                ) : (
                                  <span className="text-muted-foreground text-xs">—</span>
                                )}
                              </TableCell>
                              <TableCell>{row.logging_mode || '—'}</TableCell>
                              <TableCell>
                                {row.pii_detected ? (
                                  <Badge variant="destructive">Yes</Badge>
                                ) : (
                                  <Badge variant="secondary">No</Badge>
                                )}
                              </TableCell>
                              <TableCell className="max-w-[220px] truncate text-xs text-muted-foreground">
                                {truncateText(row.prompt_preview, 100)}
                              </TableCell>
                              <TableCell className="max-w-[220px] truncate text-xs text-muted-foreground">
                                {truncateText(row.response_preview, 100)}
                              </TableCell>
                              <TableCell>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() =>
                                    setExpandedConversationId(isExpanded ? null : rowId)
                                  }
                                >
                                  {isExpanded ? 'Hide' : 'View'}
                                </Button>
                              </TableCell>
                            </TableRow>
                            {isExpanded && (
                              <TableRow key={`${rowId}-details`}>
                                <TableCell colSpan={9} className="bg-muted/30">
                                  <div className="grid gap-2 text-sm">
                                    <div>
                                      <span className="text-muted-foreground">Request ID:</span>{' '}
                                      <span className="font-mono text-xs">{row.request_id || '—'}</span>
                                    </div>
                                    {row.workflow_id && (
                                      <div>
                                        <span className="text-muted-foreground">Workflow ID:</span>{' '}
                                        <span className="font-mono text-xs">{row.workflow_id}</span>
                                        {row.conversation_id && (
                                          <>
                                            {' · '}
                                            <span className="text-muted-foreground">Conversation ID:</span>{' '}
                                            <span className="font-mono text-xs">{row.conversation_id}</span>
                                          </>
                                        )}
                                      </div>
                                    )}
                                    {row.customer_id && (
                                      <div>
                                        <span className="text-muted-foreground">Customer ID:</span>{' '}
                                        <span className="font-mono text-xs">{row.customer_id}</span>
                                      </div>
                                    )}
                                    <div>
                                      <span className="text-muted-foreground">Prompt Redacted:</span>{' '}
                                      <span className="text-xs">{row.prompt_redacted || '—'}</span>
                                    </div>
                                    <div>
                                      <span className="text-muted-foreground">Response Redacted:</span>{' '}
                                      <span className="text-xs">{row.response_redacted || '—'}</span>
                                    </div>
                                    <div>
                                      <span className="text-muted-foreground">Prompt Full:</span>{' '}
                                      <span className="text-xs">{row.prompt_full || '—'}</span>
                                    </div>
                                    <div>
                                      <span className="text-muted-foreground">Response Full:</span>{' '}
                                      <span className="text-xs">{row.response_full || '—'}</span>
                                    </div>
                                  </div>
                                </TableCell>
                              </TableRow>
                            )}
                          </Fragment>
                        )
                      })
                    )}
                  </TableBody>
                </Table>
              </div>
              {renderPagination(
                conversationPagination,
                conversationRows.length,
                () => setConversationFilters((prev) => ({ ...prev, offset: Math.max(0, prev.offset - prev.limit) })),
                () => setConversationFilters((prev) => ({ ...prev, offset: prev.offset + prev.limit })),
                conversationHasNext
              )}
            </>
          )}
        </div>
      </SectionCard>
    )
  }

  const renderMlTab = () => {
    if (mlError?.status === 403) {
      return renderRestricted()
    }

    return (
      <SectionCard title="ML Logs" description="Persisted ML request activity" className="border-t-4 border-t-cyan-400">
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            <div className="space-y-2">
              <Label htmlFor="ml-from">From</Label>
              <Input
                id="ml-from"
                type="datetime-local"
                value={mlFilters.from}
                onChange={(e) => setMlFilters((prev) => ({ ...prev, from: e.target.value, offset: 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="ml-to">To</Label>
              <Input
                id="ml-to"
                type="datetime-local"
                value={mlFilters.to}
                onChange={(e) => setMlFilters((prev) => ({ ...prev, to: e.target.value, offset: 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="ml-tenant">Tenant ID</Label>
              <Select
                value={mlFilters.tenant_id}
                onValueChange={(value) => setMlFilters((prev) => ({ ...prev, tenant_id: value, offset: 0 }))}
                disabled={tenantsQuery.isLoading || tenants.length === 0}
              >
                <SelectTrigger id="ml-tenant">
                  <SelectValue placeholder={tenantsQuery.isLoading ? 'Loading tenants...' : 'Select tenant'} />
                </SelectTrigger>
                <SelectContent>
                  {tenants.map((tenant) => (
                    <SelectItem key={tenant.tenant_id} value={tenant.tenant_id}>
                      {tenant.tenant_id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="ml-model">Model (optional)</Label>
              <Input
                id="ml-model"
                placeholder="model name"
                value={mlFilters.model}
                onChange={(e) => setMlFilters((prev) => ({ ...prev, model: e.target.value, offset: 0 }))}
              />
            </div>
            <div className="space-y-2">
              <Label>Status (optional)</Label>
              <Select
                value={mlFilters.status || 'all'}
                onValueChange={(value) =>
                  setMlFilters((prev) => ({
                    ...prev,
                    status: value === 'all' ? '' : value,
                    offset: 0,
                  }))
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder="All" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="ok">OK</SelectItem>
                  <SelectItem value="error">Error</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          {mlLogsQuery.isLoading ? (
            <Skeleton className="h-64" />
          ) : mlLogsQuery.isError ? (
            <div className="flex items-center justify-between rounded-md border p-4">
              <div>
                <p className="text-sm text-destructive">Failed to load ML logs</p>
                <p className="text-xs text-muted-foreground">{mlError?.message || 'Unknown error'}</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => mlLogsQuery.refetch()}>
                Retry
              </Button>
            </div>
          ) : (
            <>
              <div className="border rounded-md">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Timestamp</TableHead>
                      <TableHead>Tenant</TableHead>
                      <TableHead>Model</TableHead>
                      <TableHead>Provider</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="text-right">Latency</TableHead>
                      <TableHead>Observable Fields</TableHead>
                      <TableHead className="text-right">Details</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {mlRows.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={8} className="text-center text-muted-foreground">
                          No ML logs found for this tenant.
                        </TableCell>
                      </TableRow>
                    ) : (
                      mlRows.map((row, index) => (
                        <TableRow key={`${row.timestamp || 'row'}-${index}`}>
                          <TableCell className="font-mono text-xs">{formatTimestamp(row.timestamp)}</TableCell>
                          <TableCell className="font-medium">{row.tenant_id || '—'}</TableCell>
                          <TableCell>{row.model || '—'}</TableCell>
                          <TableCell>{row.provider || '—'}</TableCell>
                          <TableCell>
                            <Badge className={getStatusBadgeClass(row.status)} variant="outline">
                              {row.status || '—'}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatLatencyMs(row.latency_ms)}
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground">
                            {previewObservable(row.metadata?.observable)}
                          </TableCell>
                          <TableCell className="text-right">
                            <Button variant="ghost" size="sm" onClick={() => setSelectedMlLog(row)}>
                              Details
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
              {renderPagination(
                mlPagination,
                mlRows.length,
                () => setMlFilters((prev) => ({ ...prev, offset: Math.max(0, prev.offset - prev.limit) })),
                () => setMlFilters((prev) => ({ ...prev, offset: prev.offset + prev.limit })),
                mlHasNext
              )}
            </>
          )}
        </div>
      </SectionCard>
    )
  }

  if (user && isRefreshingSession) {
    return (
      <div>
        <PageHeader
          title="Logs"
          description="Audit, compliance, and conversation records"
        />
        <Skeleton className="h-64" />
      </div>
    )
  }

  return (
    <RequireAdminRole allowedRoles={['admin', 'audit']}>
    <div>
      <PageHeader
        title="Logs"
        description="Audit, compliance, and conversation records"
      />

      <div className="mb-6 flex flex-wrap gap-2">
        <Button
          variant={activeTab === 'audit' ? 'default' : 'outline'}
          onClick={() => setActiveTab('audit')}
        >
          Audit Logs
        </Button>
        <Button
          variant={activeTab === 'conversation' ? 'default' : 'outline'}
          onClick={() => setActiveTab('conversation')}
        >
          Conversation Logs
        </Button>
        <Button
          variant={activeTab === 'ml' ? 'default' : 'outline'}
          onClick={() => setActiveTab('ml')}
        >
          ML Logs
        </Button>
      </div>

      <div className="space-y-6">
        {activeTab === 'audit' && renderAuditTab()}
        {activeTab === 'compliance' && renderComplianceTab()}
        {activeTab === 'conversation' && renderConversationTab()}
        {activeTab === 'ml' && renderMlTab()}
      </div>

      <Dialog open={selectedMlLog !== null} onOpenChange={(open) => !open && setSelectedMlLog(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>ML Log Details</DialogTitle>
            <DialogDescription>Request-level metadata for ML inference.</DialogDescription>
          </DialogHeader>
          {selectedMlLog && (
            <div className="space-y-4 text-sm">
              <div className="grid gap-2">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Timestamp</span>
                  <span className="font-medium">{formatTimestamp(selectedMlLog.timestamp)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Tenant</span>
                  <span className="font-medium">{selectedMlLog.tenant_id || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Model</span>
                  <span className="font-medium">{selectedMlLog.model || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Provider</span>
                  <span className="font-medium">{selectedMlLog.provider || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Status</span>
                  <span className="font-medium">{selectedMlLog.status || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Latency</span>
                  <span className="font-medium">{formatLatencyMs(selectedMlLog.latency_ms)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Strategy</span>
                  <span className="font-medium">{selectedMlLog.strategy || 'ml'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Request ID</span>
                  <span className="font-medium">{selectedMlLog.request_id || '—'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Caller</span>
                  <span className="font-medium">
                    {selectedMlLog.api_key_name || selectedMlLog.jwt_sub || '—'}
                  </span>
                </div>
              </div>

              <div className="border-t pt-4">
                <p className="text-xs text-muted-foreground mb-2">Observable metadata</p>
                <pre className="bg-muted p-3 rounded-md text-xs overflow-auto max-h-64">
                  {JSON.stringify(selectedMlLog.metadata?.observable ?? {}, null, 2)}
                </pre>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
    </RequireAdminRole>
  )
}
