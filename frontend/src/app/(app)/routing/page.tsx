'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils/cn'
import { listAdminModels, listSemanticAnchors } from '@/features/semantic/api/client'
import { useTenants, useTenantConfig, useUpdateTenantConfig, type TenantConfig } from '@/features/tenants/api/use-tenants'
import { ChevronDown, ChevronUp, Layers, Pencil, Route, Sparkles, Trash2 } from 'lucide-react'

type RouteGroupDialogState = {
  open: boolean
  mode: 'create' | 'edit'
  originalName?: string
  name: string
  models: string[]
}

type WeightState = {
  cost: number
  latency: number
  errors: number
}

type FallbackState = {
  enabled: boolean
  timeout_ms: number
  max_attempts: number
}

type SelectionState = {
  allow_model_override: boolean
  allow_route_group_override: boolean
  precedence_model: string
  conflict_policy: string
}

type SmartRuleWhen = {
  max_prompt_tokens?: number
  contains?: string[]
  prompt_length?: { gt?: number; gte?: number; lt?: number; lte?: number }
  semantic_similarity?: { threshold: number }
}

type SmartRuleAction = {
  block?: boolean
  reason?: string
  prefer_models?: string[]
  ban_models?: string[]
  set_constraints?: { max_cost_per_1m?: number; max_latency_ms?: number }
  use_anchor?: boolean
}

type SmartStageRule = {
  when: SmartRuleWhen
  action: SmartRuleAction
}

type SmartStage = {
  name: string
  rules: SmartStageRule[]
}

type RuleDialogState = {
  open: boolean
  stageIndex: number
  ruleIndex: number | null
  when: SmartRuleWhen
  action: SmartRuleAction
}

const emptyWhen: SmartRuleWhen = {}
const emptyAction: SmartRuleAction = {}

const STRATEGY_OPTIONS = [
  { value: 'smart',       label: 'Smart',        detail: 'Stages + pesos coste / latencia / errores' },
  { value: 'round_robin', label: 'Round Robin',  detail: 'Reparto round-robin por tenant' },
  { value: 'latency',     label: 'Latency',      detail: 'Orden por EWMA de latencia' },
  { value: 'cost',        label: 'Cost',         detail: 'Orden por coste estimado' },
  { value: 'header',      label: 'Header',       detail: 'Sin lógica, extrae por orden de candidatos' },
]

const STRATEGY_BY_VALUE = Object.fromEntries(STRATEGY_OPTIONS.map((o) => [o.value, o]))

const PRECEDENCE_OPTIONS = [
  { value: 'header', label: 'Header' },
  { value: 'route_group', label: 'Route group' },
  { value: 'default', label: 'Default' },
]

const CONFLICT_OPTIONS = [
  { value: 'error', label: 'Error' },
  { value: 'prefer_model', label: 'Prefer model' },
  { value: 'prefer_route_group', label: 'Prefer route group' },
]

const STAGE_META: Record<string, { title: string; description: string }> = {
  guardrails: {
    title: 'Guardrails',
    description: 'Blocks or modifies requests based on rules.',
  },
  semantic_intent: {
    title: 'Semantic Intent',
    description: 'Uses anchors and threshold to determine intent.',
  },
  structured_output: {
    title: 'Structured Output',
    description: 'Prefers specific models for structured tasks.',
  },
  route_group_resolution: {
    title: 'Route Group Resolution',
    description: 'Maps requests into configured route groups.',
  },
  model_selection: {
    title: 'Model Selection',
    description: 'Chooses the best model for the request.',
  },
  fallback: {
    title: 'Fallback',
    description: 'Handles routing failures and retries.',
  },
}

const ACTION_BADGE_ORDER = ['Block', 'Prefer models', 'Ban models', 'Use anchor', 'Constraints'] as const

function summarizeRuleWhen(when: unknown): string[] {
  if (!when || typeof when !== 'object') return []
  const w = when as Record<string, unknown>
  const labels: string[] = []
  if (w.max_prompt_tokens != null) {
    const v = w.max_prompt_tokens
    const n = typeof v === 'number' ? v : Number(v)
    labels.push(Number.isFinite(n) ? `tokens > ${n}` : 'tokens > N')
  }
  if (Array.isArray(w.contains) && w.contains.length > 0) {
    labels.push('contains keywords')
  }
  if (w.prompt_length != null && typeof w.prompt_length === 'object') {
    labels.push('length condition')
  }
  if (w.semantic_similarity != null && typeof w.semantic_similarity === 'object') {
    labels.push('semantic similarity')
  }
  return labels
}

function aggregateActionBadges(rules: unknown[]): string[] {
  const badges = new Set<string>()
  for (const rule of rules) {
    if (!rule || typeof rule !== 'object') continue
    const action = (rule as Record<string, unknown>).action
    if (!action || typeof action !== 'object') continue
    const a = action as Record<string, unknown>
    if (a.block === true) badges.add('Block')
    if (Array.isArray(a.prefer_models) && a.prefer_models.length > 0) badges.add('Prefer models')
    if (Array.isArray(a.ban_models) && a.ban_models.length > 0) badges.add('Ban models')
    if (a.use_anchor === true) badges.add('Use anchor')
    if (a.set_constraints != null && typeof a.set_constraints === 'object') badges.add('Constraints')
  }
  return ACTION_BADGE_ORDER.filter((b) => badges.has(b))
}

function collectWhenSummaries(rules: unknown[]): string[] {
  const seen = new Set<string>()
  const order: string[] = []
  for (const rule of rules) {
    if (!rule || typeof rule !== 'object') continue
    const when = (rule as Record<string, unknown>).when
    for (const label of summarizeRuleWhen(when)) {
      if (!seen.has(label)) {
        seen.add(label)
        order.push(label)
      }
    }
  }
  return order
}

