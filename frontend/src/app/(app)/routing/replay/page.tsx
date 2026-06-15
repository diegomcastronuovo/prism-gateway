'use client'

import { useState, useEffect, useCallback } from 'react'
import { useSearchParams } from 'next/navigation'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { useAuth } from '@/hooks/use-auth'
import { useReplayGlobalAccess } from '@/features/routing/api/use-replay-access'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { AlertCircle, CheckCircle2, Copy, ChevronDown, ChevronUp, ShieldAlert } from 'lucide-react'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Alert, AlertDescription } from '@/components/ui/alert'

type RoutingSnapshot = {
  provider: string
  timestamp: string
  route_group: string
  selected_model: string
  candidate_models: string[]
  routing_strategy: string
  fallback_attempts: number
}

type RoutingSnapshotResponse = {
  request_id: string
  routing_snapshot: RoutingSnapshot
  tenant_id: string
}

type ReplayResponse = {
  mode: 'deterministic'
  provider: string
  request_id: string
  routing_snapshot: RoutingSnapshot
  selected_model: string
  tenant_id: string
  // SPEC_69: Diagnostic fields
  decision_reason?: string
  decision_snapshot?: Record<string, unknown>
  routing_snapshot_full?: Record<string, unknown>
}

export default function ReplayPage() {
  const searchParams = useSearchParams()
  const { user, isRefreshingSession } = useAuth()
  const accessQuery = useReplayGlobalAccess()

  const [requestId, setRequestId] = useState('')
  const [snapshot, setSnapshot] = useState<RoutingSnapshotResponse | null>(null)
  const [replay, setReplay] = useState<ReplayResponse | null>(null)
  const [loadingSnapshot, setLoadingSnapshot] = useState(false)
  const [loadingReplay, setLoadingReplay] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showSnapshotJson, setShowSnapshotJson] = useState(false)
  const [showReplayJson, setShowReplayJson] = useState(false)

  const handleLoadSnapshotForId = useCallback(async (id: string) => {
    setError(null)
    setLoadingSnapshot(true)
    setSnapshot(null)
    setReplay(null)
    try {
      const res = await fetch(`/api/routing/requests/${encodeURIComponent(id)}/routing`, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: { message: 'Failed to load snapshot' } }))
        const msg = err.error?.message || 'Routing snapshot not found for this request ID'
        throw new Error(msg)
      }
      const data = await res.json()
      setSnapshot(data)
    } catch (err) {
      const baseMsg = err instanceof Error ? err.message : 'Unable to load replay data for this request.'
      setError(`${baseMsg}\n\nThis may happen if:\n- the request predates snapshot support\n- the request ID is incorrect\n- the request belongs to another tenant`)
    } finally {
      setLoadingSnapshot(false)
    }
  }, [])

  const handleLoadSnapshot = useCallback(async () => {
    if (!requestId.trim()) {
      setError('Please enter a request ID')
      return
    }
    await handleLoadSnapshotForId(requestId.trim())
  }, [requestId, handleLoadSnapshotForId])

  const handleReplay = useCallback(async () => {
    if (!requestId.trim()) {
      setError('Please enter a request ID')
      return
    }
    setError(null)
    setLoadingReplay(true)
    setReplay(null)
    try {
      const res = await fetch(`/api/routing/replay/${encodeURIComponent(requestId.trim())}`, {
        method: 'POST',
        credentials: 'include',
        cache: 'no-store',
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: { message: 'Failed to run replay' } }))
        throw new Error(err.error?.message || 'Replay failed. Check permissions or request availability.')
      }
      const data = await res.json()
      setReplay(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Replay failed. Check permissions or request availability.')
    } finally {
      setLoadingReplay(false)
    }
  }, [requestId])

  const handleClear = () => {
    setRequestId('')
    setSnapshot(null)
    setReplay(null)
    setError(null)
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
  }

  useEffect(() => {
    if (!user) return
    if (!accessQuery.isSuccess) return
    const urlRequestId = searchParams.get('request_id')
    if (urlRequestId?.trim()) {
      setRequestId(urlRequestId.trim())
      void handleLoadSnapshotForId(urlRequestId.trim())
    }
  }, [user, accessQuery.isSuccess, searchParams, handleLoadSnapshotForId])

  if (user && isRefreshingSession) {
    return (
      <div className="container mx-auto py-6 space-y-6">
        <PageHeader
          title="Replay"
          description="Inspect historical routing decisions and replay them deterministically for debug and audit."
        />
        <Card className="border-t-4 border-t-slate-500">
          <CardContent className="py-6 space-y-3">
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-3/4" />
          </CardContent>
        </Card>
      </div>
    )
  }

  if (user && accessQuery.isError) {
    const err = accessQuery.error as Error & { status?: number }
    const isAccessDenied = err.status === 403 || err.status === 401
    return (
      <div className="container mx-auto py-6">
        <PageHeader
          title="Replay"
          description="Inspect historical routing decisions and replay them deterministically for debug and audit."
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
              <p className="text-destructive mb-2">Unable to verify access</p>
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
      <div className="container mx-auto py-6 space-y-6">
        <PageHeader
          title="Replay"
          description="Inspect historical routing decisions and replay them deterministically for debug and audit."
        />
        <Card className="border-t-4 border-t-slate-500">
          <CardContent className="py-6 space-y-3">
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-3/4" />
          </CardContent>
        </Card>
      </div>
    )
  }

  const getStatusBadge = () => {
    if (error) return <Badge variant="destructive">Error</Badge>
    if (replay) return <Badge variant="default">Replay Completed</Badge>
    if (snapshot) return <Badge variant="secondary">Snapshot Loaded</Badge>
    return <Badge variant="outline">Idle</Badge>
  }

  const compareFields = () => {
    if (!snapshot || !replay) return []
    const s = snapshot.routing_snapshot
    const r = replay.routing_snapshot
    return [
      { field: 'request_id', snapshot: snapshot.request_id, replay: replay.request_id },
      { field: 'tenant_id', snapshot: snapshot.tenant_id, replay: replay.tenant_id },
      { field: 'provider', snapshot: s.provider, replay: r.provider },
      { field: 'selected_model', snapshot: s.selected_model, replay: r.selected_model },
      { field: 'route_group', snapshot: s.route_group, replay: r.route_group },
      { field: 'routing_strategy', snapshot: s.routing_strategy, replay: r.routing_strategy },
      { field: 'fallback_attempts', snapshot: String(s.fallback_attempts), replay: String(r.fallback_attempts) },
    ]
  }

  const allMatch = compareFields().every((row) => row.snapshot === row.replay)
  const replayRoutingSnapshotForDiagnostics: Record<string, unknown> | undefined =
    replay?.routing_snapshot_full ??
    (replay?.routing_snapshot ? (replay.routing_snapshot as unknown as Record<string, unknown>) : undefined)
  const replayDecisionSnapshotForDiagnostics: Record<string, unknown> | undefined =
    replay?.decision_snapshot
  const replayDecisionReason =
    replay?.decision_reason ||
    (typeof replayDecisionSnapshotForDiagnostics?.reason === 'string'
      ? replayDecisionSnapshotForDiagnostics.reason
      : '—')

  return (
    <div className="container mx-auto py-6 space-y-6">
      <div>
        <h1 className="text-3xl font-bold">Replay</h1>
        <p className="text-muted-foreground mt-2">
          Inspect historical routing decisions and replay them deterministically for debug and audit.
        </p>
      </div>

      <Card className="border-t-4 border-t-pink-500">
        <CardHeader>
          <CardTitle className="text-base">What Replay Does</CardTitle>
          <CardDescription>
            Replay reruns the historical routing decision for a request and compares the result with the stored routing snapshot. This helps debug why a request was routed to a specific model.
          </CardDescription>
        </CardHeader>
      </Card>

      <Card className="border-t-4 border-t-cyan-400">
        <CardHeader>
          <CardTitle>Request Selector</CardTitle>
          <CardDescription>Enter a request ID to load its routing snapshot and replay it</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Input
                placeholder="e.g. chatcmpl-mock-1773054629"
                value={requestId}
                onChange={(e) => setRequestId(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleLoadSnapshot()}
              />
              {getStatusBadge()}
            </div>
            <p className="text-sm text-muted-foreground">
              Paste a request ID from Routing Insights or Request Explorer. Example: chatcmpl-abc123
            </p>
          </div>
          <div className="flex gap-2">
            <Button onClick={handleLoadSnapshot} disabled={loadingSnapshot || !requestId.trim()}>
              {loadingSnapshot ? 'Loading...' : 'Load Snapshot'}
            </Button>
            <Button onClick={handleReplay} disabled={loadingReplay || !snapshot} variant="secondary">
              {loadingReplay ? 'Running...' : 'Replay Deterministically'}
            </Button>
            <Button onClick={handleClear} variant="outline">
              Clear
            </Button>
            {requestId.trim() && (
              <Button onClick={() => copyToClipboard(requestId.trim())} variant="ghost" size="icon">
                <Copy className="h-4 w-4" />
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {!snapshot && !loadingSnapshot && !error && (
        <>
          <Card className="border-t-4 border-t-purple-500">
            <CardContent className="py-12 text-center text-muted-foreground">
              Enter a request ID from Routing Insights or Request Explorer to inspect routing history and run deterministic replay.
            </CardContent>
          </Card>

          <Card className="border-t-4 border-t-amber-400">
            <CardHeader>
              <CardTitle className="text-base">Where to get the request ID</CardTitle>
            </CardHeader>
            <CardContent>
              <ul className="space-y-2 text-sm text-muted-foreground">
                <li>• <strong>Routing Insights</strong> — Click Replay on any request row</li>
                <li>• <strong>Request Explorer</strong> — Copy the request ID from request details</li>
                <li>• <strong>Raw request logs</strong> — Extract from your logging system</li>
              </ul>
              {!snapshot && (
                <p className="mt-4 text-sm text-muted-foreground">
                  Load a routing snapshot before running replay.
                </p>
              )}
            </CardContent>
          </Card>
        </>
      )}

      {loadingSnapshot && (
        <Card className="border-t-4 border-t-slate-500">
          <CardContent className="py-6 space-y-3">
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-3/4" />
          </CardContent>
        </Card>
      )}

      {snapshot && (
        <>
          <Card className="border-t-4 border-t-purple-500">
            <CardHeader>
              <CardTitle>Snapshot Summary</CardTitle>
              <CardDescription>
                This snapshot represents the routing decision stored at the time the request was processed.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div>
                  <div className="text-xs text-muted-foreground">Request ID</div>
                  <div className="font-medium">{snapshot.request_id}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Tenant</div>
                  <div className="font-medium">{snapshot.tenant_id}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Provider</div>
                  <div className="font-medium">{snapshot.routing_snapshot.provider}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Selected Model</div>
                  <div className="font-medium">{snapshot.routing_snapshot.selected_model}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Route Group</div>
                  <div className="font-medium">{snapshot.routing_snapshot.route_group}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground" title="Routing strategy defines how the router selects a model among candidates. Strategies may consider cost, latency, or error rates.">Routing Strategy</div>
                  <div className="font-medium">{snapshot.routing_snapshot.routing_strategy}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Fallback Attempts</div>
                  <div className="font-medium">{snapshot.routing_snapshot.fallback_attempts}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Timestamp</div>
                  <div className="font-medium text-xs">{new Date(snapshot.routing_snapshot.timestamp).toLocaleString()}</div>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="border-t-4 border-t-blue-500">
            <CardHeader>
              <CardTitle>Candidate Models</CardTitle>
              <CardDescription>
                Candidate models are the models that were eligible for routing at the time of the request. The router selected one of these models based on the routing strategy.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap gap-2">
                {snapshot.routing_snapshot.candidate_models.map((model, idx) => {
                  const isSelected = model === snapshot.routing_snapshot.selected_model
                  return (
                    <Badge key={idx} variant={isSelected ? 'default' : 'secondary'} className="gap-1">
                      {model}
                      {isSelected && <span className="text-xs">selected</span>}
                    </Badge>
                  )
                })}
              </div>
            </CardContent>
          </Card>

          <Card className="border-t-4 border-t-slate-500">
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>Snapshot JSON</CardTitle>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowSnapshotJson(!showSnapshotJson)}
                >
                  {showSnapshotJson ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                </Button>
              </div>
            </CardHeader>
            {showSnapshotJson && (
              <CardContent>
                <div className="relative">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="absolute top-2 right-2"
                    onClick={() => copyToClipboard(JSON.stringify(snapshot, null, 2))}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                  <pre className="bg-muted p-4 rounded-md overflow-x-auto text-xs">
                    {JSON.stringify(snapshot, null, 2)}
                  </pre>
                </div>
              </CardContent>
            )}
          </Card>
        </>
      )}

      {loadingReplay && (
        <Card className="border-t-4 border-t-slate-500">
          <CardContent className="py-6 space-y-3">
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-full" />
            <Skeleton className="h-6 w-3/4" />
          </CardContent>
        </Card>
      )}

      {replay && (
        <>
          <Card className="border-t-4 border-t-emerald-500">
            <CardHeader>
              <CardTitle>Replay Result</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div>
                  <div className="text-xs text-muted-foreground">Mode</div>
                  <div className="font-medium">{replay.mode}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Provider</div>
                  <div className="font-medium">{replay.provider}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Selected Model</div>
                  <div className="font-medium">{replay.selected_model}</div>
                </div>
                <div>
                  <div className="text-xs text-muted-foreground">Tenant</div>
                  <div className="font-medium">{replay.tenant_id}</div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* SPEC_69: Decision Reason */}
          <Card className="border-t-4 border-t-amber-400">
            <CardHeader>
              <CardTitle>Decision Reason</CardTitle>
              <CardDescription>
                Explains why the router chose this provider/model during replay
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="text-sm">
                <span className="font-medium">Reason: </span>
                <span className="text-muted-foreground">{replayDecisionReason}</span>
              </div>
            </CardContent>
          </Card>

          <Card className="border-t-4 border-t-indigo-500">
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>Snapshot vs Replay Comparison</CardTitle>
                {allMatch ? (
                  <Badge variant="default" className="gap-1">
                    <CheckCircle2 className="h-3 w-3" />
                    Match
                  </Badge>
                ) : (
                  <Badge variant="destructive" className="gap-1">
                    <AlertCircle className="h-3 w-3" />
                    Diff
                  </Badge>
                )}
              </div>
              <CardDescription>
                {allMatch
                  ? 'Match means the deterministic replay selected the same model as the original routing decision.'
                  : 'Diff detected: The replay selected a different model than the original routing decision. This may happen if routing configuration has changed since the request occurred.'}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="border-b">
                    <tr>
                      <th className="text-left py-2 px-3">Field</th>
                      <th className="text-left py-2 px-3">Snapshot</th>
                      <th className="text-left py-2 px-3">Replay</th>
                    </tr>
                  </thead>
                  <tbody>
                    {compareFields().map((row) => {
                      const match = row.snapshot === row.replay
                      return (
                        <tr key={row.field} className={match ? '' : 'bg-destructive/10'}>
                          <td className="py-2 px-3 font-medium">{row.field}</td>
                          <td className="py-2 px-3">{row.snapshot}</td>
                          <td className="py-2 px-3">{row.replay}</td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          {/* SPEC_69: Routing Snapshot and Decision Snapshot Accordions */}
          <Card className="border-t-4 border-t-indigo-500">
            <CardHeader>
              <CardTitle>Replay Diagnostics</CardTitle>
              <CardDescription>
                Detailed routing context and decision snapshots from the replay execution
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Accordion type="single" collapsible className="w-full">
                <AccordionItem value="routing-snapshot">
                  <AccordionTrigger className="text-sm font-medium">
                    Routing Snapshot
                  </AccordionTrigger>
                  <AccordionContent>
                    {replayRoutingSnapshotForDiagnostics ? (
                      <div className="relative">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="absolute top-2 right-2 z-10"
                          onClick={() => copyToClipboard(JSON.stringify(replayRoutingSnapshotForDiagnostics, null, 2))}
                        >
                          <Copy className="h-4 w-4" />
                        </Button>
                        <pre className="bg-muted p-4 rounded-md overflow-x-auto text-xs font-mono max-h-96">
                          {JSON.stringify(replayRoutingSnapshotForDiagnostics, null, 2)}
                        </pre>
                      </div>
                    ) : (
                      <p className="text-sm text-muted-foreground">— No routing snapshot available</p>
                    )}
                  </AccordionContent>
                </AccordionItem>
                <AccordionItem value="decision-snapshot">
                  <AccordionTrigger className="text-sm font-medium">
                    Decision Snapshot
                  </AccordionTrigger>
                  <AccordionContent>
                    {replayDecisionSnapshotForDiagnostics ? (
                      <div className="relative">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="absolute top-2 right-2 z-10"
                          onClick={() => copyToClipboard(JSON.stringify(replayDecisionSnapshotForDiagnostics, null, 2))}
                        >
                          <Copy className="h-4 w-4" />
                        </Button>
                        <pre className="bg-muted p-4 rounded-md overflow-x-auto text-xs font-mono max-h-96">
                          {JSON.stringify(replayDecisionSnapshotForDiagnostics, null, 2)}
                        </pre>
                      </div>
                    ) : (
                      <p className="text-sm text-muted-foreground">— No decision snapshot available</p>
                    )}
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </CardContent>
          </Card>

          <Card className="border-t-4 border-t-slate-500">
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>Replay JSON</CardTitle>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowReplayJson(!showReplayJson)}
                >
                  {showReplayJson ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                </Button>
              </div>
            </CardHeader>
            {showReplayJson && (
              <CardContent>
                <div className="relative">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="absolute top-2 right-2"
                    onClick={() => copyToClipboard(JSON.stringify(replay, null, 2))}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                  <pre className="bg-muted p-4 rounded-md overflow-x-auto text-xs">
                    {JSON.stringify(replay, null, 2)}
                  </pre>
                </div>
              </CardContent>
            )}
          </Card>
        </>
      )}
    </div>
  )
}
