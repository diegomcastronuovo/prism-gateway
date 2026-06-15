'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import type { CheckedState } from '@radix-ui/react-checkbox'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Checkbox } from '@/components/ui/checkbox'
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/hooks/use-toast'
import { useTenants, useTenantConfig } from '@/features/tenants/api/use-tenants'
import { useRouteGroups } from '@/features/route-groups/api/use-route-groups'
import {
  calibrateSemanticThreshold,
  createSemanticAnchor,
  createSemanticRoute,
  deleteSemanticAnchor,
  deleteSemanticRoute,
  getSemanticThreshold,
  listAdminModels,
  listSemanticAnchors,
  listSemanticRoutes,
  patchSemanticAnchor,
  runSemanticTest,
  suggestSemanticAnchors,
  updateSemanticThreshold,
} from '@/features/semantic/api/client'
import type {
  AdminModelOption,
  SemanticAnchor,
  SemanticCalibrationResponse,
  SemanticRoute,
  SemanticRouteListResponse,
  SemanticSuggestion,
  SemanticTestResponse,
} from '@/features/semantic/types'
import { Brain, Eraser, Info, Layers, Route, Sparkles, X } from 'lucide-react'

const ANCHOR_LIMIT = 50

type AnchorFormState = {
  name: string
  route_group: string
  anchor_text: string
  modality: string
  preferred_models: string[]
  tenant_id: string
}

const EMPTY_ANCHOR_FORM: AnchorFormState = {
  name: '',
  route_group: '',
  anchor_text: '',
  modality: 'text',
  preferred_models: [],
  tenant_id: '',
}

function normalizeStringArray(values: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const value of values) {
    const trimmed = value.trim()
    if (!trimmed || seen.has(trimmed)) continue
    seen.add(trimmed)
    result.push(trimmed)
  }
  return result
}

function arraysEqualIgnoreOrder(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  const sortedA = [...a].sort()
  const sortedB = [...b].sort()
  return sortedA.every((val, idx) => val === sortedB[idx])
}

type AnchorDialogState = {
  mode: 'create' | 'edit'
  anchor?: SemanticAnchor
  defaults?: Partial<{ name: string; route_group: string; text: string; modality: string; preferred_models: string[]; tenant_id: string }>
}

export default function SemanticPage() {
  return (
    <RequireAdminRole allowedRoles={['admin']}>
      <SemanticContent />
    </RequireAdminRole>
  )
}

