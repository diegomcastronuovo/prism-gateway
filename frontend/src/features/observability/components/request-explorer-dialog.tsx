'use client'

import { useMemo, useState } from 'react'
import { useRouter } from 'next/navigation'
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { ArrowLeft, Download, Copy, Check } from 'lucide-react'
import { useReplayRequest, useRequestExplorer, useRequestRoutingSnapshot } from '../api/use-request-explorer'
import type {
  RequestLogDetail,
  RequestExplorerFilters,
  RequestExplorerSortDirection,
  RequestExplorerSortField,
} from '../types/request-explorer'

interface RequestExplorerViewProps {
  onBack: () => void
}

function formatTimestamp(timestamp: string): string {
  if (!timestamp) return '-'
  const date = new Date(timestamp)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

function formatSnapshotTimestamp(timestamp?: string): string {
  if (!timestamp) return '-'
  const date = new Date(timestamp)
  if (Number.isNaN(date.getTime())) return timestamp
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function formatUsdCost(value: number): string {
  return `$${value.toFixed(6)}`
}

function formatDeltaVsSelected(modelCost: number, selectedCost?: number, isSelected = false): string {
  if (isSelected) return 'selected'
  if (!selectedCost || selectedCost <= 0) return '-'
  const delta = ((modelCost - selectedCost) / selectedCost) * 100
  const rounded = Math.round(delta)
  return `${rounded > 0 ? '+' : ''}${rounded}%`
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

function statusLabel(status: string): string {
  if (status === 'ok') return 'success'
  return status
}

function SortHeader({
  label,
  field,
  sortField,
  sortDirection,
  onSortChange,
}: {
  label: string
  field: RequestExplorerSortField
  sortField: RequestExplorerSortField
  sortDirection: RequestExplorerSortDirection
  onSortChange: (field: RequestExplorerSortField) => void
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

export function RequestExplorerView({ onBack }: RequestExplorerViewProps) {
  const router = useRouter()
  const [filters, setFilters] = useState<RequestExplorerFilters>({
    time_range: '24h',
    limit: 50,
    offset: 0,
  })
  const [currentPage, setCurrentPage] = useState(0)
  const [sortField, setSortField] = useState<RequestExplorerSortField>('timestamp')
  const [sortDirection, setSortDirection] = useState<RequestExplorerSortDirection>('desc')
  const [selectedRequest, setSelectedRequest] = useState<RequestLogDetail | null>(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [showRawJson, setShowRawJson] = useState(false)
  const [replayModalOpen, setReplayModalOpen] = useState(false)
  const [replayError, setReplayError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const { data, isLoading } = useRequestExplorer(filters)
  const replayRequest = useReplayRequest()
  const {
    data: routingSnapshot,
    isLoading: routingSnapshotLoading,
  } = useRequestRoutingSnapshot(selectedRequest?.request_id ?? null, drawerOpen && Boolean(selectedRequest?.request_id))

  const updateFilter = (key: keyof RequestExplorerFilters, value: string | boolean) => {
    setCurrentPage(0)
    setFilters((prev) => ({
      ...prev,
      offset: 0,
      [key]: value === '' ? undefined : value,
    }))
  }

  const goToPage = (page: number) => {
    const safePage = Math.max(0, page)
    const limit = Math.min(200, filters.limit || 50)
    setCurrentPage(safePage)
    setFilters((prev) => ({
      ...prev,
      limit,
      offset: safePage * limit,
    }))
  }

  const handleSortChange = (field: RequestExplorerSortField) => {
    if (field === sortField) {
      setSortDirection((prev) => (prev === 'asc' ? 'desc' : 'asc'))
      return
    }
    setSortField(field)
    setSortDirection('asc')
  }

  const requests = useMemo(() => data?.requests ?? [], [data?.requests])

  const tenantOptions = useMemo(
    () => Array.from(new Set(requests.map((request) => request.tenant_id).filter(Boolean))).sort(),
    [requests]
  )
  const modelOptions = useMemo(
    () => Array.from(new Set(requests.map((request) => request.model).filter(Boolean))).sort(),
    [requests]
  )
  const providerOptions = useMemo(
    () => Array.from(new Set(requests.map((request) => request.provider).filter(Boolean))).sort(),
    [requests]
  )
  const statusOptions = useMemo(
    () => Array.from(new Set(requests.map((request) => request.status).filter(Boolean))).sort(),
    [requests]
  )
  const normalizedStatusOptions = useMemo(() => {
    const base = new Set(statusOptions)
    base.add('ok')
    base.add('error')
    return Array.from(base).sort()
  }, [statusOptions])

  const sortedRequests = useMemo(() => {
    const sorted = [...requests]
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
  }, [requests, sortDirection, sortField])

  const handleRowClick = (request: RequestLogDetail) => {
    setSelectedRequest(request)
    setShowRawJson(false)
    setReplayModalOpen(false)
    setReplayError(null)
    setDrawerOpen(true)
  }

  const handleReplayRequest = async () => {
    if (!selectedRequest?.request_id) return
    setReplayError(null)
    try {
      await replayRequest.mutateAsync(selectedRequest.request_id)
      setReplayModalOpen(true)
    } catch {
      setReplayError('Replay failed.')
    }
  }

  const handleCopyRequestId = async () => {
    if (!selectedRequest?.request_id) return
    try {
      await navigator.clipboard.writeText(selectedRequest.request_id)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // optional: could show a small inline error, but avoiding new UI deps
    }
  }

  const navigateToReplay = () => {
    if (!selectedRequest?.request_id) return
    const id = encodeURIComponent(selectedRequest.request_id)
    router.push(`/routing/replay?request_id=${id}`)
  }

  const exportRequestsToCsv = () => {
    const headers = [
      'timestamp',
      'tenant_id',
      'request_id',
      'model',
      'provider',
      'strategy',
      'latency_ms',
      'status',
      'fallback_used',
    ]
    const rows = sortedRequests.map((request) => [
      request.timestamp,
      request.tenant_id,
      request.request_id,
      request.model,
      request.provider,
      request.strategy,
      String(request.latency_ms ?? ''),
      request.status,
      String(request.fallback_used),
    ])
    const escapeCell = (value: string) => `"${value.replace(/"/g, '""')}"`
    const content = [
      headers.map(escapeCell).join(','),
      ...rows.map((row) => row.map((cell) => escapeCell(String(cell))).join(',')),
    ].join('\n')

    const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const now = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    const fileName = `request_explorer_${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}.csv`

    const link = document.createElement('a')
    link.href = url
    link.download = fileName
    link.click()
    URL.revokeObjectURL(url)
  }

  const pagination = data?.pagination
  const total = pagination?.total ?? data?.total ?? sortedRequests.length
  const limit = pagination?.limit ?? filters.limit ?? 50
  const totalPages = Math.max(1, Math.ceil(total / limit))
  const routingDetails = routingSnapshot?.routing_snapshot
  const replayData = replayRequest.data
  const replayDetails = replayData?.routing_snapshot
  const replaySelectedModel = replayData?.selected_model || replayDetails?.selected_model
  const replayCosts = replayDetails?.estimated_costs_usd
  const replaySelectedCost =
    replaySelectedModel && replayCosts ? replayCosts[replaySelectedModel] : undefined

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="outline" onClick={onBack}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Observability
        </Button>
        <h2 className="text-2xl font-bold">Request Explorer</h2>
      </div>

      <div className="space-y-4">
        <div className="grid grid-cols-6 gap-4 p-4 border rounded-lg bg-muted/50">
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
            <Label htmlFor="status-filter">Status</Label>
            <Select value={filters.status || 'all'} onValueChange={(v) => updateFilter('status', v === 'all' ? '' : v)}>
              <SelectTrigger id="status-filter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                {normalizedStatusOptions.map((status) => (
                  <SelectItem key={status} value={status}>
                    {status}
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
            <Label htmlFor="time-filter">Time Range</Label>
            <Select value={filters.time_range || '24h'} onValueChange={(v) => updateFilter('time_range', v)}>
              <SelectTrigger id="time-filter">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1h">Last 1 hour</SelectItem>
                <SelectItem value="24h">Last 24 hours</SelectItem>
                <SelectItem value="7d">Last 7 days</SelectItem>
                <SelectItem value="30d">Last 30 days</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="flex justify-end">
          <Button variant="outline" size="sm" onClick={exportRequestsToCsv} disabled={sortedRequests.length === 0}>
            <Download className="mr-2 h-4 w-4" />
            Export CSV
          </Button>
        </div>

        <div className="border rounded-md">
          {isLoading ? (
            <Skeleton className="h-96" />
          ) : (
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
                  <TableHead>
                    <SortHeader
                      label="Model"
                      field="model"
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
                  <TableHead>Request ID</TableHead>
                  <TableHead>Strategy</TableHead>
                  <TableHead>
                    <SortHeader
                      label="Latency (ms)"
                      field="latency_ms"
                      sortField={sortField}
                      sortDirection={sortDirection}
                      onSortChange={handleSortChange}
                    />
                  </TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Fallback</TableHead>
                  <TableHead>Cache</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sortedRequests.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={10} className="text-center text-muted-foreground">
                      No requests found
                    </TableCell>
                  </TableRow>
                ) : (
                  sortedRequests.map((request) => (
                    <TableRow
                      key={request.id}
                      className="cursor-pointer hover:bg-muted/50"
                      onClick={() => handleRowClick(request)}
                    >
                      <TableCell className="font-mono text-xs">
                        {formatTimestamp(request.timestamp)}
                      </TableCell>
                      <TableCell className="font-medium">{request.tenant_id}</TableCell>
                      <TableCell>{request.model}</TableCell>
                      <TableCell>{request.provider}</TableCell>
                      <TableCell className="font-mono text-xs">{request.request_id}</TableCell>
                      <TableCell>
                        <Badge variant="secondary">{request.strategy}</Badge>
                      </TableCell>
                      <TableCell className="tabular-nums">{request.latency_ms}</TableCell>
                      <TableCell>
                        <Badge className={getStatusBadgeClassName(request.status)} variant={getStatusVariant(request.status)}>
                          {statusLabel(request.status)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {request.fallback_used ? (
                          <Badge variant="secondary">Yes</Badge>
                        ) : (
                          <span className="text-muted-foreground text-sm">-</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {request.cache_status ? (
                          <Badge variant="outline">{request.cache_status.toUpperCase()}</Badge>
                        ) : (
                          <span className="text-muted-foreground text-sm">-</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          )}
        </div>

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
      </div>

      <Sheet open={drawerOpen} onOpenChange={setDrawerOpen}>
        <SheetContent className="w-[800px] sm:w-[900px] max-w-[95vw] overflow-y-auto">
          <SheetHeader>
            <SheetTitle>Request Details</SheetTitle>
            <SheetDescription>
              <div className="flex items-center gap-2">
                <span>Request ID:</span>
                <span className="font-mono break-all">{selectedRequest?.request_id || '-'}</span>
                {selectedRequest?.request_id && (
                  <button
                    type="button"
                    className="inline-flex items-center justify-center rounded-md border px-2 py-1 text-xs hover:bg-accent"
                    title="Copy Request ID"
                    onClick={handleCopyRequestId}
                  >
                    {copied ? (
                      <>
                        <Check className="h-3 w-3 mr-1" /> Copied
                      </>
                    ) : (
                      <>
                        <Copy className="h-3 w-3 mr-1" /> Copy
                      </>
                    )}
                  </button>
                )}
              </div>
            </SheetDescription>
          </SheetHeader>

          {selectedRequest && (
            <div className="space-y-6 mt-6">
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" onClick={() => setShowRawJson((prev) => !prev)}>
                  View Raw JSON
                </Button>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleReplayRequest}
                  disabled={!selectedRequest.request_id || replayRequest.isPending}
                >
                  {replayRequest.isPending ? 'Replaying...' : 'Replay Request'}
                </Button>
                <Button
                  variant="default"
                  size="sm"
                  onClick={navigateToReplay}
                  disabled={!selectedRequest?.request_id}
                  title="Open in Replay"
                >
                  Replay
                </Button>
                {replayError && <p className="text-sm text-destructive">{replayError}</p>}
              </div>

              <div className="border rounded-lg p-4 bg-muted/50">
                <h3 className="font-semibold mb-4">Request Info</h3>
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div>
                    <h4 className="font-medium mb-1">Request ID</h4>
                    <div className="flex items-center gap-2">
                      <p className="font-mono break-all">{selectedRequest.request_id}</p>
                      <button
                        type="button"
                        className="inline-flex items-center justify-center rounded-md border px-2 py-1 text-xs hover:bg-accent"
                        title="Copy Request ID"
                        onClick={handleCopyRequestId}
                        disabled={!selectedRequest.request_id}
                      >
                        {copied ? (
                          <>
                            <Check className="h-3 w-3 mr-1" /> Copied
                          </>
                        ) : (
                          <>
                            <Copy className="h-3 w-3 mr-1" /> Copy
                          </>
                        )}
                      </button>
                    </div>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Timestamp</h4>
                    <p>{formatTimestamp(selectedRequest.timestamp)}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Tenant</h4>
                    <p>{selectedRequest.tenant_id}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Model</h4>
                    <p>{selectedRequest.model}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Provider</h4>
                    <p>{selectedRequest.provider}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Strategy</h4>
                    <Badge variant="secondary">{selectedRequest.strategy}</Badge>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Status</h4>
                    <Badge className={getStatusBadgeClassName(selectedRequest.status)} variant={getStatusVariant(selectedRequest.status)}>
                      {statusLabel(selectedRequest.status)}
                    </Badge>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Latency (ms)</h4>
                    <p className="tabular-nums">{selectedRequest.latency_ms}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Fallback</h4>
                    <p>{selectedRequest.fallback_used ? 'Yes' : '-'}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Attempt</h4>
                    <p>{selectedRequest.attempt ?? '-'}</p>
                  </div>
                </div>
              </div>

              <div className="border rounded-lg p-4 bg-muted/50">
                <h3 className="font-semibold mb-4">Diagnostics</h3>
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div className="col-span-2">
                    <h4 className="font-medium mb-1">Decision Reason</h4>
                    <p className="text-muted-foreground">{selectedRequest.decision_reason || '—'}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Error Type</h4>
                    {selectedRequest.error_type ? (
                      <Badge variant="destructive">{selectedRequest.error_type}</Badge>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">PII Request Decision</h4>
                    {selectedRequest.pii_webhook_request_decision ? (
                      <Badge variant="outline">{selectedRequest.pii_webhook_request_decision}</Badge>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">PII Response Decision</h4>
                    {selectedRequest.pii_webhook_response_decision ? (
                      <Badge variant="outline">{selectedRequest.pii_webhook_response_decision}</Badge>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </div>
                  {selectedRequest.metadata && Object.keys(selectedRequest.metadata).length > 0 && (
                    <div className="col-span-2">
                      <h4 className="font-medium mb-2">Metadata</h4>
                      <pre className="bg-background p-3 rounded-md text-xs overflow-auto max-h-40">
                        {JSON.stringify(selectedRequest.metadata, null, 2)}
                      </pre>
                    </div>
                  )}
                </div>
              </div>

              {routingSnapshotLoading && <Skeleton className="h-40" />}

              {!routingSnapshotLoading && routingDetails && (
                <div className="border rounded-lg p-4 bg-muted/50 space-y-4">
                  <h3 className="font-semibold">Routing Snapshot</h3>
                  <div className="grid grid-cols-2 gap-4 text-sm">
                    <div>
                      <h4 className="font-medium mb-1">Selected Model</h4>
                      <p>{routingDetails.selected_model || '-'}</p>
                    </div>
                    <div>
                      <h4 className="font-medium mb-1">Provider</h4>
                      <p>{routingDetails.provider || '-'}</p>
                    </div>
                    <div>
                      <h4 className="font-medium mb-1">Routing Strategy</h4>
                      <p>{routingDetails.routing_strategy || '-'}</p>
                    </div>
                    <div>
                      <h4 className="font-medium mb-1">Fallback Attempts</h4>
                      <p>{routingDetails.fallback_attempts ?? '-'}</p>
                    </div>
                    <div>
                      <h4 className="font-medium mb-1">Cost Optimizer</h4>
                      <p>{routingDetails.cost_optimizer_applied ? 'Yes' : 'No'}</p>
                    </div>
                  </div>

                  {Array.isArray(routingDetails.candidate_models) && routingDetails.candidate_models.length > 0 && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Candidate Models</h4>
                      <div className="flex flex-wrap gap-2">
                        {routingDetails.candidate_models.map((candidate) => (
                          <Badge
                            key={candidate}
                            variant={candidate === routingDetails.selected_model ? 'default' : 'outline'}
                          >
                            {candidate}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}

                  {routingDetails.estimated_costs_usd && Object.keys(routingDetails.estimated_costs_usd).length > 0 && (
                    <div>
                      <h4 className="text-sm font-medium mb-2">Estimated Costs (USD)</h4>
                      <div className="border rounded-md">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>Model</TableHead>
                              <TableHead>Estimated Cost USD</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {Object.entries(routingDetails.estimated_costs_usd).map(([model, cost]) => (
                              <TableRow key={model}>
                                <TableCell>{model}</TableCell>
                                <TableCell className="tabular-nums">{cost}</TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </div>
                    </div>
                  )}
                </div>
              )}

              {(selectedRequest.routing_snapshot || selectedRequest.decision_snapshot) && (
                <div className="border rounded-lg p-4 bg-muted/50">
                  <h3 className="font-semibold mb-4">Snapshots</h3>
                  <Accordion type="single" collapsible className="w-full">
                    {selectedRequest.routing_snapshot && (
                      <AccordionItem value="routing-snapshot">
                        <AccordionTrigger className="text-sm font-medium">
                          Routing Snapshot (JSON)
                        </AccordionTrigger>
                        <AccordionContent>
                          <pre className="bg-background p-3 rounded-md text-xs overflow-auto max-h-96">
                            {JSON.stringify(selectedRequest.routing_snapshot, null, 2)}
                          </pre>
                        </AccordionContent>
                      </AccordionItem>
                    )}
                    {selectedRequest.decision_snapshot && (
                      <AccordionItem value="decision-snapshot">
                        <AccordionTrigger className="text-sm font-medium">
                          Decision Snapshot (JSON)
                        </AccordionTrigger>
                        <AccordionContent>
                          <pre className="bg-background p-3 rounded-md text-xs overflow-auto max-h-96">
                            {JSON.stringify(selectedRequest.decision_snapshot, null, 2)}
                          </pre>
                        </AccordionContent>
                      </AccordionItem>
                    )}
                  </Accordion>
                </div>
              )}

              {showRawJson && (
                <div>
                  <h4 className="text-sm font-medium mb-2">Raw Request JSON</h4>
                  <pre className="bg-background p-3 rounded-md text-xs overflow-auto max-h-64">
                    {JSON.stringify(selectedRequest.raw_request ?? selectedRequest, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </SheetContent>
      </Sheet>

      <Dialog open={replayModalOpen} onOpenChange={setReplayModalOpen}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Replay Result</DialogTitle>
            <DialogDescription>Request ID: {replayData?.request_id || selectedRequest?.request_id}</DialogDescription>
          </DialogHeader>

          {replayData && (
            <div className="space-y-4">
              <div className="border rounded-lg p-4 bg-muted/50 space-y-4">
                <h3 className="font-semibold">Replay Routing Snapshot</h3>
                <div>
                  <h4 className="font-medium mb-1">Snapshot Time</h4>
                  <p className="text-sm">{formatSnapshotTimestamp(replayDetails?.timestamp)}</p>
                </div>
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div>
                    <h4 className="font-medium mb-1">Selected Model</h4>
                    <p>{replaySelectedModel || '-'}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Provider</h4>
                    <p>{replayData.provider || replayDetails?.provider || '-'}</p>
                  </div>
                  <div>
                    <h4 className="font-medium mb-1">Routing Strategy</h4>
                    <p>{replayDetails?.routing_strategy || '-'}</p>
                  </div>
                </div>

                {Array.isArray(replayDetails?.candidate_models) && replayDetails.candidate_models.length > 0 && (
                  <div>
                    <h4 className="text-sm font-medium mb-2">Candidate Models</h4>
                    <div className="flex flex-wrap gap-2">
                      {replayDetails.candidate_models.map((candidate) => (
                        <Badge
                          key={candidate}
                          variant={
                            candidate === (replayData.selected_model || replayDetails.selected_model)
                              ? 'default'
                              : 'outline'
                          }
                        >
                          {candidate}
                        </Badge>
                      ))}
                    </div>
                  </div>
                )}

                {replayDetails?.estimated_costs_usd && Object.keys(replayDetails.estimated_costs_usd).length > 0 && (
                  <div>
                    <h4 className="text-sm font-medium mb-2">Estimated Costs (USD)</h4>
                    <div className="border rounded-md">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>Model</TableHead>
                            <TableHead>Estimated Cost USD</TableHead>
                            <TableHead>Δ vs Selected</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {Object.entries(replayDetails.estimated_costs_usd).map(([model, cost]) => (
                            <TableRow
                              key={model}
                              className={model === replaySelectedModel ? 'bg-muted/60 font-semibold' : undefined}
                            >
                              <TableCell>
                                <div className="flex items-center gap-2">
                                  <span>{model}</span>
                                  {model === replaySelectedModel && <Badge variant="secondary">selected</Badge>}
                                </div>
                              </TableCell>
                              <TableCell className="tabular-nums">{formatUsdCost(cost)}</TableCell>
                              <TableCell className="tabular-nums">
                                {formatDeltaVsSelected(cost, replaySelectedCost, model === replaySelectedModel)}
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  </div>
                )}
              </div>

              {/* SPEC_68: Replay Diagnostics Section */}
              <div className="border rounded-lg p-4 bg-muted/50">
                <h3 className="font-semibold mb-4">Replay Diagnostics</h3>
                <div className="space-y-4">
                  <div>
                    <h4 className="font-medium mb-1 text-sm">Decision Reason</h4>
                    <p className="text-muted-foreground text-sm">
                      {replayData.decision_reason || '—'}
                    </p>
                  </div>

                  {(replayData.decision_snapshot || replayData.routing_snapshot_full) && (
                    <Accordion type="single" collapsible className="w-full">
                      {replayData.decision_snapshot && (
                        <AccordionItem value="decision-snapshot">
                          <AccordionTrigger className="text-sm font-medium">
                            Decision Snapshot
                          </AccordionTrigger>
                          <AccordionContent>
                            <pre className="bg-background p-3 rounded-md text-xs overflow-auto max-h-96 font-mono">
                              {JSON.stringify(replayData.decision_snapshot, null, 2)}
                            </pre>
                          </AccordionContent>
                        </AccordionItem>
                      )}
                      {replayData.routing_snapshot_full && (
                        <AccordionItem value="routing-snapshot-full">
                          <AccordionTrigger className="text-sm font-medium">
                            Routing Snapshot
                          </AccordionTrigger>
                          <AccordionContent>
                            <pre className="bg-background p-3 rounded-md text-xs overflow-auto max-h-96 font-mono">
                              {JSON.stringify(replayData.routing_snapshot_full, null, 2)}
                            </pre>
                          </AccordionContent>
                        </AccordionItem>
                      )}
                    </Accordion>
                  )}
                </div>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
