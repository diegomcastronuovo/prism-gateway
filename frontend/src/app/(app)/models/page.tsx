'use client'

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Search, Layers, Plus, ShieldAlert, Code2 } from 'lucide-react'
import { cn } from '@/lib/utils/cn'
import { useModels, useModel, useModelBenchmarks } from '@/features/models/api/use-models'
import { useProviders } from '@/features/providers/api/use-providers'
import { useRouteGroups } from '@/features/route-groups/api/use-route-groups'
import { useTenants } from '@/features/tenants/api/use-tenants'
import { ModelsTable } from '@/features/models/components/models-table'
import { ModelDetailPanel } from '@/features/models/components/model-detail-panel'
import { CreateModelDialog } from '@/features/models/components/create-model-dialog'
import { EditModelDialog } from '@/features/models/components/edit-model-dialog'
import { DeleteModelDialog } from '@/features/models/components/delete-model-dialog'
import { CodingModelEditPanel, type CodingModel } from '@/features/models/components/coding-model-edit-panel'
import { useAuth } from '@/hooks/use-auth'
import { catalogProviderIdsForModels } from '@/features/providers/lib/catalog-provider-ids'

const CODING_FAMILIES: CodingModel['family'][] = ['haiku', 'sonnet', 'opus']

type CodingModelsState =
  | { status: 'loading' }
  | { status: 'licensed'; data: CodingModel[] }
  | { status: 'not_licensed' }
  | { status: 'unavailable'; error?: string }