function SemanticContent() {
  const { toast } = useToast()
  const queryClient = useQueryClient()
  const tenantsQuery = useTenants()
  const tenants = tenantsQuery.data || []
  const [selectedTenantId, setSelectedTenantId] = useState<string | null>(tenants[0]?.tenant_id ?? null)
  const tenantConfigQuery = useTenantConfig(selectedTenantId)
  const [includeAnchorText, setIncludeAnchorText] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [anchorDialog, setAnchorDialog] = useState<AnchorDialogState | null>(null)
  const [anchorToDelete, setAnchorToDelete] = useState<SemanticAnchor | null>(null)
  const [routeDialogOpen, setRouteDialogOpen] = useState(false)
  const [routeToDelete, setRouteToDelete] = useState<SemanticRoute | null>(null)
  const [playgroundInput, setPlaygroundInput] = useState('')
  const [playgroundResult, setPlaygroundResult] = useState<SemanticTestResponse | null>(null)
  const [playgroundError, setPlaygroundError] = useState<string | null>(null)
  const [suggestDataset, setSuggestDataset] = useState('')
  const [suggestMaxClusters, setSuggestMaxClusters] = useState(3)
  const [suggestions, setSuggestions] = useState<SemanticSuggestion[] | null>(null)
  const [calibrationRows, setCalibrationRows] = useState([{ text: '', route: '' }])
  const [calibrationResult, setCalibrationResult] = useState<SemanticCalibrationResponse | null>(null)
  const [thresholdDraft, setThresholdDraft] = useState<number | null>(null)
  const [anchorForm, setAnchorForm] = useState<AnchorFormState>(EMPTY_ANCHOR_FORM)
  const [preferredModelSearch, setPreferredModelSearch] = useState('')
  const [semanticInfoOpen, setSemanticInfoOpen] = useState(false)
  const suggestionSectionRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!selectedTenantId && tenants.length > 0) {
      setSelectedTenantId(tenants[0].tenant_id)
    }
  }, [selectedTenantId, tenants])


  useEffect(() => {
    if (!anchorDialog) {
      setAnchorForm(EMPTY_ANCHOR_FORM)
      setPreferredModelSearch('')
      return
    }
    setAnchorForm({
      name: anchorDialog.anchor?.name ?? anchorDialog.defaults?.name ?? '',
      route_group: anchorDialog.anchor?.route_group ?? anchorDialog.defaults?.route_group ?? '',
      anchor_text: anchorDialog.anchor?.anchor_text ?? anchorDialog.defaults?.text ?? '',
      modality: anchorDialog.anchor?.modality ?? anchorDialog.defaults?.modality ?? 'text',
      preferred_models: anchorDialog.anchor?.preferred_models ?? anchorDialog.defaults?.preferred_models ?? [],
      tenant_id: anchorDialog.anchor
        ? ''
        : (anchorDialog.defaults?.tenant_id ?? selectedTenantId ?? tenants[0]?.tenant_id ?? ''),
    })
    setPreferredModelSearch('')
  }, [anchorDialog, selectedTenantId, tenants])

  const anchorsQuery = useQuery({
    queryKey: ['semantic-anchors', selectedTenantId, includeAnchorText],
    queryFn: () => listSemanticAnchors({ limit: ANCHOR_LIMIT, includeAnchorText, tenantId: selectedTenantId }),
    enabled: Boolean(selectedTenantId),
  })

  const modelsQuery = useQuery<AdminModelOption[]>({
    queryKey: ['admin-models'],
    queryFn: listAdminModels,
  })

  const adminModels: AdminModelOption[] = modelsQuery.data ?? []
  const normalizedPreferredModels = useMemo(() => normalizeStringArray(anchorForm.preferred_models), [anchorForm.preferred_models])
  const normalizedOriginalPreferred = useMemo(
    () => normalizeStringArray(anchorDialog?.anchor?.preferred_models ?? []),
    [anchorDialog]
  )

  const anchorDialogTenantId = anchorDialog?.mode === 'create' ? anchorForm.tenant_id : selectedTenantId ?? ''
  const anchorDialogTenantConfigQuery = useTenantConfig(anchorDialogTenantId || null)
  const anchorDialogAllowedModels = useMemo(() => {
    const raw = anchorDialogTenantConfigQuery.data?.config?.allowed_models
    return Array.isArray(raw) ? (raw as string[]) : []
  }, [anchorDialogTenantConfigQuery.data])

  const filteredModelOptions = useMemo(() => {
    const allowedSet = anchorDialogAllowedModels.length > 0 ? new Set(anchorDialogAllowedModels) : null
    const base = allowedSet ? adminModels.filter((m) => allowedSet.has(m.id)) : adminModels
    const sorted = [...base].sort((a, b) => a.id.localeCompare(b.id))
    const needle = preferredModelSearch.trim().toLowerCase()
    if (!needle) return sorted
    return sorted.filter((model) => {
      const idMatch = model.id.toLowerCase().includes(needle)
      const providerMatch = model.provider?.toLowerCase().includes(needle)
      const routeMatch = (model.route_groups || []).some((group) => group.toLowerCase().includes(needle))
      return idMatch || providerMatch || routeMatch
    })
  }, [adminModels, anchorDialogAllowedModels, preferredModelSearch])
  const preferredModelsDisplay = normalizedPreferredModels

  const routesQuery = useQuery<SemanticRouteListResponse>({
    queryKey: ['semantic-routes', selectedTenantId],
    queryFn: () => (listSemanticRoutes as (tenantId: string) => Promise<SemanticRouteListResponse>)(selectedTenantId!),
    enabled: Boolean(selectedTenantId),
  })

  const routeGroupTenantId = anchorDialogTenantId
  const routeGroupsQuery = useRouteGroups(routeGroupTenantId)
  const routeGroupOptions = useMemo(() => {
    const options = (routeGroupsQuery.data ?? []).map((group) => group.id)
    if (anchorForm.route_group && !options.includes(anchorForm.route_group)) {
      options.push(anchorForm.route_group)
    }
    return options.sort((a, b) => a.localeCompare(b))
  }, [anchorForm.route_group, routeGroupsQuery.data])

  const routeGroupsFromAnchors = useMemo(() => {
    const anchors = anchorsQuery.data?.data ?? []
    const seen = new Set<string>()
    for (const a of anchors) {
      const rg = a.route_group?.trim()
      if (rg) seen.add(rg)
    }
    return Array.from(seen).sort((a, b) => a.localeCompare(b))
  }, [anchorsQuery.data])

  const calibrationRouteOptions = useMemo(() => {
    const seen = new Set<string>(routeGroupsFromAnchors)
    for (const row of calibrationRows) {
      const r = row.route.trim()
      if (r) seen.add(r)
    }
    return Array.from(seen).sort((a, b) => a.localeCompare(b))
  }, [routeGroupsFromAnchors, calibrationRows])

  const thresholdQuery = useQuery({
    queryKey: ['semantic-threshold', selectedTenantId],
    queryFn: () => (selectedTenantId ? getSemanticThreshold(selectedTenantId) : Promise.resolve({ tenant_id: '', threshold_default: null })),
    enabled: Boolean(selectedTenantId),
  })

  const filteredAnchors = useMemo(() => {
    const anchors = anchorsQuery.data?.data ?? []
    if (!searchTerm.trim()) return anchors
    const needle = searchTerm.toLowerCase()
    return anchors.filter((anchor) =>
      anchor.name.toLowerCase().includes(needle) || anchor.route_group.toLowerCase().includes(needle)
    )
  }, [anchorsQuery.data, searchTerm])

  const createAnchorMutation = useMutation({
    mutationFn: createSemanticAnchor,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['semantic-anchors'] })
      toast({ title: 'Anchor created' })
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to create anchor', description: error.message, variant: 'destructive' })
    },
  })

  const updateAnchorMutation = useMutation({
    mutationFn: ({ name, payload }: { name: string; payload: Record<string, unknown> }) => {
      if (!selectedTenantId) throw new Error('Select a tenant')
      return (patchSemanticAnchor as (tenantId: string, name: string, payload: Record<string, unknown>) => Promise<SemanticAnchor>)(
        selectedTenantId,
        name,
        payload
      )
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['semantic-anchors'] })
      toast({ title: 'Anchor updated' })
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to update anchor', description: error.message, variant: 'destructive' })
    },
  })

  const deleteAnchorMutation = useMutation({
    mutationFn: (name: string) => {
      if (!selectedTenantId) throw new Error('Select a tenant')
      return (deleteSemanticAnchor as (tenantId: string, name: string) => Promise<void>)(selectedTenantId, name)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['semantic-anchors'] })
      toast({ title: 'Anchor deleted' })
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to delete anchor', description: error.message, variant: 'destructive' })
    },
  })

  const createRouteMutation = useMutation({
    mutationFn: (payload: { name: string; action: string; description?: string; utterances?: string[] }) => {
      if (!selectedTenantId) throw new Error('Select a tenant')
      return (createSemanticRoute as (
        tenantId: string,
        payload: { name: string; action: string; description?: string; utterances?: string[] }
      ) => Promise<SemanticRoute>)(selectedTenantId, payload)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['semantic-routes', selectedTenantId] })
      toast({ title: 'Route created' })
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to create route', description: error.message, variant: 'destructive' })
    },
  })

  const deleteRouteMutation = useMutation({
    mutationFn: (name: string) => {
      if (!selectedTenantId) throw new Error('Select a tenant')
      return (deleteSemanticRoute as (tenantId: string, name: string) => Promise<void>)(selectedTenantId, name)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['semantic-routes', selectedTenantId] })
      toast({ title: 'Route deleted' })
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to delete route', description: error.message, variant: 'destructive' })
    },
  })

  const playgroundMutation = useMutation({
    mutationFn: runSemanticTest,
    onSuccess: (data) => {
      setPlaygroundResult(data)
      setPlaygroundError(null)
    },
    onError: (error: Error) => {
      setPlaygroundResult(null)
      setPlaygroundError(error.message)
    },
  })

  const suggestMutation = useMutation({
    mutationFn: suggestSemanticAnchors,
    onSuccess: (data) => {
      setSuggestions(data.anchors || [])
      toast({ title: 'Suggestions ready' })
    },
    onError: (error: Error) => {
      toast({ title: 'Suggestion failed', description: error.message, variant: 'destructive' })
    },
  })

  const calibrateMutation = useMutation<SemanticCalibrationResponse, Error, { dataset: { text: string; route: string }[]; tenantId: string }>({
    mutationFn: (payload) => calibrateSemanticThreshold(payload as { dataset: { text: string; route: string }[]; tenantId: string }),
    onSuccess: (data) => {
      setCalibrationResult(data)
      toast({ title: 'Calibration completed' })
    },
    onError: (error: Error) => {
      toast({ title: 'Calibration failed', description: error.message, variant: 'destructive' })
    },
  })

  const updateThresholdMutation = useMutation({
    mutationFn: updateSemanticThreshold,
    onSuccess: (data) => {
      toast({ title: 'Threshold updated' })
      queryClient.invalidateQueries({ queryKey: ['semantic-threshold', data.tenant_id] })
      setThresholdDraft(null)
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to update threshold', description: error.message, variant: 'destructive' })
    },
  })

  const currentThreshold = thresholdQuery.data?.threshold_default ?? null
  const displayThreshold = thresholdDraft ?? currentThreshold ?? 0
  const thresholdDirty = thresholdDraft != null && thresholdDraft !== currentThreshold

  useEffect(() => {
    if (currentThreshold != null && thresholdDraft == null) {
      setThresholdDraft(currentThreshold)
    }
  }, [currentThreshold, thresholdDraft])

  const scrollToSuggestionSection = () => {
    suggestionSectionRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  function handleOpenCreateAnchor(defaults?: AnchorDialogState['defaults']) {
    setAnchorDialog({
      mode: 'create',
      defaults: { tenant_id: selectedTenantId ?? '', ...defaults },
    })
  }

  const trimmedAnchorName = anchorForm.name.trim()
  const trimmedRouteGroup = anchorForm.route_group.trim()
  const trimmedAnchorText = anchorForm.anchor_text.trim()
  const selectedAnchorTenant = anchorForm.tenant_id || selectedTenantId || ''
  const preferredChanged =
    anchorDialog?.mode === 'edit' && anchorDialog.anchor
      ? !arraysEqualIgnoreOrder(normalizedPreferredModels, normalizedOriginalPreferred)
      : false

  function handleSubmitAnchor(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!anchorDialog) return
    if (disableSemanticActions) return
    if (!trimmedAnchorName || !trimmedRouteGroup) return
    const preferredForSave = normalizedPreferredModels
    if (anchorDialog.mode === 'create') {
      if (!selectedAnchorTenant) return
      const payload: {
        name: string
        route_group: string
        modality?: string
        text?: string
        preferred_models?: string[]
        tenantId?: string
      } = {
        name: trimmedAnchorName,
        route_group: trimmedRouteGroup,
        modality: anchorForm.modality,
      }
      if (trimmedAnchorText) {
        payload.text = trimmedAnchorText
      }
      if (preferredForSave.length > 0) {
        payload.preferred_models = preferredForSave
      }
      createAnchorMutation.mutate({ ...payload, tenantId: selectedAnchorTenant }, {
        onSuccess: () => {
          setAnchorDialog(null)
        },
      })
    } else if (anchorDialog.anchor) {
      const patchPayload: Record<string, unknown> = {}
      if (trimmedRouteGroup !== anchorDialog.anchor.route_group) {
        patchPayload.route_group = trimmedRouteGroup
      }
      if (trimmedAnchorText !== (anchorDialog.anchor.anchor_text ?? '')) {
        patchPayload.anchor_text = trimmedAnchorText
      }
      if (preferredChanged) {
        patchPayload.preferred_models = preferredForSave
      }
      if (Object.keys(patchPayload).length === 0) {
        return
      }
      updateAnchorMutation.mutate(
        { name: anchorDialog.anchor.name, payload: patchPayload },
        {
          onSuccess: () => setAnchorDialog(null),
        }
      )
    }
  }

  const anchorHasChanges =
    anchorDialog?.mode === 'edit' && anchorDialog.anchor
      ? trimmedRouteGroup !== anchorDialog.anchor.route_group ||
        trimmedAnchorText !== (anchorDialog.anchor.anchor_text ?? '') ||
        preferredChanged
      : true

  function togglePreferredModel(modelId: string, checked: CheckedState) {
    const isChecked = checked === true
    setAnchorForm((prev) => {
      const exists = prev.preferred_models.includes(modelId)
      if (isChecked && !exists) {
        return { ...prev, preferred_models: [...prev.preferred_models, modelId] }
      }
      if (!isChecked && exists) {
        return { ...prev, preferred_models: prev.preferred_models.filter((id) => id !== modelId) }
      }
      return prev
    })
  }

  function removePreferredModel(modelId: string) {
    setAnchorForm((prev) => ({
      ...prev,
      preferred_models: prev.preferred_models.filter((id) => id !== modelId),
    }))
  }

  const anchorSubmitDisabled =
    createAnchorMutation.isPending ||
    updateAnchorMutation.isPending ||
    !trimmedAnchorName ||
    !trimmedRouteGroup ||
    (anchorDialog?.mode === 'edit' ? !anchorHasChanges : false) ||
    (anchorDialog?.mode === 'create' && !selectedAnchorTenant)

  function handleDeleteAnchor() {
    if (!anchorToDelete) return
    deleteAnchorMutation.mutate(anchorToDelete.name, {
      onSuccess: () => setAnchorToDelete(null),
    })
  }

  function handleRunPlayground() {
    if (disableSemanticActions) return
    if (!playgroundInput.trim()) {
      setPlaygroundError('Enter text to test')
      setPlaygroundResult(null)
      return
    }
    if (!selectedTenantId) {
      setPlaygroundError('Select a tenant before testing')
      setPlaygroundResult(null)
      return
    }
    playgroundMutation.mutate({ text: playgroundInput.trim(), tenantId: selectedTenantId })
  }

  function handleSuggestAnchors() {
    if (disableSemanticActions) return
    const dataset = suggestDataset
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
    if (dataset.length === 0) {
      toast({ title: 'Dataset required', description: 'Add at least one line', variant: 'destructive' })
      return
    }
    if (!selectedTenantId) {
      toast({ title: 'Tenant required', description: 'Select a tenant before generating suggestions', variant: 'destructive' })
      return
    }
    suggestMutation.mutate({ dataset, max_clusters: suggestMaxClusters, tenantId: selectedTenantId })
  }

  function handleCalibrate() {
    if (disableSemanticActions) return
    if (!selectedTenantId) {
      toast({ title: 'Tenant required', description: 'Select a tenant before calibrating', variant: 'destructive' })
      return
    }
    const dataset = calibrationRows
      .map((row) => ({ text: row.text.trim(), route: row.route.trim() }))
      .filter((row) => row.text && row.route)
    if (dataset.length === 0) {
      toast({ title: 'Dataset required', description: 'Add at least one labeled row', variant: 'destructive' })
      return
    }
    calibrateMutation.mutate({ dataset, tenantId: selectedTenantId })
  }

  function handleApplyRecommendedThreshold() {
    if (disableSemanticActions) return
    if (!calibrationResult || !selectedTenantId) return
    updateThresholdMutation.mutate({ tenant_id: selectedTenantId, threshold_default: calibrationResult.recommended_threshold })
  }

  function handleCreateRoute(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (disableSemanticActions) return
    const form = event.currentTarget
    if (!selectedTenantId) {
      toast({ title: 'Tenant required', description: 'Select a tenant before creating a route', variant: 'destructive' })
      return
    }
    const formData = new FormData(form)
    const name = (formData.get('name') as string).trim()
    const action = (formData.get('action') as string).trim()
    if (!name || !action) return
    const description = (formData.get('description') as string).trim()
    const utterances = ((formData.get('utterances') as string) || '')
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
    createRouteMutation.mutate(
      { name, action, ...(description ? { description } : {}), utterances },
      {
        onSuccess: () => {
          setRouteDialogOpen(false)
          form.reset()
        },
      }
    )
  }

  function handleDeleteRoute() {
    if (!routeToDelete) return
    if (disableSemanticActions) return
    deleteRouteMutation.mutate(routeToDelete.name, {
      onSuccess: () => setRouteToDelete(null),
    })
  }

  const anchorsError = anchorsQuery.error as Error | null
  const routesError = routesQuery.error as Error | null
  const thresholdError = thresholdQuery.error as Error | null
  const anchorsTenantLabel = anchorsQuery.data?.tenant_id
  const embeddingModel = useMemo(() => {
    const config = tenantConfigQuery.data?.config as Record<string, unknown> | undefined
    const routing = config?.routing as Record<string, unknown> | undefined
    const semantic = routing?.semantic as Record<string, unknown> | undefined
    return typeof semantic?.embedding_model === 'string' ? semantic.embedding_model : ''
  }, [tenantConfigQuery.data])
  const hasSemanticEmbeddingModel = embeddingModel.trim().length > 0
  const disableSemanticActions = !hasSemanticEmbeddingModel

  const isSemanticRoutingEnabled = useMemo(() => {
    const config = tenantConfigQuery.data?.config as Record<string, unknown> | undefined
    if (!config) return false
    const routing = config.routing as Record<string, unknown> | undefined
    const smart = routing?.smart as Record<string, unknown> | undefined
    const stages = smart?.stages as Array<Record<string, unknown>> | undefined
    if (!Array.isArray(stages)) return false
    const semanticStage = stages.find((stage) => stage?.name === 'semantic_intent')
    if (!semanticStage) return false
    const rules = semanticStage.rules as unknown
    return Array.isArray(rules) && rules.length > 0
  }, [tenantConfigQuery.data])

  return (
    <div>
      <PageHeader
        title="Semantic Control"
        description="Manage anchors, thresholds, routes, and semantic tests. This configuration is tenant-specific."
        action={
          <div className="flex w-full max-w-full flex-col gap-3 sm:w-auto sm:max-w-none sm:flex-row sm:items-end sm:justify-end sm:gap-3">
            <div className="w-full min-w-0 sm:w-56">
              <Label htmlFor="page-tenant" className="text-xs text-muted-foreground">
                Tenant
              </Label>
              <Select
                value={selectedTenantId ?? ''}
                onValueChange={(value) => {
                  setSelectedTenantId(value)
                  setThresholdDraft(null)
                }}
              >
                <SelectTrigger id="page-tenant" className="mt-1">
                  <SelectValue placeholder="Select a tenant" />
                </SelectTrigger>
                <SelectContent>
                  {tenants.map((tenant) => (
                    <SelectItem key={tenant.tenant_id} value={tenant.tenant_id}>
                      {tenant.tenant_id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <div className="mt-2 text-xs text-muted-foreground">
                Embedding model for tenant:{' '}
                <span className={hasSemanticEmbeddingModel ? 'text-foreground' : 'text-destructive'}>
                  {hasSemanticEmbeddingModel ? embeddingModel : 'Not configured'}
                </span>
              </div>
            </div>
            <div className="flex flex-col">
              <span className="text-xs text-muted-foreground opacity-0 select-none">Tenant</span>
              <Button
                type="button"
                variant="default"
                className="shrink-0 px-6 mt-1"
                title="Learn how semantic routing works"
                aria-label="Detailed Information. Learn how semantic routing works"
                onClick={() => setSemanticInfoOpen(true)}
              >
                <Info className="mr-2 h-4 w-4" />
                Detailed Information
              </Button>
            </div>
          </div>
        }
      />
      {disableSemanticActions && (
        <div className="mb-6 rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          <p>Semantic features are disabled because this tenant does not have an embedding model configured.</p>
          <p className="mt-1 text-xs text-muted-foreground">
            Configure “Embedding model for tenant” in the tenant features/configuration before using Semantic.
          </p>
        </div>
      )}

      <div className="space-y-6">
        {selectedTenantId && !tenantConfigQuery.isLoading && (
          <div
            className={
              isSemanticRoutingEnabled
                ? 'rounded-lg border p-4'
                : 'rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-900 dark:bg-red-950/20'
            }
          >
            <p className={isSemanticRoutingEnabled ? 'text-sm font-medium' : 'text-sm font-medium text-red-700 dark:text-red-400'}>
              Semantic Routing is{' '}
              <span className={isSemanticRoutingEnabled ? 'text-green-600 dark:text-green-400' : 'text-red-700 dark:text-red-400'}>
                {isSemanticRoutingEnabled ? 'enabled' : 'disabled'}
              </span>{' '}
              for tenant <span className="font-semibold">{selectedTenantId}</span>
            </p>
            {!isSemanticRoutingEnabled && (
              <p className="mt-1 text-xs text-red-600/80 dark:text-red-500/80">
                None of the configuration on this page will be active until Semantic Routing is enabled for this tenant. Enable it from the tenant Features section.
              </p>
            )}
          </div>
        )}

        <SectionCard
          title="Semantic Anchors"
          description="Manage anchor definitions for semantic routing"
          className="border-t-4 border-t-pink-500"
        >
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
              <div className="flex flex-col gap-2 md:flex-row md:items-center md:gap-4">
                <Input
                  placeholder="Search anchors"
                  value={searchTerm}
                  onChange={(event) => setSearchTerm(event.target.value)}
                  className="w-full md:w-72"
                  disabled={disableSemanticActions}
                />
                <div className="flex items-center gap-2">
                  <Switch
                    id="include-anchor-text"
                    checked={includeAnchorText}
                    onCheckedChange={(value) => setIncludeAnchorText(Boolean(value))}
                    disabled={disableSemanticActions}
                  />
                  <Label htmlFor="include-anchor-text" className="text-sm text-muted-foreground">
                    Show anchor text
                  </Label>
                </div>
              </div>
              <div className="flex gap-2">
                <Button variant="outline" onClick={scrollToSuggestionSection} disabled={disableSemanticActions}>
                  <Sparkles className="mr-2 h-4 w-4" /> Suggest Anchors
                </Button>
                <Button onClick={() => handleOpenCreateAnchor()} disabled={disableSemanticActions}>
                  <Brain className="mr-2 h-4 w-4" /> Create Anchor
                </Button>
              </div>
            </div>

            {anchorsQuery.isLoading ? (
              <Skeleton className="h-64" />
            ) : anchorsError ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
                {anchorsError.message}
              </div>
            ) : filteredAnchors.length === 0 ? (
              <div className="space-y-4">
                  <EmptyState
                    icon={Brain}
                    title="No semantic anchors"
                    description="Create anchors or use suggestions to bootstrap semantic routing"
                    action={
                      <Button onClick={() => handleOpenCreateAnchor()} disabled={disableSemanticActions}>
                        Create anchor
                      </Button>
                    }
                  />
                <div className="flex justify-center">
                  <Button variant="outline" onClick={scrollToSuggestionSection} disabled={disableSemanticActions}>
                    Suggest anchors
                  </Button>
                </div>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                      <TableRow>
                        <TableHead>Tenant</TableHead>
                        <TableHead>Name</TableHead>
                        <TableHead>Route Group</TableHead>
                        <TableHead>Preferred Models</TableHead>
                        <TableHead>Modality</TableHead>
                        <TableHead>Vector Dims</TableHead>
                        {includeAnchorText && <TableHead>Anchor Text</TableHead>}
                        <TableHead className="text-right">Actions</TableHead>
                      </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredAnchors.map((anchor) => (
                      <TableRow key={anchor.name}>
                        <TableCell>{anchorsTenantLabel ?? '—'}</TableCell>
                        <TableCell className="font-semibold">{anchor.name}</TableCell>
                        <TableCell>{anchor.route_group}</TableCell>
                        <TableCell>
                          {anchor.preferred_models && anchor.preferred_models.length > 0 ? (
                            <div className="flex flex-wrap gap-1">
                              {anchor.preferred_models.map((model) => (
                                <Badge key={model} variant="outline">
                                  {model}
                                </Badge>
                              ))}
                            </div>
                          ) : (
                            <span className="text-xs text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        <TableCell>{anchor.modality || 'text'}</TableCell>
                        <TableCell>{anchor.vector_dims ?? '—'}</TableCell>
                        {includeAnchorText && (
                          <TableCell className="max-w-sm">
                            {anchor.anchor_text ? (
                              <p className="truncate text-sm text-muted-foreground" title={anchor.anchor_text}>
                                {anchor.anchor_text}
                              </p>
                            ) : (
                              <span className="text-xs text-muted-foreground">Not available</span>
                            )}
                          </TableCell>
                        )}
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setAnchorDialog({ mode: 'edit', anchor })}
                              disabled={disableSemanticActions}
                            >
                              Edit
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="text-destructive"
                              onClick={() => setAnchorToDelete(anchor)}
                              disabled={disableSemanticActions}
                            >
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
          </div>
        </SectionCard>

        <SectionCard
          title="Semantic Threshold"
          description="Control pass/fail sensitivity per tenant"
          className="border-t-4 border-t-cyan-400"
        >
          <div className="space-y-4">
            <div className="flex flex-col gap-4 md:flex-row md:items-center md:gap-6">
              <div className="md:w-64">
                <Label htmlFor="threshold-tenant">Tenant</Label>
                <Select
                  value={selectedTenantId ?? ''}
                  onValueChange={(value) => {
                    setSelectedTenantId(value)
                    setThresholdDraft(null)
                  }}
                  disabled={disableSemanticActions}
                >
                  <SelectTrigger id="threshold-tenant">
                    <SelectValue placeholder="Select tenant" />
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
              <div className="flex-1 space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-sm">
                    Threshold ({displayThreshold.toFixed(2)})
                  </Label>
                  <Input
                    type="number"
                    step="0.01"
                    min="0"
                    max="1"
                    className="w-24"
                    value={displayThreshold.toFixed(2)}
                    disabled={disableSemanticActions}
                    onChange={(event) => {
                      const value = Number(event.target.value)
                      setThresholdDraft(Number.isFinite(value) ? value : null)
                    }}
                  />
                </div>
                <input
                  type="range"
                  min="0"
                  max="1"
                  step="0.01"
                  value={Math.min(1, Math.max(0, displayThreshold))}
                  disabled={disableSemanticActions}
                  onChange={(event) => setThresholdDraft(Number(event.target.value))}
                  className="w-full"
                />
                <p className="text-xs text-muted-foreground">Lower values are more permissive; higher values are stricter.</p>
              </div>
              <Button
                disabled={disableSemanticActions || !thresholdDirty || !selectedTenantId || updateThresholdMutation.isPending}
                onClick={() => {
                  if (!selectedTenantId || thresholdDraft == null) return
                  updateThresholdMutation.mutate({ tenant_id: selectedTenantId, threshold_default: Number(thresholdDraft.toFixed(4)) })
                }}
              >
                Save threshold
              </Button>
            </div>
            {thresholdQuery.isLoading && <Skeleton className="h-10" />}
            {thresholdError && <div className="text-sm text-destructive">{thresholdError.message}</div>}
          </div>
        </SectionCard>

        <SectionCard
          title="Semantic Playground"
          description="Test semantic matches in real time"
          className="border-t-4 border-t-purple-500"
        >
          <div className="space-y-4">
            <Textarea
              placeholder="Enter text to test semantic routing"
              value={playgroundInput}
              onChange={(event) => setPlaygroundInput(event.target.value)}
              onKeyDown={(event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
                  handleRunPlayground()
                }
              }}
              disabled={disableSemanticActions}
            />
            <div className="flex items-center justify-between">
              <p className="text-xs text-muted-foreground">Cmd/Ctrl+Enter to run</p>
              <Button onClick={handleRunPlayground} disabled={disableSemanticActions || playgroundMutation.isPending}>
                {playgroundMutation.isPending ? 'Testing…' : 'Test semantic match'}
              </Button>
            </div>
            {playgroundError && (
              <div className="rounded border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
                {playgroundError}
              </div>
            )}
            {playgroundResult && (
              <div className="rounded-lg border bg-card p-4">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="outline">Threshold {playgroundResult.threshold.toFixed(2)}</Badge>
                  {playgroundResult.top_match ? (
                    <Badge variant={playgroundResult.top_match.passed ? 'default' : 'secondary'}>
                      {playgroundResult.top_match.passed ? 'Passed' : 'Failed'}
                    </Badge>
                  ) : (
                    <Badge variant="secondary">No match</Badge>
                  )}
                </div>
                {playgroundResult.top_match ? (
                  <dl className="mt-4 grid gap-3 md:grid-cols-4 text-sm">
                    <div>
                      <dt className="text-muted-foreground">Top anchor</dt>
                      <dd className="font-medium">{playgroundResult.top_match.anchor}</dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Route group</dt>
                      <dd className="font-medium">{playgroundResult.top_match.route_group}</dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Similarity</dt>
                      <dd className="font-medium">{playgroundResult.top_match.similarity.toFixed(3)}</dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Decision</dt>
                      <dd className="font-medium">{playgroundResult.decision?.result || '—'}</dd>
                    </div>
                  </dl>
                ) : (
                  <p className="mt-4 text-sm text-muted-foreground">No anchor exceeded the threshold.</p>
                )}
              </div>
            )}
          </div>
        </SectionCard>

        <div ref={suggestionSectionRef}>
          <SectionCard
            title="Anchor Suggestions"
            description="Generate candidate anchors from sample prompts"
            className="border-t-4 border-t-amber-400"
          >
          <div className="space-y-4">
            <div className="grid gap-4 md:grid-cols-[2fr_1fr]">
              <div>
                <Label>Dataset (one prompt per line)</Label>
                <Textarea
                  rows={6}
                  value={suggestDataset}
                  onChange={(event) => setSuggestDataset(event.target.value)}
                  placeholder={'stock market outlook\nbanking regulation\ninterest rate forecast'}
                  disabled={disableSemanticActions}
                />
              </div>
              <div>
                <Label htmlFor="max-clusters">Max clusters</Label>
                <Input
                  id="max-clusters"
                  type="number"
                  min="1"
                  max="20"
                  value={suggestMaxClusters}
                  onChange={(event) => setSuggestMaxClusters(Number(event.target.value) || 1)}
                  className="mb-4"
                  disabled={disableSemanticActions}
                />
                <Button onClick={handleSuggestAnchors} disabled={disableSemanticActions || suggestMutation.isPending} className="w-full">
                  {suggestMutation.isPending ? 'Suggesting…' : 'Suggest anchors'}
                </Button>
              </div>
            </div>
            {suggestions && suggestions.length > 0 && (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Suggested anchor</TableHead>
                      <TableHead>Examples</TableHead>
                      <TableHead className="text-right">Action</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {suggestions.map((suggestion) => (
                      <TableRow key={suggestion.anchor}>
                        <TableCell>{suggestion.anchor}</TableCell>
                        <TableCell>{suggestion.examples}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() =>
                              handleOpenCreateAnchor({
                                name: suggestion.anchor.replace(/\s+/g, '_'),
                                text: suggestion.anchor,
                                route_group: '',
                              })
                            }
                            disabled={disableSemanticActions}
                          >
                            Use as anchor
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>
          </SectionCard>
        </div>

        <SectionCard
          title="Threshold Calibration"
          description="Recommend a threshold from labeled utterances"
          className="border-t-4 border-t-blue-500"
        >
          <div className="space-y-4">
            <div className="space-y-3">
              {calibrationRows.map((row, index) => (
                <div key={index} className="grid gap-3 md:grid-cols-2">
                  <div>
                    <Label>Text</Label>
                    <Input
                      value={row.text}
                      onChange={(event) => {
                        const next = [...calibrationRows]
                        next[index] = { ...row, text: event.target.value }
                        setCalibrationRows(next)
                      }}
                      disabled={disableSemanticActions}
                    />
                  </div>
                  <div className="flex gap-2">
                    <div className="flex-1">
                      <Label>Expected route</Label>
                      {calibrationRouteOptions.length > 0 ? (
                        <Select
                          value={row.route || undefined}
                          onValueChange={(value) => {
                            const next = [...calibrationRows]
                            next[index] = { ...row, route: value }
                            setCalibrationRows(next)
                          }}
                          disabled={disableSemanticActions}
                        >
                          <SelectTrigger className="w-full">
                            <SelectValue placeholder="Select route group" />
                          </SelectTrigger>
                          <SelectContent>
                            {calibrationRouteOptions.map((opt) => (
                              <SelectItem key={opt} value={opt}>
                                {opt}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <Input
                          value={row.route}
                          onChange={(event) => {
                            const next = [...calibrationRows]
                            next[index] = { ...row, route: event.target.value }
                            setCalibrationRows(next)
                          }}
                          placeholder="No route groups in anchors yet — type expected label"
                          disabled={disableSemanticActions}
                        />
                      )}
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      className="self-end"
                      onClick={() => setCalibrationRows((rows) => rows.filter((_, idx) => idx !== index))}
                      disabled={disableSemanticActions || calibrationRows.length === 1}
                    >
                      <Eraser className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setCalibrationRows((rows) => [...rows, { text: '', route: '' }])}
                disabled={disableSemanticActions}
              >
                Add row
              </Button>
              <Button onClick={handleCalibrate} disabled={disableSemanticActions || !selectedTenantId || calibrateMutation.isPending}>
                {calibrateMutation.isPending ? 'Calibrating…' : 'Calibrate threshold'}
              </Button>
            </div>
            {calibrationResult && (
              <div className="rounded-lg border bg-card p-4">
                <div className="flex flex-wrap gap-4">
                  <div>
                    <p className="text-xs text-muted-foreground">Recommended threshold</p>
                    <p className="text-xl font-semibold">{calibrationResult.recommended_threshold.toFixed(2)}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Precision</p>
                    <p className="text-lg font-medium">{(calibrationResult.precision ?? 0).toFixed(2)}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Recall</p>
                    <p className="text-lg font-medium">{(calibrationResult.recall ?? 0).toFixed(2)}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">F1</p>
                    <p className="text-lg font-medium">{(calibrationResult.f1 ?? 0).toFixed(2)}</p>
                  </div>
                </div>
                <Button
                  className="mt-4"
                  disabled={disableSemanticActions || !selectedTenantId || updateThresholdMutation.isPending}
                  onClick={handleApplyRecommendedThreshold}
                >
                  Apply recommended threshold
                </Button>
              </div>
            )}
          </div>
        </SectionCard>

        <SectionCard
          title="Semantic Routes"
          description="Manage semantic-driven tool routing"
          className="border-t-4 border-t-emerald-500"
        >
          <div className="space-y-4">
            <div className="flex justify-end">
              <Button disabled={disableSemanticActions || !selectedTenantId} onClick={() => setRouteDialogOpen(true)}>
                <Route className="mr-2 h-4 w-4" /> Create route
              </Button>
            </div>
            {routesQuery.isLoading ? (
              <Skeleton className="h-48" />
            ) : routesError ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
                {routesError.message}
              </div>
            ) : (routesQuery.data?.routes?.length ?? 0) === 0 ? (
              <EmptyState
                icon={Layers}
                title="No semantic routes"
                description="Add semantic routes to drive tool-routing decisions"
                action={
                  <Button disabled={disableSemanticActions || !selectedTenantId} onClick={() => setRouteDialogOpen(true)}>
                    Create route
                  </Button>
                }
              />
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Name</TableHead>
                      <TableHead>Description</TableHead>
                      <TableHead>Action</TableHead>
                      <TableHead>Threshold</TableHead>
                      <TableHead>Utterances</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {routesQuery.data?.routes?.map((route) => (
                      <TableRow key={route.name}>
                        <TableCell className="font-semibold">{route.name}</TableCell>
                        <TableCell>{route.description || '—'}</TableCell>
                        <TableCell>{route.action}</TableCell>
                        <TableCell>{route.threshold?.toFixed(2) ?? '—'}</TableCell>
                        <TableCell>{route.utterances?.length ?? 0}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive"
                            onClick={() => setRouteToDelete(route)}
                            disabled={disableSemanticActions}
                          >
                            Delete
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>
        </SectionCard>
      </div>

      <Dialog open={anchorDialog !== null} onOpenChange={(open) => !open && setAnchorDialog(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{anchorDialog?.mode === 'edit' ? 'Edit anchor' : 'Create anchor'}</DialogTitle>
            <DialogDescription>Manage semantic anchor details</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={handleSubmitAnchor}>
            {anchorDialog?.mode === 'create' && (
              <div>
                <Label htmlFor="anchor-tenant">Tenant</Label>
                <Select
                  value={anchorForm.tenant_id}
                  onValueChange={(value) => setAnchorForm((prev) => ({ ...prev, tenant_id: value }))}
                  disabled={disableSemanticActions}
                >
                  <SelectTrigger id="anchor-tenant">
                    <SelectValue placeholder="Select tenant" />
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
            )}
            <div>
              <Label htmlFor="anchor-name">Name</Label>
              <Input
                id="anchor-name"
                name="name"
                value={anchorForm.name}
                onChange={(event) => setAnchorForm((prev) => ({ ...prev, name: event.target.value }))}
                readOnly={anchorDialog?.mode === 'edit'}
                required
                disabled={disableSemanticActions}
              />
            </div>
            <div>
              <Label htmlFor="anchor-route">Route group</Label>
              {routeGroupsQuery.isLoading ? (
                <Skeleton className="h-10" />
              ) : routeGroupOptions.length > 0 ? (
                <Select
                  value={anchorForm.route_group || undefined}
                  onValueChange={(value) => setAnchorForm((prev) => ({ ...prev, route_group: value }))}
                  disabled={disableSemanticActions || !routeGroupTenantId}
                >
                  <SelectTrigger id="anchor-route">
                    <SelectValue placeholder="Select route group" />
                  </SelectTrigger>
                  <SelectContent>
                    {routeGroupOptions.map((group) => (
                      <SelectItem key={group} value={group}>
                        {group}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  id="anchor-route"
                  name="route_group"
                  value={anchorForm.route_group}
                  onChange={(event) => setAnchorForm((prev) => ({ ...prev, route_group: event.target.value }))}
                  placeholder={routeGroupTenantId ? 'No route groups for this tenant' : 'Select a tenant first'}
                  required
                  disabled={disableSemanticActions || !routeGroupTenantId}
                />
              )}
            </div>
            <div>
              <Label htmlFor="anchor-text">Anchor text</Label>
              <Textarea
                id="anchor-text"
                name="anchor_text"
                rows={4}
                value={anchorForm.anchor_text}
                onChange={(event) => setAnchorForm((prev) => ({ ...prev, anchor_text: event.target.value }))}
                placeholder="banking finance markets investments"
                disabled={disableSemanticActions}
              />
            </div>
            <div>
              <Label>Preferred Models</Label>
              {modelsQuery.isLoading ? (
                <Skeleton className="mt-2 h-32 w-full" />
              ) : modelsQuery.isError ? (
                <div className="mt-2 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
                  Failed to load available models.
                </div>
              ) : (
                <div className="mt-2 space-y-2">
                  <Input
                    placeholder="Search models"
                    value={preferredModelSearch}
                    onChange={(event) => setPreferredModelSearch(event.target.value)}
                    className="h-8"
                    disabled={disableSemanticActions}
                  />
                  <div className="max-h-48 overflow-y-auto rounded-md border p-2 space-y-2">
                    {filteredModelOptions.map((model) => (
                      <label key={model.id} className="flex items-start gap-2 text-sm">
                        <Checkbox
                          checked={preferredModelsDisplay.includes(model.id)}
                          onCheckedChange={(checked) => togglePreferredModel(model.id, checked)}
                          disabled={disableSemanticActions}
                        />
                        <div>
                          <p className="font-medium leading-tight">{model.id}</p>
                          <p className="text-xs text-muted-foreground">
                            {model.provider}
                            {model.route_groups?.length ? ` · ${model.route_groups.join(', ')}` : ''}
                          </p>
                        </div>
                      </label>
                    ))}
                    {filteredModelOptions.length === 0 && (
                      <p className="text-sm text-muted-foreground">No models match your search.</p>
                    )}
                  </div>
                  {preferredModelsDisplay.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {preferredModelsDisplay.map((modelId) => (
                        <Badge key={modelId} variant="secondary" className="flex items-center gap-1">
                          {modelId}
                          <button
                            type="button"
                            className="text-muted-foreground transition hover:text-foreground"
                            onClick={() => removePreferredModel(modelId)}
                            aria-label={`Remove ${modelId}`}
                            disabled={disableSemanticActions}
                          >
                            <X className="h-3 w-3" />
                          </button>
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">Leave empty to allow any eligible model.</p>
                  )}
                </div>
              )}
            </div>
            <div>
              <Label htmlFor="anchor-modality">Modality</Label>
              <Select
                value={anchorForm.modality}
                onValueChange={(value) => setAnchorForm((prev) => ({ ...prev, modality: value }))}
                disabled={anchorDialog?.mode === 'edit' || disableSemanticActions}
              >
                <SelectTrigger id="anchor-modality" disabled={anchorDialog?.mode === 'edit' || disableSemanticActions}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="text">Text</SelectItem>
                  <SelectItem value="image">Image</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setAnchorDialog(null)}>
                Cancel
              </Button>
              <Button type="submit" disabled={anchorSubmitDisabled}>
                {anchorDialog?.mode === 'edit' ? 'Save changes' : 'Create anchor'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={semanticInfoOpen} onOpenChange={setSemanticInfoOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Semantic Routing Overview</DialogTitle>
            <DialogDescription>
              Semantic routing uses anchors, route groups, and thresholds to classify intent and drive the routing decision.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3 text-sm text-muted-foreground">
            <p>Anchors define representative intents.</p>
            <p>Routes map those intents to downstream actions.</p>
            <p>Thresholds control how strict the matching must be.</p>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setSemanticInfoOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={anchorToDelete !== null} onOpenChange={(open) => !open && setAnchorToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete semantic anchor?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the anchor from semantic routing. This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={handleDeleteAnchor}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={routeDialogOpen} onOpenChange={setRouteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create semantic route</DialogTitle>
            <DialogDescription>Route semantic intents to downstream actions</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={handleCreateRoute}>
            <div>
              <Label htmlFor="route-name">Name</Label>
              <Input id="route-name" name="name" required />
            </div>
            <div>
              <Label htmlFor="route-action">Action</Label>
              <Input id="route-action" name="action" required placeholder="weather_api" />
            </div>
            <div>
              <Label htmlFor="route-description">Description</Label>
              <Input id="route-description" name="description" placeholder="Route weather questions to tool" />
            </div>
            <div>
              <Label htmlFor="route-utterances">Utterances (one per line)</Label>
              <Textarea id="route-utterances" name="utterances" rows={4} />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setRouteDialogOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={createRouteMutation.isPending}>
                Create route
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={routeToDelete !== null} onOpenChange={(open) => !open && setRouteToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete semantic route?</AlertDialogTitle>
            <AlertDialogDescription>This removes the route from tool-based semantic routing.</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={handleDeleteRoute}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
