'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
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
import { useToast } from '@/hooks/use-toast'
import { listSemanticAnchors } from '@/features/semantic/api/client'
import { useTenants, useTenantConfig, type TenantConfig } from '@/features/tenants/api/use-tenants'
import { Route, Sparkles } from 'lucide-react'

type ToolRoute = {
  name: string
  description?: string
  action: string
  utterances?: string[] | null
  threshold?: number | null
}

type ToolRoutingTestResult = {
  status: 'matched' | 'no_match' | 'error'
  matched: boolean
  route?: string
  action?: string
  similarity?: number
  skippedModelRouting: boolean
  rawContent?: string
  errorMessage?: string
}

type CreateRouteState = {
  name: string
  action: string
  description: string
  threshold: string
  utterances: string
}

function parseToolRoutingResult(payload: unknown, headers: Headers): ToolRoutingTestResult {
  // Preferred signal: response headers
  const toolRoute = headers.get('x-tool-route')
  const toolAction = headers.get('x-tool-action')
  const similarityHeader = headers.get('x-tool-route-similarity')

  if (toolRoute || toolAction) {
    return {
      status: 'matched',
      matched: true,
      route: toolRoute ?? undefined,
      action: toolAction ?? undefined,
      similarity: similarityHeader ? Number(similarityHeader) : undefined,
      skippedModelRouting: true,
    }
  }

  // Secondary signal: body model field
  const model = typeof payload === 'object' && payload && 'model' in payload
    ? String((payload as { model?: string }).model ?? '')
    : ''

  if (model === 'tool-route') {
    const choices = typeof payload === 'object' && payload && 'choices' in payload
      ? (payload as { choices?: Array<{ message?: { content?: string } }> }).choices
      : []
    const content = Array.isArray(choices) && choices.length > 0 ? choices[0]?.message?.content ?? '' : ''
    let contentJson: Record<string, unknown> | null = null
    if (typeof content === 'string' && content.trim().startsWith('{')) {
      try { contentJson = JSON.parse(content) } catch { contentJson = null }
    }
    return {
      status: 'matched',
      matched: true,
      route: contentJson && typeof contentJson.route === 'string' ? contentJson.route : undefined,
      action: contentJson && typeof contentJson.tool === 'string' ? contentJson.tool : undefined,
      similarity: contentJson && typeof contentJson.similarity === 'number' ? contentJson.similarity : undefined,
      skippedModelRouting: true,
      rawContent: typeof content === 'string' ? content : undefined,
    }
  }

  // Case B: response OK but no tool route matched
  return {
    status: 'no_match',
    matched: false,
    skippedModelRouting: false,
  }
}

