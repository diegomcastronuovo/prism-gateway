'use client'

import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Boxes, Code2, ShieldAlert } from 'lucide-react'
import { cn } from '@/lib/utils/cn'
import { useAuth } from '@/hooks/use-auth'
import { useGlobalConfig } from '@/features/global-config/api/use-global-config'
import { useUpdateProvider } from '@/features/providers/api/use-providers'
import { getVersionForProviderMutation } from '@/features/providers/api/provider-mutation-version'
import { providersFromGlobalConfig } from '@/features/providers/lib/from-global-config'
import { ProviderList } from '@/features/providers/components/provider-list'
import { ProviderDetailPanel } from '@/features/providers/components/provider-detail-panel'
import { EditProviderDialog } from '@/features/providers/components/edit-provider-dialog'
import {
  ClaudeCodeDetailPanel,
  type ClaudeCodeState,
  type ClaudeCodeProviderData,
} from '@/features/providers/components/claude-code-detail-panel'

type SelectedItem =
  | { kind: 'provider'; id: string }
  | { kind: 'coding_provider' }

function ProvidersContent() {
  const [selected, setSelected] = useState<SelectedItem | null>(null)
  const [editDialogOpen, setEditDialogOpen] = useState(false)

  const selectedProviderId = selected?.kind === 'provider' ? selected.id : null

  const queryClient = useQueryClient()
  const { user, isRefreshingSession } = useAuth()

  /** Single source of truth: same document PATCH uses for provider updates */
  const globalConfigQuery = useGlobalConfig(!!user)
  // React Query can keep previous data on error — never surface stale providers (FinOps-style)
  const globalConfigData = globalConfigQuery.isError ? undefined : globalConfigQuery.data
  const globalConfigLoading = globalConfigQuery.isLoading
  const globalConfigFetching = globalConfigQuery.isFetching
  const globalConfigError = globalConfigQuery.isError ? globalConfigQuery.error : null

  const accessDeniedStatus =
    globalConfigError instanceof Error
      ? (globalConfigError as Error & { status?: number }).status
      : undefined
  const isAccessDenied = accessDeniedStatus === 403 || accessDeniedStatus === 401

  const providers = useMemo(
    () => providersFromGlobalConfig(globalConfigData),
    [globalConfigData]
  )

  const selectedProvider = useMemo(() => {
    if (!selectedProviderId) return undefined
    return providers.find((p) => p.id === selectedProviderId)
  }, [providers, selectedProviderId])

  // Claude Code provider — fetched in parallel, 403 = not licensed (not an error)
  const claudeCodeQuery = useQuery<ClaudeCodeState>({
    queryKey: ['claude-code-provider'],
    queryFn: async (): Promise<ClaudeCodeState> => {
      const res = await fetch('/api/providers/claude-code')
      if (res.status === 403) return { status: 'not_licensed' }
      if (!res.ok) return { status: 'unavailable', error: 'Provider unavailable' }
      const data = (await res.json()) as ClaudeCodeProviderData
      return { status: 'licensed', data }
    },
    staleTime: 30_000,
    enabled: !!user,
  })

  const claudeCodeState: ClaudeCodeState = claudeCodeQuery.isLoading
    ? { status: 'loading' }
    : (claudeCodeQuery.data ?? { status: 'unavailable' })

  const updateProvider = useUpdateProvider()

  const handleToggleEnabled = async (enabled: boolean) => {
    if (!selectedProvider) return

    const version = getVersionForProviderMutation(
      queryClient,
      globalConfigData?.version ?? selectedProvider.version
    )
    await updateProvider.mutateAsync({
      providerId: selectedProvider.id,
      config: { Enabled: enabled },
      version,
    })
  }

  const listLoading = !!user && (globalConfigLoading || isRefreshingSession)
  const detailLoading = !!user && (globalConfigLoading || isRefreshingSession) && !globalConfigData

  if (globalConfigError) {
    return (
      <div>
        <PageHeader
          title="Providers"
          description="AI providers configured in the gateway"
        />
        <SectionCard title={isAccessDenied ? 'Access limited' : 'Error'}>
          {isAccessDenied ? (
            <EmptyState
              icon={ShieldAlert}
              title="Insufficient permissions"
              description="Your role cannot view or edit global provider configuration. This page only shows data when the gateway allows access to global configuration."
            />
          ) : (
            <div className="text-center py-8">
              <p className="text-destructive mb-2">Failed to load global configuration</p>
              <p className="text-sm text-muted-foreground">
                {globalConfigError instanceof Error ? globalConfigError.message : 'Unknown error'}
              </p>
            </div>
          )}
        </SectionCard>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Providers"
        description="Manage AI providers and their credentials. This is a Global configuration screen."
      />

      {globalConfigFetching && globalConfigData ? (
        <p className="text-xs text-muted-foreground mb-2" aria-live="polite">
          Syncing configuration…
        </p>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-1 space-y-6">
          <SectionCard
            title="All Providers"
            description={providers.length ? `${providers.length} providers available` : undefined}
            className="border-t-4 border-t-pink-500"
          >
            {listLoading ? (
              <div className="space-y-2">
                {[...Array(5)].map((_, i) => (
                  <Skeleton key={i} className="h-16 w-full" />
                ))}
              </div>
            ) : !providers.length ? (
              <EmptyState
                icon={Boxes}
                title="No providers configured"
                description="Providers will appear here once configured in the gateway"
              />
            ) : (
              <ProviderList
                providers={providers}
                isLoading={listLoading}
                selectedProviderId={selectedProviderId}
                onSelectProvider={(id) => setSelected({ kind: 'provider', id })}
              />
            )}
          </SectionCard>

          {/* Coding Providers */}
          <SectionCard
            title="Coding Providers"
            description="Licensed coding capabilities"
            className="border-t-4 border-t-cyan-400"
          >
            <button
              onClick={() => setSelected({ kind: 'coding_provider' })}
              className={cn(
                'w-full text-left p-4 rounded-lg border transition-colors',
                selected?.kind === 'coding_provider'
                  ? 'bg-primary text-primary-foreground border-primary'
                  : 'bg-card hover:bg-accent hover:text-accent-foreground'
              )}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className={cn(
                    'flex h-10 w-10 items-center justify-center rounded-lg',
                    selected?.kind === 'coding_provider' ? 'bg-primary-foreground/20' : 'bg-primary/10'
                  )}>
                    <Code2 className={cn(
                      'h-5 w-5',
                      selected?.kind === 'coding_provider' ? 'text-primary-foreground' : 'text-primary'
                    )} />
                  </div>
                  <p className="font-medium">Claude Code</p>
                </div>
                {claudeCodeQuery.isLoading ? (
                  <Badge variant="secondary">Loading…</Badge>
                ) : claudeCodeState.status === 'licensed' ? (
                  <Badge variant="default">Licensed</Badge>
                ) : claudeCodeState.status === 'not_licensed' ? (
                  <Badge variant="destructive">Not Licensed</Badge>
                ) : (
                  <Badge variant="secondary">Unavailable</Badge>
                )}
              </div>
            </button>
          </SectionCard>
        </div>

        <div className="lg:col-span-2">
          {selected?.kind === 'coding_provider' ? (
            <ClaudeCodeDetailPanel state={claudeCodeState} />
          ) : (
            <ProviderDetailPanel
              provider={selectedProvider}
              isLoading={detailLoading}
              onEdit={() => setEditDialogOpen(true)}
              onToggleEnabled={handleToggleEnabled}
            />
          )}
        </div>
      </div>

      <EditProviderDialog
        open={editDialogOpen}
        onOpenChange={setEditDialogOpen}
        provider={selectedProvider ?? null}
      />
    </div>
  )
}

export default function ProvidersPage() {
  return (
    <RequireAdminRole allowedRoles={['admin', 'local_admin', 'user']}>
      <ProvidersContent />
    </RequireAdminRole>
  )
}