function finalizeWhen(w: SmartRuleWhen): SmartRuleWhen {
  const out: SmartRuleWhen = {}
  if (w.max_prompt_tokens != null && Number.isFinite(w.max_prompt_tokens) && w.max_prompt_tokens >= 1) {
    out.max_prompt_tokens = Math.floor(w.max_prompt_tokens)
  }
  if (w.contains && w.contains.length > 0) {
    out.contains = [...w.contains]
  }
  const pl = w.prompt_length
  if (pl) {
    const cleaned: NonNullable<SmartRuleWhen['prompt_length']> = {}
    if (pl.gt != null && Number.isFinite(pl.gt)) cleaned.gt = pl.gt
    if (pl.gte != null && Number.isFinite(pl.gte)) cleaned.gte = pl.gte
    if (pl.lt != null && Number.isFinite(pl.lt)) cleaned.lt = pl.lt
    if (pl.lte != null && Number.isFinite(pl.lte)) cleaned.lte = pl.lte
    if (Object.keys(cleaned).length > 0) out.prompt_length = cleaned
  }
  if (
    w.semantic_similarity?.threshold != null &&
    Number.isFinite(w.semantic_similarity.threshold)
  ) {
    out.semantic_similarity = { threshold: w.semantic_similarity.threshold }
  }
  return out
}

function finalizeAction(a: SmartRuleAction): SmartRuleAction {
  const out: SmartRuleAction = {}
  if (a.block === true) {
    out.block = true
    if (a.reason != null && String(a.reason).trim() !== '') {
      out.reason = String(a.reason).trim()
    }
  }
  if (a.prefer_models && a.prefer_models.length > 0) {
    out.prefer_models = [...a.prefer_models]
  }
  if (a.ban_models && a.ban_models.length > 0) {
    out.ban_models = [...a.ban_models]
  }
  if (a.use_anchor === true) {
    out.use_anchor = true
  }
  const sc = a.set_constraints
  if (sc) {
    const c: NonNullable<SmartRuleAction['set_constraints']> = {}
    if (sc.max_cost_per_1m != null && Number.isFinite(sc.max_cost_per_1m)) {
      c.max_cost_per_1m = sc.max_cost_per_1m
    }
    if (sc.max_latency_ms != null && Number.isFinite(sc.max_latency_ms)) {
      c.max_latency_ms = sc.max_latency_ms
    }
    if (Object.keys(c).length > 0) {
      out.set_constraints = c
    }
  }
  return out
}

function patchPromptLength(
  when: SmartRuleWhen,
  field: 'gt' | 'gte' | 'lt' | 'lte',
  raw: string,
): SmartRuleWhen {
  const cur = { ...(when.prompt_length ?? {}) }
  if (raw.trim() === '') {
    delete cur[field]
  } else {
    const n = Number(raw)
    if (Number.isFinite(n)) cur[field] = n
  }
  const empty = cur.gt == null && cur.gte == null && cur.lt == null && cur.lte == null
  return {
    ...when,
    prompt_length: empty ? undefined : cur,
  }
}