function ToolsContent() {
  const { toast } = useToast()
  const queryClient = useQueryClient()
  const tenantsQuery = useTenants()
  const [selectedTenantId, setSelectedTenantId] = useState<string | null>(null)
  const tenantConfigQuery = useTenantConfig(selectedTenantId)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [createForm, setCreateForm] = useState<CreateRouteState>({
    name: '',
    action: '',
    description: '',
    threshold: '0.80',
    utterances: '',
  })
  const [routeToDelete, setRouteToDelete] = useState<ToolRoute | null>(null)
  const [routeToEdit, setRouteToEdit] = useState<ToolRoute | null>(null)
  const [editForm, setEditForm] = useState<CreateRouteState>({
    name: '',
    action: '',
    description: '',
    threshold: '0.80',
    utterances: '',
  })
  const [editUtterancesLoading, setEditUtterancesLoading] = useState(false)
  const [testPrompt, setTestPrompt] = useState('')
  const [testModel, setTestModel] = useState('')
  const [testResult, setTestResult] = useState<ToolRoutingTestResult | null>(null)

  useEffect(() => {
    if (!selectedTenantId && tenantsQuery.data?.length) {
      setSelectedTenantId(tenantsQuery.data[0].tenant_id)
    }
  }, [selectedTenantId, tenantsQuery.data])

  useEffect(() => {
    setTestResult(null)
  }, [selectedTenantId])

  const { data: toolRoutesResponse, isLoading: isLoadingRoutes, error: routesError } = useQuery({
    queryKey: ['tool-routes', selectedTenantId],
    queryFn: async () => {
      const res = await fetch(`/api/semantic/routes?tenant_id=${encodeURIComponent(selectedTenantId ?? '')}`, {
        cache: 'no-store',
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        throw new Error(payload.error || 'Failed to fetch tool routes')
      }
      return res.json() as Promise<{ data?: ToolRoute[] }>
    },
    enabled: Boolean(selectedTenantId),
  })

  const toolRoutes = toolRoutesResponse?.data ?? []

  const semanticAnchorsQuery = useQuery({
    queryKey: ['semantic-anchors-summary', selectedTenantId],
    queryFn: () => listSemanticAnchors({ limit: 50, includeAnchorText: false, tenantId: selectedTenantId }),
    enabled: Boolean(selectedTenantId),
  })

  const createRouteMutation = useMutation({
    mutationFn: async () => {
      if (!selectedTenantId) throw new Error('Tenant required')
      const utterances = createForm.utterances
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean)
      const thresholdNum = Number(createForm.threshold)
      const res = await fetch(`/api/semantic/routes?tenant_id=${encodeURIComponent(selectedTenantId)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: createForm.name.trim(),
          action: createForm.action.trim(),
          description: createForm.description.trim() || undefined,
          threshold: thresholdNum,
          utterances,
        }),
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        throw new Error(payload.error || 'Failed to create tool route')
      }
      return res.json()
    },
    onSuccess: () => {
      toast({ title: 'Tool route created' })
      setCreateDialogOpen(false)
      setCreateForm({ name: '', action: '', description: '', threshold: '0.80', utterances: '' })
      queryClient.invalidateQueries({ queryKey: ['tool-routes', selectedTenantId] })
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to create tool route', description: error.message, variant: 'destructive' })
    },
  })

  const deleteRouteMutation = useMutation({
    mutationFn: async () => {
      if (!selectedTenantId || !routeToDelete) throw new Error('Tenant required')
      const res = await fetch(
        `/api/semantic/routes/${encodeURIComponent(routeToDelete.name)}?tenant_id=${encodeURIComponent(selectedTenantId)}`,
        { method: 'DELETE' }
      )
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        throw new Error(payload.error || 'Failed to delete tool route')
      }
      return res.json()
    },
    onSuccess: () => {
      toast({ title: 'Tool route deleted' })
      setRouteToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['tool-routes', selectedTenantId] })
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to delete tool route', description: error.message, variant: 'destructive' })
    },
  })

  const editRouteMutation = useMutation({
    mutationFn: async () => {
      if (!selectedTenantId || !routeToEdit) throw new Error('Tenant required')
      const utterances = editForm.utterances
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean)
      const thresholdNum = Number(editForm.threshold)
      const res = await fetch(
        `/api/semantic/routes/${encodeURIComponent(routeToEdit.name)}?tenant_id=${encodeURIComponent(selectedTenantId)}`,
        {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            description: editForm.description.trim() || undefined,
            action: editForm.action.trim(),
            threshold: thresholdNum,
            utterances,
          }),
        }
      )
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        throw new Error(payload.error || 'Failed to update tool route')
      }
      return res.json()
    },
    onSuccess: () => {
      toast({ title: 'Tool route updated' })
      setRouteToEdit(null)
      queryClient.invalidateQueries({ queryKey: ['tool-routes', selectedTenantId] })
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to update tool route', description: error.message, variant: 'destructive' })
    },
  })

  async function openEditDialog(route: ToolRoute) {
    // Pre-fill immediately with list data (utterances will be empty — list never includes them)
    setEditForm({
      name: route.name,
      action: route.action,
      description: route.description ?? '',
      threshold: route.threshold != null ? String(route.threshold) : '0.80',
      utterances: '',
    })
    setRouteToEdit(route)
    // Fetch full route detail (including utterances) via GET /api/semantic/routes/[name]
    setEditUtterancesLoading(true)
    try {
      const res = await fetch(
        `/api/semantic/routes/${encodeURIComponent(route.name)}?tenant_id=${encodeURIComponent(selectedTenantId ?? '')}`,
        { cache: 'no-store' }
      )
      if (res.ok) {
        const detail = await res.json() as { utterances?: string[]; name?: string }
        const utterances = Array.isArray(detail.utterances) ? detail.utterances : []
        setEditForm((prev) => ({ ...prev, utterances: utterances.join('\n') }))
      }
    } catch {
      // fail silently — user can still edit utterances manually
    } finally {
      setEditUtterancesLoading(false)
    }
  }

  const testMutation = useMutation({
    mutationFn: async () => {
      const model = testModel.trim()
      const res = await fetch(
        `/api/tool-routing/test?tenant_id=${encodeURIComponent(selectedTenantId ?? '')}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            model,
            messages: [{ role: 'user', content: testPrompt.trim() }],
          }),
        }
      )
      const payload = await res.json().catch(() => ({}))
      if (!res.ok) {
        const message = typeof payload?.error === 'string' ? payload.error : `Request failed (${res.status})`
        throw new Error(message)
      }
      return { payload, headers: res.headers }
    },
    onSuccess: (data) => {
      setTestResult(parseToolRoutingResult(data.payload, data.headers))
    },
    onError: (error: Error) => {
      setTestResult({
        status: 'error',
        matched: false,
        skippedModelRouting: false,
        errorMessage: error.message,
      })
    },
  })

  const tenantConfig = tenantConfigQuery.data as TenantConfig | undefined
  const config = tenantConfig?.config as Record<string, unknown> | undefined
  const routing = (config?.routing as Record<string, unknown>) || {}
  const fallback = (routing.fallback as Record<string, unknown>) || {}
  const semantic = (routing.semantic as Record<string, unknown>) || {}
  const fallbackEnabled = Boolean(fallback.enabled ?? false)
  const thresholdDefault = typeof semantic.threshold_default === 'number' ? semantic.threshold_default : null
  const anchorCount = semanticAnchorsQuery.data?.data?.length ?? 0
  const allowedModels = useMemo(() => {
    const raw = config?.allowed_models
    return Array.isArray(raw) ? raw.filter((model) => typeof model === 'string') : []
  }, [config])

  useEffect(() => {
    if (!testModel && allowedModels.length > 0) {
      setTestModel(allowedModels[0])
    }
  }, [allowedModels, testModel])

  const thresholdNum = Number(createForm.threshold)
  const thresholdInvalid = createForm.threshold.trim() === '' || !Number.isFinite(thresholdNum) || thresholdNum < 0 || thresholdNum > 1
  const createDisabled =
    !selectedTenantId ||
    !createForm.name.trim() ||
    !createForm.action.trim() ||
    thresholdInvalid ||
    createForm.utterances.split('\n').map((line) => line.trim()).filter(Boolean).length === 0

  const editThresholdNum = Number(editForm.threshold)
  const editThresholdInvalid = editForm.threshold.trim() === '' || !Number.isFinite(editThresholdNum) || editThresholdNum < 0 || editThresholdNum > 1
  const editDisabled =
    !selectedTenantId ||
    !editForm.name.trim() ||
    !editForm.action.trim() ||
    editThresholdInvalid ||
    editForm.utterances.split('\n').map((line) => line.trim()).filter(Boolean).length === 0

  const testDisabled = !selectedTenantId || !testPrompt.trim() || !testModel.trim()

  const isToolRoutingEnabled = (config?.tool_routing_enabled as boolean | null | undefined) !== false

  return (
    <div>
      <PageHeader
        title="Tool Routing"
        description="Define semantic tool/action routes that can short-circuit model inference. This configuration is tenant-specific."
        action={
          <div className="flex w-full max-w-full flex-col gap-3 sm:w-auto sm:max-w-none sm:flex-row sm:items-end sm:justify-end sm:gap-3">
            <div className="w-full min-w-0 sm:w-60">
              <Label htmlFor="tool-tenant" className="text-xs text-muted-foreground">
                Tenant
              </Label>
              <Select
                value={selectedTenantId ?? ''}
                onValueChange={(value) => setSelectedTenantId(value)}
              >
                <SelectTrigger id="tool-tenant" className="mt-1">
                  <SelectValue placeholder="Select a tenant" />
                </SelectTrigger>
                <SelectContent>
                  {(tenantsQuery.data ?? []).map((tenant) => (
                    <SelectItem key={tenant.tenant_id} value={tenant.tenant_id}>
                      {tenant.tenant_id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {selectedTenantId && (
              <div className="flex flex-wrap gap-2">
                <Badge variant="secondary">Routes: {toolRoutes.length}</Badge>
                <Badge variant="secondary">
                  Threshold: {thresholdDefault != null ? thresholdDefault.toFixed(2) : '—'}
                </Badge>
                <Badge variant={fallbackEnabled ? 'default' : 'secondary'}>
                  Fallback: {fallbackEnabled ? 'Enabled' : 'Disabled'}
                </Badge>
              </div>
            )}
          </div>
        }
      />

      {!selectedTenantId ? (
        <SectionCard title="Tool Routing" className="border-t-4 border-t-pink-500">
          <EmptyState
            icon={Route}
            title="Select a tenant"
            description="Choose a tenant to manage tool routing."
          />
        </SectionCard>
      ) : (
        <div className="space-y-6">
          {!tenantConfigQuery.isLoading && (
            <div
              className={
                isToolRoutingEnabled
                  ? 'rounded-lg border p-4'
                  : 'rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-900 dark:bg-red-950/20'
              }
            >
              <p className={isToolRoutingEnabled ? 'text-sm font-medium' : 'text-sm font-medium text-red-700 dark:text-red-400'}>
                Tool Routing is{' '}
                <span className={isToolRoutingEnabled ? 'text-green-600 dark:text-green-400' : 'text-red-700 dark:text-red-400'}>
                  {isToolRoutingEnabled ? 'enabled' : 'disabled'}
                </span>{' '}
                for tenant <span className="font-semibold">{selectedTenantId}</span>
              </p>
              {!isToolRoutingEnabled && (
                <p className="mt-1 text-xs text-red-600/80 dark:text-red-500/80">
                  None of the configuration on this page will be active until Tool Routing is enabled for this tenant. Enable it from the tenant Features section.
                </p>
              )}
            </div>
          )}

          <SectionCard title="What Tool Routing Does" className="border-t-4 border-t-cyan-400">
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>Tool Routing lets the gateway return a deterministic tool/action decision before LLM inference.</p>
              <p>When a semantic route matches above its threshold, the request is short-circuited and the gateway returns a structured tool decision.</p>
              <p>The gateway does not execute tools directly. Tool execution happens in the calling application or agent layer.</p>
            </div>
          </SectionCard>

          <SectionCard
            title="Tool Routes"
            description="Semantic routes that return tool decisions for this tenant."
            className="border-t-4 border-t-purple-500"
            action={
              <Button onClick={() => setCreateDialogOpen(true)} disabled={!selectedTenantId}>
                Create route
              </Button>
            }
          >
            {isLoadingRoutes ? (
              <Skeleton className="h-40" />
            ) : routesError ? (
              <div className="text-sm text-destructive">
                {routesError instanceof Error ? routesError.message : 'Failed to load routes'}
              </div>
            ) : toolRoutes.length === 0 ? (
              <EmptyState
                icon={Route}
                title="No tool routes configured for this tenant yet."
                description="Create a route to enable deterministic tool decisions."
                action={<Button onClick={() => setCreateDialogOpen(true)}>Create route</Button>}
              />
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Name</TableHead>
                      <TableHead>Description</TableHead>
                      <TableHead>Action</TableHead>
                      <TableHead className="text-right">Threshold</TableHead>
                      <TableHead className="text-right">Utterances</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {toolRoutes.map((route) => (
                      <TableRow key={route.name}>
                        <TableCell className="font-medium">{route.name}</TableCell>
                        <TableCell>{route.description || '—'}</TableCell>
                        <TableCell>{route.action}</TableCell>
                        <TableCell className="text-right tabular-nums">
                          {route.threshold != null ? route.threshold.toFixed(2) : '—'}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {Array.isArray(route.utterances) ? route.utterances.length : 0}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-1">
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => openEditDialog(route)}
                            >
                              Edit
                            </Button>
                            <Button size="sm" variant="ghost" onClick={() => setRouteToDelete(route)}>
                              Delete
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </SectionCard>

          <SectionCard
            title="Test Tool Routing"
            description="Use a real prompt to see whether tool routing intercepts the request before LLM inference."
            className="border-t-4 border-t-blue-500"
          >
            <div className="space-y-4">
              <div className="grid gap-4 md:grid-cols-[1fr_240px]">
                <div className="space-y-2">
                  <Label>Prompt</Label>
                  <Textarea
                    rows={4}
                    value={testPrompt}
                    onChange={(event) => setTestPrompt(event.target.value)}
                    placeholder="is it raining today"
                  />
                </div>
                <div className="space-y-2">
                  <Label>Model</Label>
                  {allowedModels.length > 0 ? (
                    <Select value={testModel} onValueChange={setTestModel}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select model" />
                      </SelectTrigger>
                      <SelectContent>
                        {allowedModels.map((model) => (
                          <SelectItem key={model} value={model}>
                            {model}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <Input
                      value={testModel}
                      onChange={(event) => setTestModel(event.target.value)}
                      placeholder="gpt-4o-mini"
                    />
                  )}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button onClick={() => testMutation.mutate()} disabled={testDisabled || testMutation.isPending}>
                  {testMutation.isPending ? 'Testing…' : 'Test tool routing'}
                </Button>
                <span className="text-xs text-muted-foreground">Runs against selected tenant via the real runtime endpoint.</span>
              </div>
              {testResult && (
                <div className="rounded-md border p-4 space-y-2 text-sm">
                  {testResult.status === 'error' ? (
                    <div className="space-y-1">
                      <Badge variant="destructive">Error</Badge>
                      <p className="text-destructive">{testResult.errorMessage}</p>
                    </div>
                  ) : (
                    <>
                      <div className="flex flex-wrap gap-2">
                        <Badge variant={testResult.matched ? 'default' : 'secondary'}>
                          {testResult.matched ? 'Matched' : 'No match'}
                        </Badge>
                        <Badge variant="outline">Model skipped: {testResult.skippedModelRouting ? 'Yes' : 'No'}</Badge>
                      </div>
                      <div className="grid gap-2 md:grid-cols-3">
                        <div>
                          <p className="text-xs text-muted-foreground">Route</p>
                          <p className="font-medium">{testResult.route || '—'}</p>
                        </div>
                        <div>
                          <p className="text-xs text-muted-foreground">Action</p>
                          <p className="font-medium">{testResult.action || '—'}</p>
                        </div>
                        <div>
                          <p className="text-xs text-muted-foreground">Similarity</p>
                          <p className="font-medium">
                            {typeof testResult.similarity === 'number' ? testResult.similarity.toFixed(4) : '—'}
                          </p>
                        </div>
                      </div>
                      {testResult.rawContent && (
                        <div className="text-xs text-muted-foreground">Raw: {testResult.rawContent}</div>
                      )}
                    </>
                  )}
                </div>
              )}
            </div>
          </SectionCard>

          <SectionCard title="Runtime Notes / Integration" className="border-t-4 border-t-amber-400">
            <div className="space-y-2 text-sm text-muted-foreground">
              <p>Tool Routing runs before normal model routing.</p>
              <p>If a route matches above threshold, the gateway returns a tool decision and model routing is skipped.</p>
              <p>If no route matches, normal routing continues.</p>
              <p>If tool routing fails internally, the system fails open and normal routing continues.</p>
            </div>
          </SectionCard>

          <SectionCard title="Semantic Summary" className="border-t-4 border-t-emerald-500">
            <div className="flex flex-wrap items-center gap-4">
              <div>
                <p className="text-xs text-muted-foreground">Anchors</p>
                <p className="text-lg font-semibold">
                  {semanticAnchorsQuery.isLoading ? '…' : anchorCount}
                </p>
              </div>
              <Button asChild variant="outline">
                <Link href="/semantic">
                  <Sparkles className="mr-2 h-4 w-4" /> Open Semantic page
                </Link>
              </Button>
            </div>
          </SectionCard>
        </div>
      )}

      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create tool route</DialogTitle>
            <DialogDescription>
              Create a semantic action route that returns a tool decision before model inference
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <Label htmlFor="tool-route-name">Name</Label>
              <Input
                id="tool-route-name"
                value={createForm.name}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, name: event.target.value }))}
                placeholder="weather_tool"
              />
            </div>
            <div>
              <Label htmlFor="tool-route-action">Action / Tool name</Label>
              <Input
                id="tool-route-action"
                value={createForm.action}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, action: event.target.value }))}
                placeholder="get_weather"
              />
              <p className="text-xs text-muted-foreground mt-1">
                This is the tool/action identifier returned by the gateway when the route matches.
              </p>
            </div>
            <div>
              <Label htmlFor="tool-route-description">Description</Label>
              <Input
                id="tool-route-description"
                value={createForm.description}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, description: event.target.value }))}
                placeholder="Route weather questions to tool"
              />
            </div>
            <div>
              <Label htmlFor="tool-route-threshold">Threshold</Label>
              <Input
                id="tool-route-threshold"
                type="number"
                step="0.01"
                min="0"
                max="1"
                placeholder="0.80"
                value={createForm.threshold}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, threshold: event.target.value }))}
              />
              {thresholdInvalid && createForm.threshold.trim() !== '' && (
                <p className="text-xs text-destructive mt-1">Threshold must be between 0 and 1</p>
              )}
              <p className="text-xs text-muted-foreground mt-1">
                Minimum semantic similarity required for this route to match.
              </p>
            </div>
            <div>
              <Label htmlFor="tool-route-utterances">Utterances (one per line)</Label>
              <Textarea
                id="tool-route-utterances"
                rows={4}
                value={createForm.utterances}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, utterances: event.target.value }))}
                placeholder="what is the weather\nis it raining today\ntemperature today"
              />
              <p className="text-xs text-muted-foreground mt-1">
                Provide several example prompts that represent this tool-use case.
              </p>
            </div>
            <div className="text-xs text-muted-foreground">
              Tenant: {selectedTenantId ?? '—'}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setCreateDialogOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => createRouteMutation.mutate()} disabled={createDisabled || createRouteMutation.isPending}>
              {createRouteMutation.isPending ? 'Creating…' : 'Create route'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={routeToEdit !== null} onOpenChange={(open) => !open && setRouteToEdit(null)}>
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>Edit tool route</DialogTitle>
            <DialogDescription>
              Update the route definition. Utterances will be re-embedded on save.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <Label htmlFor="edit-route-name">Name</Label>
              <Input
                id="edit-route-name"
                value={editForm.name}
                onChange={(event) => setEditForm((prev) => ({ ...prev, name: event.target.value }))}
                placeholder="weather_tool"
              />
            </div>
            <div>
              <Label htmlFor="edit-route-description">Description</Label>
              <Input
                id="edit-route-description"
                value={editForm.description}
                onChange={(event) => setEditForm((prev) => ({ ...prev, description: event.target.value }))}
                placeholder="Route weather questions to tool"
              />
            </div>
            <div>
              <Label htmlFor="edit-route-action">Action / Tool name</Label>
              <Input
                id="edit-route-action"
                value={editForm.action}
                onChange={(event) => setEditForm((prev) => ({ ...prev, action: event.target.value }))}
                placeholder="get_weather"
              />
              <p className="text-xs text-muted-foreground mt-1">
                This is the tool/action identifier returned by the gateway when the route matches.
              </p>
            </div>
            <div>
              <Label htmlFor="edit-route-threshold">Threshold</Label>
              <Input
                id="edit-route-threshold"
                type="number"
                step="0.01"
                min="0"
                max="1"
                placeholder="0.80"
                value={editForm.threshold}
                onChange={(event) => setEditForm((prev) => ({ ...prev, threshold: event.target.value }))}
              />
              {editThresholdInvalid && editForm.threshold.trim() !== '' && (
                <p className="text-xs text-destructive mt-1">Threshold must be between 0 and 1</p>
              )}
              <p className="text-xs text-muted-foreground mt-1">
                Minimum semantic similarity required for this route to match.
              </p>
            </div>
            <div>
              <Label htmlFor="edit-route-utterances">
                Utterances (one per line)
                {editUtterancesLoading && (
                  <span className="ml-2 text-xs text-muted-foreground">Loading…</span>
                )}
              </Label>
              <Textarea
                id="edit-route-utterances"
                rows={8}
                value={editForm.utterances}
                onChange={(event) => setEditForm((prev) => ({ ...prev, utterances: event.target.value }))}
                placeholder={'what is the weather\nis it raining today\ntemperature today'}
                disabled={editUtterancesLoading}
              />
              <p className="text-xs text-muted-foreground mt-1">
                Provide several example prompts that represent this tool-use case.
              </p>
            </div>
            <div className="text-xs text-muted-foreground">
              Tenant: {selectedTenantId ?? '—'}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setRouteToEdit(null)}>
              Cancel
            </Button>
            <Button onClick={() => editRouteMutation.mutate()} disabled={editDisabled || editRouteMutation.isPending}>
              {editRouteMutation.isPending ? 'Saving…' : 'Save changes'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={routeToDelete !== null} onOpenChange={(open) => !open && setRouteToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete tool route?</AlertDialogTitle>
            <AlertDialogDescription>
              Delete tool route "{routeToDelete?.name}" from tenant "{selectedTenantId}"?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => deleteRouteMutation.mutate()}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

export default function ToolsPage() {
  return (
    <RequireAdminRole>
      <ToolsContent />
    </RequireAdminRole>
  )
}