function ModelsPageContent() {
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedModelId, setSelectedModelId] = useState<string | null>(null)
  const [selectedCodingFamily, setSelectedCodingFamily] = useState<CodingModel['family'] | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [windowHours, setWindowHours] = useState(24)

  const { user } = useAuth()

  const codingModelsQuery = useQuery<CodingModelsState>({
    queryKey: ['claude-code-coding-models'],
    queryFn: async (): Promise<CodingModelsState> => {
      const res = await fetch('/api/providers/claude-code/coding_models')
      if (res.status === 403) return { status: 'not_licensed' }
      if (!res.ok) return { status: 'unavailable', error: `Error ${res.status}` }
      const data = (await res.json()) as CodingModel[]
      return { status: 'licensed', data }
    },
    staleTime: 30_000,
    enabled: !!user,
  })

  const codingModelsState: CodingModelsState = codingModelsQuery.isLoading
    ? { status: 'loading' }
    : (codingModelsQuery.data ?? { status: 'unavailable' })

  const selectedCodingModel: CodingModel | null = useMemo(() => {
    if (!selectedCodingFamily || codingModelsState.status !== 'licensed') return null
    return codingModelsState.data.find((m) => m.family === selectedCodingFamily) ?? null
  }, [selectedCodingFamily, codingModelsState])

  const { data: models, isLoading: modelsLoading, error: modelsError } = useModels()
  const { data: model, isLoading: modelLoading } = useModel(selectedModelId)
  const { data: benchmarks, isLoading: benchmarksLoading, refetch: refetchBenchmarks } = useModelBenchmarks(windowHours)
  const { data: providers } = useProviders()
  const tenantsQuery = useTenants()
  const routeGroupTenantId = tenantsQuery.data?.[0]?.tenant_id ?? ''
  const { data: routeGroups } = useRouteGroups(routeGroupTenantId)

  const providerList = useMemo(() => catalogProviderIdsForModels(providers), [providers])
  const routeGroupList = routeGroups?.map((rg) => rg.id) || []
  const existingModelIds = models?.map((m) => m.id) || []

  const selectedBenchmark = benchmarks?.find((b) => b.model === selectedModelId)

  const handleDeleteSuccess = () => {
    if (selectedModelId) {
      setSelectedModelId(null)
    }
  }

  if (modelsError) {
    return (
      <div>
        <PageHeader
          title="Models"
          description="AI models configured in the gateway"
        />
        <SectionCard title="Error">
          <div className="text-center py-8">
            <p className="text-destructive mb-2">Failed to load models</p>
            <p className="text-sm text-muted-foreground">{modelsError.message}</p>
          </div>
        </SectionCard>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Models"
        description="Manage AI models and view performance. This is a Global configuration screen."
      />

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Left: Model List + Coding Models */}
        <div className="lg:col-span-2 space-y-6">
          <SectionCard
            title="All Models"
            description={models ? `${models.length} models available` : undefined}
            className="border-t-4 border-t-pink-500"
          >
            <div className="space-y-4">
              {/* Search and Create */}
              <div className="flex gap-2">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    placeholder="Search models by name or provider..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    className="pl-9"
                  />
                </div>
                <Button onClick={() => setCreateDialogOpen(true)}>
                  <Plus className="h-4 w-4 mr-1" />
                  Create Model
                </Button>
              </div>

              {/* Models Table */}
              {modelsLoading ? (
                <div className="space-y-2">
                  {[...Array(5)].map((_, i) => (
                    <Skeleton key={i} className="h-12 w-full" />
                  ))}
                </div>
              ) : !models || models.length === 0 ? (
                <EmptyState
                  icon={Layers}
                  title="No models configured"
                  description="Models will appear here once configured in the gateway"
                />
              ) : (
                <ModelsTable
                  models={models}
                  benchmarks={benchmarks}
                  isLoading={modelsLoading || benchmarksLoading}
                  selectedModelId={selectedModelId}
                  onSelectModel={(id) => {
                    setSelectedModelId(id)
                    setSelectedCodingFamily(null)
                  }}
                  searchQuery={searchQuery}
                />
              )}
            </div>
          </SectionCard>

          {/* Coding Models */}
          <SectionCard title="Coding Models" description="Claude Code pricing by family" className="border-t-4 border-t-cyan-400">
            {codingModelsState.status === 'loading' ? (
              <div className="space-y-2">
                {[...Array(3)].map((_, i) => (
                  <Skeleton key={i} className="h-14 w-full" />
                ))}
              </div>
            ) : codingModelsState.status === 'not_licensed' ? (
              <p className="text-sm text-muted-foreground py-2">Claude Code not licensed</p>
            ) : codingModelsState.status === 'unavailable' ? (
              <p className="text-sm text-destructive py-2">
                {codingModelsState.error ?? 'Could not load coding models'}
              </p>
            ) : (
              <div className="space-y-2">
                {CODING_FAMILIES.map((family) => {
                  const m = codingModelsState.data.find((d) => d.family === family)
                  const isSelected = selectedCodingFamily === family
                  return (
                    <button
                      key={family}
                      onClick={() => {
                        setSelectedCodingFamily(family)
                        setSelectedModelId(null)
                      }}
                      className={cn(
                        'w-full text-left p-3 rounded-lg border transition-colors',
                        isSelected
                          ? 'bg-primary text-primary-foreground border-primary'
                          : 'bg-card hover:bg-accent hover:text-accent-foreground'
                      )}
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          <div className={cn(
                            'flex h-8 w-8 items-center justify-center rounded-lg',
                            isSelected ? 'bg-primary-foreground/20' : 'bg-primary/10'
                          )}>
                            <Code2 className={cn(
                              'h-4 w-4',
                              isSelected ? 'text-primary-foreground' : 'text-primary'
                            )} />
                          </div>
                          <span className="font-medium capitalize">{family}</span>
                        </div>
                        {m ? (
                          <div className={cn(
                            'text-xs space-x-2',
                            isSelected ? 'text-primary-foreground/80' : 'text-muted-foreground'
                          )}>
                            <span>in: {m.input_price}</span>
                            <span>out: {m.output_price}</span>
                          </div>
                        ) : (
                          <Badge variant="secondary" className="text-xs">—</Badge>
                        )}
                      </div>
                    </button>
                  )
                })}
              </div>
            )}
          </SectionCard>
        </div>

        {/* Right: Model Detail or Coding Model Edit */}
        <div className="lg:col-span-1 rounded-lg border bg-card border-t-4 border-t-purple-500">
          {selectedCodingFamily ? (
            <CodingModelEditPanel model={selectedCodingModel} />
          ) : (
            <ModelDetailPanel
              model={model}
              benchmark={selectedBenchmark}
              isLoading={modelLoading}
              onEdit={() => setEditDialogOpen(true)}
              onDelete={() => setDeleteDialogOpen(true)}
              windowHours={windowHours}
              onWindowHoursChange={setWindowHours}
              onRefreshBenchmarks={refetchBenchmarks}
            />
          )}
        </div>
      </div>

      {/* Dialogs */}
      <CreateModelDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        providers={providerList}
        routeGroups={routeGroupList}
        existingModelIds={existingModelIds}
      />

      <EditModelDialog
        open={editDialogOpen}
        onOpenChange={setEditDialogOpen}
        model={model || null}
        providers={providerList}
      />

      <DeleteModelDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        model={model || null}
        onSuccess={handleDeleteSuccess}
      />
    </div>
  )
}

export default function ModelsPage() {
  const { user } = useAuth()

  if (user?.role === 'local_admin' || user?.role === 'user' || user?.role === 'finance') {
    return (
      <div>
        <PageHeader
          title="Models"
          description="AI models configured in the gateway"
        />
        <SectionCard title="Access limited">
          <EmptyState
            icon={ShieldAlert}
            title="Insufficient permissions"
            description="Your role cannot access the global Models catalog. You can still assign allowed models from Tenant configuration → Routing."
          />
        </SectionCard>
      </div>
    )
  }

  return <ModelsPageContent />
}