function clampNumber(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function normalizeRouteGroups(value: unknown): Record<string, string[]> {
  if (!value || typeof value !== 'object') return {}
  const entries = Object.entries(value as Record<string, unknown>)
  const result: Record<string, string[]> = {}
  for (const [key, models] of entries) {
    if (!Array.isArray(models)) continue
    result[key] = models.filter((model) => typeof model === 'string') as string[]
  }
  return result
}

function RoutingContent() {
  const tenantsQuery = useTenants()
  const [selectedTenantId, setSelectedTenantId] = useState<string | null>(null)
  const tenantConfigQuery = useTenantConfig(selectedTenantId)
  const updateTenantConfig = useUpdateTenantConfig()

  const [strategy, setStrategy] = useState('smart')
  const [weights, setWeights] = useState<WeightState>({ cost: 0.4, latency: 0.3, errors: 0.3 })
  const [fallback, setFallback] = useState<FallbackState>({ enabled: false, timeout_ms: 20000, max_attempts: 1 })
  const [selection, setSelection] = useState<SelectionState>({
    allow_model_override: false,
    allow_route_group_override: false,
    precedence_model: 'header',
    conflict_policy: 'error',
  })
  const [routeGroups, setRouteGroups] = useState<Record<string, string[]>>({})
  const [routeGroupDialog, setRouteGroupDialog] = useState<RouteGroupDialogState>({
    open: false,
    mode: 'create',
    name: '',
    models: [],
  })
  const [editableStages, setEditableStages] = useState<SmartStage[]>([])
  const [ruleDialog, setRuleDialog] = useState<RuleDialogState>({
    open: false,
    stageIndex: 0,
    ruleIndex: null,
    when: emptyWhen,
    action: emptyAction,
  })

  useEffect(() => {
    if (!selectedTenantId && tenantsQuery.data?.length) {
      setSelectedTenantId(tenantsQuery.data[0].tenant_id)
    }
  }, [selectedTenantId, tenantsQuery.data])

  useEffect(() => {
    const tenantConfig = tenantConfigQuery.data
    if (!tenantConfig) return
    const config = tenantConfig.config as Record<string, unknown>
    const routing = (config?.routing as Record<string, unknown>) || {}
    const smart = (routing.smart as Record<string, unknown>) || {}
    const weightConfig = (smart.weights as Record<string, unknown>) || {}
    const fallbackConfig = (routing.fallback as Record<string, unknown>) || {}
    const selectionConfig = (config?.selection as Record<string, unknown>) || {}
    const precedenceConfig = (selectionConfig.precedence as Record<string, unknown>) || {}

    setStrategy(String(routing.strategy ?? config.routing_strategy ?? 'smart'))
    setWeights({
      cost: Number(weightConfig.cost ?? 0.4),
      latency: Number(weightConfig.latency ?? 0.3),
      errors: Number(weightConfig.errors ?? 0.3),
    })
    setFallback({
      enabled: Boolean(fallbackConfig.enabled ?? false),
      timeout_ms: Number(fallbackConfig.timeout_ms ?? 20000),
      max_attempts: Number(fallbackConfig.max_attempts ?? 1),
    })
    setSelection({
      allow_model_override: Boolean(selectionConfig.allow_model_override ?? false),
      allow_route_group_override: Boolean(selectionConfig.allow_route_group_override ?? false),
      precedence_model: String(precedenceConfig.model ?? 'header'),
      conflict_policy: String(precedenceConfig.conflict_policy ?? 'error'),
    })
    setRouteGroups(
      normalizeRouteGroups(selectionConfig.route_groups ?? config.route_groups ?? {})
    )

    const rawStages = smart.stages
    setEditableStages(
      Array.isArray(rawStages)
        ? (JSON.parse(JSON.stringify(rawStages)) as SmartStage[])
        : []
    )
  }, [tenantConfigQuery.data])

  const { data: adminModels = [], isLoading: isLoadingModels } = useQuery({
    queryKey: ['admin-models'],
    queryFn: listAdminModels,
  })

  const semanticAnchorsQuery = useQuery({
    queryKey: ['semantic-anchors-summary', selectedTenantId],
    queryFn: () => listSemanticAnchors({ limit: 50, includeAnchorText: false, tenantId: selectedTenantId }),
    enabled: Boolean(selectedTenantId),
  })

  const semanticRoutesQuery = useQuery({
    queryKey: ['semantic-routes-summary', selectedTenantId],
    queryFn: async () => {
      const res = await fetch(`/api/semantic/routes?tenant_id=${encodeURIComponent(selectedTenantId ?? '')}`, {
        cache: 'no-store',
      })
      if (!res.ok) {
        const payload = await res.json().catch(() => ({}))
        throw new Error(payload.error || 'Failed to fetch semantic routes')
      }
      return res.json() as Promise<{ routes?: unknown[] }>
    },
    enabled: Boolean(selectedTenantId),
  })

  const tenantConfig = tenantConfigQuery.data as TenantConfig | undefined
  const config = tenantConfig?.config as Record<string, unknown> | undefined
  const routing = (config?.routing as Record<string, unknown>) || {}
  const smart = (routing.smart as Record<string, unknown>) || {}
  const stages = Array.isArray(smart.stages) ? smart.stages : []
  const semantic = (routing.semantic as Record<string, unknown>) || {}
  const thresholdDefault = typeof semantic.threshold_default === 'number' ? semantic.threshold_default : null

  const allowedModels = useMemo(() => {
    const raw = config?.allowed_models
    return Array.isArray(raw) ? raw.filter((model) => typeof model === 'string') : []
  }, [config])

  const modelOptions = useMemo(() => adminModels.map((model) => model.id), [adminModels])
  const routeGroupModelOptions = useMemo(() => {
    if (allowedModels.length === 0) return modelOptions
    const allowedSet = new Set(allowedModels)
    return modelOptions.filter((model) => allowedSet.has(model))
  }, [allowedModels, modelOptions])

  const routeGroupEntries = useMemo(() => Object.entries(routeGroups), [routeGroups])
  const anchorCount = semanticAnchorsQuery.data?.data?.length ?? 0
  const semanticRoutesCount = semanticRoutesQuery.data?.routes?.length ?? 0

  const fallbackLabel = fallback.enabled ? 'Enabled' : 'Disabled'
  const weightsSum = weights.cost + weights.latency + weights.errors
  const weightsValid =
    [weights.cost, weights.latency, weights.errors].every((w) => w >= 0 && w <= 1) &&
    Math.abs(weightsSum - 1) < 0.001
  const fallbackValid =
    !fallback.enabled ||
    (Number.isFinite(fallback.timeout_ms) && fallback.timeout_ms > 0 && fallback.max_attempts >= 1)

  const strategyDirty = (routing.strategy ?? config?.routing_strategy ?? 'smart') !== strategy
  const weightsDirty =
    Number((smart.weights as Record<string, unknown>)?.cost ?? 0.4) !== weights.cost ||
    Number((smart.weights as Record<string, unknown>)?.latency ?? 0.3) !== weights.latency ||
    Number((smart.weights as Record<string, unknown>)?.errors ?? 0.3) !== weights.errors
  const fallbackDirty =
    Boolean((routing.fallback as Record<string, unknown>)?.enabled ?? false) !== fallback.enabled ||
    Number((routing.fallback as Record<string, unknown>)?.timeout_ms ?? 20000) !== fallback.timeout_ms ||
    Number((routing.fallback as Record<string, unknown>)?.max_attempts ?? 1) !== fallback.max_attempts
  const selectionDirty =
    Boolean((config?.selection as Record<string, unknown>)?.allow_model_override ?? false) !== selection.allow_model_override ||
    Boolean((config?.selection as Record<string, unknown>)?.allow_route_group_override ?? false) !== selection.allow_route_group_override ||
    String(((config?.selection as Record<string, unknown>)?.precedence as Record<string, unknown>)?.model ?? 'header') !== selection.precedence_model ||
    String(((config?.selection as Record<string, unknown>)?.precedence as Record<string, unknown>)?.conflict_policy ?? 'error') !== selection.conflict_policy

  const routeGroupsDirty = useMemo(() => {
    const current = normalizeRouteGroups((config?.selection as Record<string, unknown>)?.route_groups ?? config?.route_groups ?? {})
    const currentKeys = Object.keys(current).sort()
    const nextKeys = Object.keys(routeGroups).sort()
    if (currentKeys.length !== nextKeys.length) return true
    for (const key of currentKeys) {
      if (!routeGroups[key]) return true
      const a = [...current[key]].sort()
      const b = [...routeGroups[key]].sort()
      if (a.length !== b.length) return true
      if (a.some((value, idx) => value !== b[idx])) return true
    }
    return false
  }, [config, routeGroups])

  const pipelineStages = useMemo(() => {
    if (editableStages.length === 0) return []
    return editableStages.map((stage) => {
      const stageName = stage.name
      return {
        description: STAGE_META[stageName]?.description ?? 'Routing stage.',
      }
    })
  }, [editableStages])

  const stagesDirty = useMemo(() => {
    return JSON.stringify(stages) !== JSON.stringify(editableStages)
  }, [stages, editableStages])

  const openRuleDialog = (stageIndex: number, ruleIndex: number | null) => {
    if (ruleIndex === null) {
      setRuleDialog({
        open: true,
        stageIndex,
        ruleIndex: null,
        when: {},
        action: {},
      })
      return
    }
    const rule = editableStages[stageIndex]?.rules[ruleIndex]
    if (!rule) return
    setRuleDialog({
      open: true,
      stageIndex,
      ruleIndex,
      when: JSON.parse(JSON.stringify(rule.when ?? {})) as SmartRuleWhen,
      action: JSON.parse(JSON.stringify(rule.action ?? {})) as SmartRuleAction,
    })
  }

  const handleSaveRule = () => {
    const when = finalizeWhen(ruleDialog.when)
    const action = finalizeAction(ruleDialog.action)
    const newRule: SmartStageRule = { when, action }
    setEditableStages((prev) => {
      const next = JSON.parse(JSON.stringify(prev)) as SmartStage[]
      const stage = next[ruleDialog.stageIndex]
      if (!stage) return prev
      const rules = [...stage.rules]
      if (ruleDialog.ruleIndex === null) {
        rules.push(newRule)
      } else {
        rules[ruleDialog.ruleIndex] = newRule
      }
      next[ruleDialog.stageIndex] = { ...stage, rules }
      return next
    })
    setRuleDialog({
      open: false,
      stageIndex: 0,
      ruleIndex: null,
      when: emptyWhen,
      action: emptyAction,
    })
  }

  const toggleRuleModel = (field: 'prefer_models' | 'ban_models', modelId: string) => {
    setRuleDialog((prev) => {
      const cur = prev.action[field] ?? []
      const nextList = cur.includes(modelId) ? cur.filter((id) => id !== modelId) : [...cur, modelId]
      return {
        ...prev,
        action: {
          ...prev.action,
          [field]: nextList.length > 0 ? nextList : undefined,
        },
      }
    })
  }

  const patchRuleConstraint = (key: 'max_cost_per_1m' | 'max_latency_ms', raw: string) => {
    setRuleDialog((prev) => {
      const sc: NonNullable<SmartRuleAction['set_constraints']> = {
        ...(prev.action.set_constraints ?? {}),
      }
      if (raw.trim() === '') {
        delete sc[key]
      } else {
        const n = parseFloat(raw)
        if (Number.isFinite(n)) sc[key] = n
      }
      const empty = sc.max_cost_per_1m == null && sc.max_latency_ms == null
      return {
        ...prev,
        action: {
          ...prev.action,
          set_constraints: empty ? undefined : sc,
        },
      }
    })
  }

  const handleSave = async (patch: Record<string, unknown>) => {
    if (!tenantConfig) return
    await updateTenantConfig.mutateAsync({
      tenantId: tenantConfig.tenant_id,
      version: tenantConfig.version,
      patch,
    })
  }

  const handleRouteGroupSave = async () => {
    if (!routeGroupDialog.name.trim()) return
    const name = routeGroupDialog.name.trim()
    const next = { ...routeGroups }
    if (routeGroupDialog.mode === 'edit' && routeGroupDialog.originalName && routeGroupDialog.originalName !== name) {
      delete next[routeGroupDialog.originalName]
    }
    next[name] = routeGroupDialog.models
    setRouteGroups(next)
    await handleSave({ selection: { route_groups: next } })
    setRouteGroupDialog({ open: false, mode: 'create', name: '', models: [] })
  }

  const isLoading = tenantsQuery.isLoading || tenantConfigQuery.isLoading

  return (
    <div>
      <PageHeader
        title="Routing"
        description="Configure routing rules and strategies. This configuration is tenant-specific."
        action={
          <div className="flex w-full max-w-full flex-col gap-3 sm:w-auto sm:max-w-none sm:flex-row sm:items-end sm:justify-end sm:gap-3">
            <div className="w-full min-w-0 sm:w-60">
              <Label htmlFor="routing-tenant" className="text-xs text-muted-foreground">
                Tenant
              </Label>
              <Select
                value={selectedTenantId ?? ''}
                onValueChange={(value) => setSelectedTenantId(value)}
              >
                <SelectTrigger id="routing-tenant" className="mt-1">
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
                <Badge variant="secondary">Strategy: {STRATEGY_BY_VALUE[strategy]?.label ?? strategy}</Badge>
                <Badge variant="secondary">
                  Threshold: {thresholdDefault != null ? thresholdDefault.toFixed(2) : '—'}
                </Badge>
                <Badge variant={fallback.enabled ? 'default' : 'secondary'}>
                  Fallback: {fallbackLabel}
                </Badge>
              </div>
            )}
          </div>
        }
      />

      {!selectedTenantId ? (
        <SectionCard title="Routing" className="border-t-4 border-t-pink-500">
          <EmptyState
            icon={Route}
            title="Select a tenant"
            description="Choose a tenant to view routing configuration."
          />
        </SectionCard>
      ) : isLoading ? (
        <div className="space-y-6">
          <SectionCard title="Routing Strategy" className="border-t-4 border-t-pink-500">
            <Skeleton className="h-20" />
          </SectionCard>
          <SectionCard title="Pipeline / Stages" className="border-t-4 border-t-cyan-400">
            <Skeleton className="h-36" />
          </SectionCard>
        </div>
      ) : (
        <div className="space-y-6">
          <SectionCard
            title="Routing Strategy"
            description="Choose how requests are evaluated and how models are selected."
            className="border-t-4 border-t-pink-500"
            action={
              <Button
                size="sm"
                onClick={() => handleSave({ routing: { strategy } })}
                disabled={!strategyDirty || updateTenantConfig.isPending}
              >
                Save strategy
              </Button>
            }
          >
            <div className="max-w-xs space-y-2">
              <Select value={strategy} onValueChange={setStrategy}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {STRATEGY_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {STRATEGY_BY_VALUE[strategy]?.detail && (
                <p className="text-xs text-muted-foreground">
                  <span className="font-medium text-foreground">Details:</span>{' '}
                  {STRATEGY_BY_VALUE[strategy].detail}
                </p>
              )}
            </div>
          </SectionCard>

          <SectionCard
            title="Pipeline / Stages"
            description={'See how requests move through the routing pipeline. The tenant must have the routing strategy set to \u201cSmart\u201d to use stages. Any model referenced in a Stage (e.g., via prefer_models) must be included in the candidate pool (i.e., within the route group or allowed models used by the Smart strategy).'}
            className={cn(
              'border-t-4 border-t-cyan-400',
              stagesDirty ? 'bg-red-50 border-red-200 dark:bg-red-950/20 dark:border-red-900' : ''
            )}
            action={
              <div className="flex flex-wrap items-center justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    setEditableStages((prev) => [...prev, { name: 'new_stage', rules: [] }])
                  }
                >
                  + Add stage
                </Button>
                <Button
                  type="button"
                  size="sm"
                  disabled={!stagesDirty || updateTenantConfig.isPending}
                  onClick={() =>
                    handleSave({ routing: { smart: { stages: editableStages } } })
                  }
                >
                  {updateTenantConfig.isPending ? 'Saving...' : 'Save stages'}
                </Button>
              </div>
            }
          >
            {pipelineStages.length === 0 ? (
              <EmptyState
                icon={Route}
                title="No stages configured"
                description="Add routing stages to control how requests are evaluated before model selection."
              />
            ) : (
              <div className="space-y-4">
                {pipelineStages.map((stage, index) => (
                  <div key={index} className="rounded-md border p-4">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0 flex-1 space-y-2">
                        <Input
                          value={editableStages[index]?.name ?? ''}
                          onChange={(event) => {
                            const name = event.target.value
                            setEditableStages((prev) => {
                              const next = [...prev]
                              const cur = next[index]
                              if (!cur) return prev
                              next[index] = { ...cur, name }
                              return next
                            })
                          }}
                          className={cn(
                            'h-8 max-w-md border-transparent bg-transparent px-0 text-base font-medium shadow-none',
                            'transition-colors hover:bg-muted/30 focus-visible:border-input focus-visible:ring-1 focus-visible:ring-ring'
                          )}
                          aria-label="Stage name"
                        />
                        <p className="text-sm text-muted-foreground">{stage.description}</p>
                        <div className="space-y-3 border-t pt-3">
                          {(editableStages[index]?.rules ?? []).length === 0 ? (
                            <p className="text-sm text-muted-foreground">No rules yet</p>
                          ) : null}
                          {(editableStages[index]?.rules ?? []).map((rule, ruleIndex) => {
                            const ruleBadges = aggregateActionBadges([rule as unknown])
                            const ruleWhens = collectWhenSummaries([rule as unknown])
                            return (
                            <div
                              key={ruleIndex}
                              className="flex flex-wrap items-start justify-between gap-2 rounded-md border p-2"
                            >
                              <div className="min-w-0 flex-1 space-y-1">
                                {ruleBadges.length > 0 && (
                                  <div className="flex flex-wrap gap-1.5">
                                    {ruleBadges.map((label) => (
                                      <Badge key={label} variant="secondary">
                                        {label}
                                      </Badge>
                                    ))}
                                  </div>
                                )}
                                {ruleWhens.length > 0 && (
                                  <p className="text-xs text-muted-foreground">
                                    {ruleWhens.join(' · ')}
                                  </p>
                                )}
                                {ruleBadges.length === 0 && ruleWhens.length === 0 && (
                                    <p className="text-xs text-muted-foreground">Empty rule</p>
                                  )}
                              </div>
                              <div className="flex shrink-0 gap-0.5">
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  className="h-8 w-8 p-0"
                                  onClick={() => openRuleDialog(index, ruleIndex)}
                                  aria-label="Edit rule"
                                >
                                  <Pencil className="h-4 w-4" />
                                </Button>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  className="h-8 w-8 p-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
                                  onClick={() =>
                                    setEditableStages((prev) => {
                                      const next = JSON.parse(JSON.stringify(prev)) as SmartStage[]
                                      const st = next[index]
                                      if (!st) return prev
                                      next[index] = {
                                        ...st,
                                        rules: st.rules.filter((_, i) => i !== ruleIndex),
                                      }
                                      return next
                                    })
                                  }
                                  aria-label="Delete rule"
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </div>
                            </div>
                            )
                          })}
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="h-8 px-2"
                            onClick={() => openRuleDialog(index, null)}
                          >
                            + Add rule
                          </Button>
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center gap-0.5">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-8 w-8 p-0"
                          disabled={index === 0}
                          onClick={() =>
                            setEditableStages((prev) => {
                              if (index <= 0) return prev
                              const next = [...prev]
                              ;[next[index - 1], next[index]] = [next[index], next[index - 1]]
                              return next
                            })
                          }
                          aria-label="Move stage up"
                        >
                          <ChevronUp className="h-4 w-4" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-8 w-8 p-0"
                          disabled={index >= pipelineStages.length - 1}
                          onClick={() =>
                            setEditableStages((prev) => {
                              if (index >= prev.length - 1) return prev
                              const next = [...prev]
                              ;[next[index], next[index + 1]] = [next[index + 1], next[index]]
                              return next
                            })
                          }
                          aria-label="Move stage down"
                        >
                          <ChevronDown className="h-4 w-4" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-8 w-8 p-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
                          onClick={() =>
                            setEditableStages((prev) => prev.filter((_, i) => i !== index))
                          }
                          aria-label="Remove stage"
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </SectionCard>

          <Dialog
            open={ruleDialog.open}
            onOpenChange={(open) =>
              !open && setRuleDialog((prev) => ({ ...prev, open: false }))
            }
          >
            <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
              <DialogHeader>
                <DialogTitle>
                  {ruleDialog.ruleIndex === null ? 'Add rule' : 'Edit rule'}
                </DialogTitle>
                <DialogDescription>
                  Stage: {editableStages[ruleDialog.stageIndex]?.name ?? ''}
                </DialogDescription>
              </DialogHeader>

              <div className="space-y-4">
                <div>
                  <p className="mb-3 text-sm font-semibold">When</p>
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="rule-max-prompt-tokens">Max prompt tokens. (The rule applies if the input prompt lenght is below the value of this field). Leave empty to disable this limit.</Label>
                      <Input
                        id="rule-max-prompt-tokens"
                        type="number"
                        min={1}
                        value={ruleDialog.when.max_prompt_tokens ?? ''}
                        onChange={(e) => {
                          const v = e.target.value
                          setRuleDialog((prev) => ({
                            ...prev,
                            when: {
                              ...prev.when,
                              max_prompt_tokens:
                                v === ''
                                  ? undefined
                                  : Number.isFinite(Number(v))
                                    ? Number(v)
                                    : undefined,
                            },
                          }))
                        }}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="rule-contains">Contains keywords</Label>
                      <Textarea
                        id="rule-contains"
                        rows={3}
                        placeholder="One keyword per line"
                        value={(ruleDialog.when.contains ?? []).join('\n')}
                        onChange={(e) => {
                          const lines = e.target.value
                            .split('\n')
                            .map((s) => s.trim())
                            .filter(Boolean)
                          setRuleDialog((prev) => ({
                            ...prev,
                            when: {
                              ...prev.when,
                              contains: lines.length === 0 ? undefined : lines,
                            },
                          }))
                        }}
                      />
                    </div>
                    <div className="space-y-2">
                      <p className="text-sm font-medium">Prompt length (characters)</p>
                      <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                          <Label htmlFor="rule-pl-gt">&gt; (gt)</Label>
                          <Input
                            id="rule-pl-gt"
                            type="number"
                            value={ruleDialog.when.prompt_length?.gt ?? ''}
                            onChange={(e) =>
                              setRuleDialog((prev) => ({
                                ...prev,
                                when: patchPromptLength(prev.when, 'gt', e.target.value),
                              }))
                            }
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label htmlFor="rule-pl-gte">≥ (gte)</Label>
                          <Input
                            id="rule-pl-gte"
                            type="number"
                            value={ruleDialog.when.prompt_length?.gte ?? ''}
                            onChange={(e) =>
                              setRuleDialog((prev) => ({
                                ...prev,
                                when: patchPromptLength(prev.when, 'gte', e.target.value),
                              }))
                            }
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label htmlFor="rule-pl-lt">&lt; (lt)</Label>
                          <Input
                            id="rule-pl-lt"
                            type="number"
                            value={ruleDialog.when.prompt_length?.lt ?? ''}
                            onChange={(e) =>
                              setRuleDialog((prev) => ({
                                ...prev,
                                when: patchPromptLength(prev.when, 'lt', e.target.value),
                              }))
                            }
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label htmlFor="rule-pl-lte">≤ (lte)</Label>
                          <Input
                            id="rule-pl-lte"
                            type="number"
                            value={ruleDialog.when.prompt_length?.lte ?? ''}
                            onChange={(e) =>
                              setRuleDialog((prev) => ({
                                ...prev,
                                when: patchPromptLength(prev.when, 'lte', e.target.value),
                              }))
                            }
                          />
                        </div>
                      </div>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="rule-semantic-threshold">
                        Semantic similarity threshold (0–1)
                      </Label>
                      <Input
                        id="rule-semantic-threshold"
                        type="number"
                        min={0}
                        max={1}
                        step={0.01}
                        value={ruleDialog.when.semantic_similarity?.threshold ?? ''}
                        onChange={(e) => {
                          const v = e.target.value
                          const t = parseFloat(v)
                          setRuleDialog((prev) => ({
                            ...prev,
                            when: {
                              ...prev.when,
                              semantic_similarity:
                                v === '' || !Number.isFinite(t) ? undefined : { threshold: t },
                            },
                          }))
                        }}
                      />
                    </div>
                  </div>
                </div>

                <div className="border-t" />

                <div>
                  <p className="mb-3 text-sm font-semibold">Action</p>
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <Switch
                          id="rule-block"
                          checked={ruleDialog.action.block ?? false}
                          onCheckedChange={(checked) =>
                            setRuleDialog((prev) => ({
                              ...prev,
                              action: {
                                ...prev.action,
                                block: checked ? true : undefined,
                                reason: checked ? prev.action.reason : undefined,
                              },
                            }))
                          }
                        />
                        <Label htmlFor="rule-block">Block request</Label>
                      </div>
                      {ruleDialog.action.block ? (
                        <div className="space-y-2 pl-1">
                          <Label htmlFor="rule-block-reason">Reason</Label>
                          <Input
                            id="rule-block-reason"
                            value={ruleDialog.action.reason ?? ''}
                            onChange={(e) =>
                              setRuleDialog((prev) => ({
                                ...prev,
                                action: {
                                  ...prev.action,
                                  reason: e.target.value || undefined,
                                },
                              }))
                            }
                            placeholder="Optional"
                          />
                        </div>
                      ) : null}
                    </div>
                    <div className="space-y-2">
                      <Label>Prefer models</Label>
                      {isLoadingModels ? (
                        <Skeleton className="h-32" />
                      ) : routeGroupModelOptions.length === 0 ? (
                        <p className="text-sm text-muted-foreground">No models available.</p>
                      ) : (
                        <div className="max-h-40 space-y-2 overflow-y-auto rounded-md border p-3">
                          {routeGroupModelOptions.map((modelId) => (
                            <label
                              key={`prefer-${modelId}`}
                              className="flex items-center gap-2 text-sm"
                            >
                              <Checkbox
                                checked={
                                  ruleDialog.action.prefer_models?.includes(modelId) ?? false
                                }
                                onCheckedChange={() =>
                                  toggleRuleModel('prefer_models', modelId)
                                }
                              />
                              <span>{modelId}</span>
                            </label>
                          ))}
                        </div>
                      )}
                    </div>
                    <div className="space-y-2">
                      <Label>Ban models</Label>
                      {isLoadingModels ? (
                        <Skeleton className="h-32" />
                      ) : routeGroupModelOptions.length === 0 ? (
                        <p className="text-sm text-muted-foreground">No models available.</p>
                      ) : (
                        <div className="max-h-40 space-y-2 overflow-y-auto rounded-md border p-3">
                          {routeGroupModelOptions.map((modelId) => (
                            <label
                              key={`ban-${modelId}`}
                              className="flex items-center gap-2 text-sm"
                            >
                              <Checkbox
                                checked={ruleDialog.action.ban_models?.includes(modelId) ?? false}
                                onCheckedChange={() => toggleRuleModel('ban_models', modelId)}
                              />
                              <span>{modelId}</span>
                            </label>
                          ))}
                        </div>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      <Switch
                        id="rule-use-anchor"
                        checked={ruleDialog.action.use_anchor ?? false}
                        onCheckedChange={(checked) =>
                          setRuleDialog((prev) => ({
                            ...prev,
                            action: {
                              ...prev.action,
                              use_anchor: checked ? true : undefined,
                            },
                          }))
                        }
                      />
                      <Label htmlFor="rule-use-anchor">Use anchor</Label>
                    </div>
                    <div className="space-y-2">
                      <Label>Constraints</Label>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <div className="space-y-1.5">
                          <Label htmlFor="rule-max-cost">Max cost per 1M (USD)</Label>
                          <Input
                            id="rule-max-cost"
                            type="number"
                            value={ruleDialog.action.set_constraints?.max_cost_per_1m ?? ''}
                            onChange={(e) =>
                              patchRuleConstraint('max_cost_per_1m', e.target.value)
                            }
                          />
                        </div>
                        <div className="space-y-1.5">
                          <Label htmlFor="rule-max-latency">Max latency (ms)</Label>
                          <Input
                            id="rule-max-latency"
                            type="number"
                            value={ruleDialog.action.set_constraints?.max_latency_ms ?? ''}
                            onChange={(e) =>
                              patchRuleConstraint('max_latency_ms', e.target.value)
                            }
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setRuleDialog((prev) => ({ ...prev, open: false }))}
                >
                  Cancel
                </Button>
                <Button type="button" onClick={handleSaveRule}>
                  Save rule
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>

          {strategy === 'smart' && (
            <SectionCard
              title="Smart Routing Weights"
              description="Control how strongly cost, latency, and errors influence model selection."
              className="border-t-4 border-t-purple-500"
              action={
                <Button
                  size="sm"
                  onClick={() =>
                    handleSave({
                      routing: {
                        smart: {
                          weights: {
                            cost: weights.cost,
                            latency: weights.latency,
                            errors: weights.errors,
                          },
                        },
                      },
                    })
                  }
                  disabled={!weightsDirty || !weightsValid || updateTenantConfig.isPending}
                >
                  Save weights
                </Button>
              }
            >
              <div className="grid gap-4 md:grid-cols-3">
                <div className="space-y-2">
                  <Label>Cost</Label>
                  <Input
                    type="number"
                    step="0.01"
                    min="0"
                    max="1"
                    value={weights.cost}
                    onChange={(event) =>
                      setWeights((prev) => ({
                        ...prev,
                        cost: clampNumber(Number(event.target.value), 0, 1),
                      }))
                    }
                  />
                  <p className="text-xs text-muted-foreground">Higher favors cheaper models.</p>
                </div>
                <div className="space-y-2">
                  <Label>Latency</Label>
                  <Input
                    type="number"
                    step="0.01"
                    min="0"
                    max="1"
                    value={weights.latency}
                    onChange={(event) =>
                      setWeights((prev) => ({
                        ...prev,
                        latency: clampNumber(Number(event.target.value), 0, 1),
                      }))
                    }
                  />
                  <p className="text-xs text-muted-foreground">Higher favors faster models.</p>
                </div>
                <div className="space-y-2">
                  <Label>Errors</Label>
                  <Input
                    type="number"
                    step="0.01"
                    min="0"
                    max="1"
                    value={weights.errors}
                    onChange={(event) =>
                      setWeights((prev) => ({
                        ...prev,
                        errors: clampNumber(Number(event.target.value), 0, 1),
                      }))
                    }
                  />
                  <p className="text-xs text-muted-foreground">Higher avoids unstable models.</p>
                </div>
              </div>
              <div className="mt-4 text-sm text-muted-foreground">
                Total: {weightsSum.toFixed(2)} {weightsValid ? '' : '(must equal 1.00)'}
              </div>
            </SectionCard>
          )}

          <SectionCard
            title="Fallback"
            description="Define what happens when primary routing cannot complete."
            className="border-t-4 border-t-amber-400"
            action={
              <Button
                size="sm"
                onClick={() =>
                  handleSave({
                    routing: {
                      fallback: {
                        enabled: fallback.enabled,
                        timeout_ms: fallback.timeout_ms,
                        max_attempts: fallback.max_attempts,
                      },
                    },
                  })
                }
                disabled={!fallbackDirty || !fallbackValid || updateTenantConfig.isPending}
              >
                Save fallback
              </Button>
            }
          >
            <div className="grid gap-4 md:grid-cols-3">
              <div className="flex items-center gap-2">
                <Switch
                  checked={fallback.enabled}
                  onCheckedChange={(value) => setFallback((prev) => ({ ...prev, enabled: Boolean(value) }))}
                />
                <span className="text-sm">Enabled</span>
              </div>
              <div className="space-y-2">
                <Label>Timeout (ms)</Label>
                <Input
                  type="number"
                  min="1"
                  value={fallback.timeout_ms}
                  onChange={(event) =>
                    setFallback((prev) => ({ ...prev, timeout_ms: Number(event.target.value) }))
                  }
                />
              </div>
              <div className="space-y-2">
                <Label>Max attempts</Label>
                <Input
                  type="number"
                  min="1"
                  value={fallback.max_attempts}
                  onChange={(event) =>
                    setFallback((prev) => ({ ...prev, max_attempts: Number(event.target.value) }))
                  }
                />
              </div>
            </div>
            {!fallbackValid && (
              <p className="mt-3 text-sm text-destructive">
                Timeout must be greater than 0 and max attempts must be at least 1.
              </p>
            )}
          </SectionCard>

          <SectionCard
            title="Selection / Precedence"
            description="Control header overrides and precedence rules."
            className="border-t-4 border-t-emerald-500"
            action={
              <Button
                size="sm"
                onClick={() =>
                  handleSave({
                    selection: {
                      allow_model_override: selection.allow_model_override,
                      allow_route_group_override: selection.allow_route_group_override,
                      precedence: {
                        model: selection.precedence_model,
                        conflict_policy: selection.conflict_policy,
                      },
                    },
                  })
                }
                disabled={!selectionDirty || updateTenantConfig.isPending}
              >
                Save selection
              </Button>
            }
          >
            <div className="space-y-4">
              <div className="grid gap-4 md:grid-cols-2">
                <div className="flex items-center justify-between rounded-md border p-3">
                  <div>
                    <p className="text-sm font-medium">Model override (header)</p>
                    <p className="text-xs text-muted-foreground">Allow clients to force a model via request headers.</p>
                  </div>
                  <Switch
                    checked={selection.allow_model_override}
                    onCheckedChange={(checked) => setSelection((prev) => ({ ...prev, allow_model_override: checked }))}
                  />
                </div>
                <div className="flex items-center justify-between rounded-md border p-3">
                  <div>
                    <p className="text-sm font-medium">Route group override (header)</p>
                    <p className="text-xs text-muted-foreground">Allow clients to force a route group via request headers.</p>
                  </div>
                  <Switch
                    checked={selection.allow_route_group_override}
                    onCheckedChange={(checked) => setSelection((prev) => ({ ...prev, allow_route_group_override: checked }))}
                  />
                </div>
              </div>

              {!selection.allow_model_override && !selection.allow_route_group_override ? (
                <p className="text-xs text-muted-foreground">Header overrides are disabled for this tenant.</p>
              ) : (!selection.allow_model_override || !selection.allow_route_group_override) ? (
                <p className="text-xs text-muted-foreground">Conflict policy only applies when both model and route group overrides are enabled.</p>
              ) : null}

              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label>Precedence source</Label>
                  <p className="text-xs text-muted-foreground">Precedence applies only to enabled overrides.</p>
                  <Select
                    value={selection.precedence_model}
                    onValueChange={(value) => setSelection((prev) => ({ ...prev, precedence_model: value }))}
                    disabled={!selection.allow_model_override && !selection.allow_route_group_override}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {PRECEDENCE_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>Conflict policy</Label>
                  <p className="text-xs text-muted-foreground">Behavior when the requested model is not part of the assigned route group.</p>
                  <Select
                    value={selection.conflict_policy}
                    onValueChange={(value) => setSelection((prev) => ({ ...prev, conflict_policy: value }))}
                    disabled={!selection.allow_model_override || !selection.allow_route_group_override}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {CONFLICT_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </div>
          </SectionCard>

          <SectionCard
            title="Semantic Integration"
            description="View how semantic matching influences routing outcomes."
            className="border-t-4 border-t-blue-500"
          >
            <div className="grid gap-4 md:grid-cols-3">
              <div className="rounded-md border p-4">
                <p className="text-xs text-muted-foreground">Threshold</p>
                <p className="text-lg font-semibold">
                  {thresholdDefault != null ? thresholdDefault.toFixed(2) : '—'}
                </p>
              </div>
              <div className="rounded-md border p-4">
                <p className="text-xs text-muted-foreground">Anchors</p>
                <p className="text-lg font-semibold">
                  {semanticAnchorsQuery.isLoading ? '…' : anchorCount}
                </p>
              </div>
              <div className="rounded-md border p-4">
                <p className="text-xs text-muted-foreground">Semantic routes</p>
                <p className="text-lg font-semibold">
                  {semanticRoutesQuery.isLoading ? '…' : semanticRoutesCount}
                </p>
              </div>
            </div>
            <div className="mt-4 flex flex-wrap gap-2">
              <Button asChild variant="outline">
                <Link href="/semantic">
                  <Sparkles className="mr-2 h-4 w-4" /> Open Semantic page
                </Link>
              </Button>
              <Button asChild>
                <Link href="/semantic">
                  <Route className="mr-2 h-4 w-4" /> Test semantic routing
                </Link>
              </Button>
            </div>
          </SectionCard>
        </div>
      )}

      <Dialog open={routeGroupDialog.open} onOpenChange={(open) => !open && setRouteGroupDialog({ open: false, mode: 'create', name: '', models: [] })}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{routeGroupDialog.mode === 'edit' ? 'Edit route group' : 'Create route group'}</DialogTitle>
            <DialogDescription>Select models allowed for this tenant.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="route-group-name">Route group name</Label>
              <Input
                id="route-group-name"
                value={routeGroupDialog.name}
                onChange={(event) => setRouteGroupDialog((prev) => ({ ...prev, name: event.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label>Models</Label>
              <div className="max-h-60 overflow-y-auto rounded-md border p-3 space-y-2">
                {isLoadingModels ? (
                  <Skeleton className="h-32" />
                ) : routeGroupModelOptions.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No models available for this tenant.</p>
                ) : (
                  routeGroupModelOptions.map((model) => (
                    <label key={model} className="flex items-center gap-2 text-sm">
                      <Checkbox
                        checked={routeGroupDialog.models.includes(model)}
                        onCheckedChange={(checked) => {
                          const isChecked = checked === true
                          setRouteGroupDialog((prev) => ({
                            ...prev,
                            models: isChecked
                              ? [...prev.models, model]
                              : prev.models.filter((item) => item !== model),
                          }))
                        }}
                      />
                      <span>{model}</span>
                    </label>
                  ))
                )}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setRouteGroupDialog({ open: false, mode: 'create', name: '', models: [] })}
            >
              Cancel
            </Button>
            <Button
              onClick={handleRouteGroupSave}
              disabled={!routeGroupDialog.name.trim() || updateTenantConfig.isPending}
            >
              Save route group
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default function RoutingPage() {
  return (
    <RequireAdminRole>
      <RoutingContent />
    </RequireAdminRole>
  )
}
