'use client'

import { useState, type ComponentProps } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard as BaseSectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils/cn'
import { GitCompare, AlertCircle, CheckCircle, Clock, History as HistoryIcon, ShieldAlert } from 'lucide-react'
import { ConfigDiffView } from '@/features/config/components/config-diff-view'
import { useTenants } from '@/features/tenants/api/use-tenants'
import { useToast } from '@/hooks/use-toast'
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

const SectionCard = (props: ComponentProps<typeof BaseSectionCard>) => (
  <BaseSectionCard
    {...props}
    className={cn('border-t-4 border-t-indigo-500', props.className)}
  />
)

type ConfigHistoryEntry = {
  scope: string
  tenant_id: string
  changed_at: string
  changed_by: string
  change_type: string
  from_version: number
  to_version: number
  is_rollback: boolean
}

type ConfigHistoryResponse = {
  object: string
  data: ConfigHistoryEntry[]
  pagination: {
    limit: number
    offset: number
    returned: number
  }
}

type ConfigVersion = {
  version: number
  created_at: string
  is_current: boolean
}

type ConfigVersionsResponse = {
  object: string
  scope: string
  tenant_id: string
  current_version: number
  data: ConfigVersion[]
  pagination: {
    limit: number
    offset: number
    returned: number
  }
}

async function fetchConfigHistory(scope?: string, tenantId?: string): Promise<ConfigHistoryResponse> {
  const params = new URLSearchParams()
  if (scope) params.set('scope', scope)
  if (tenantId) params.set('tenant_id', tenantId)
  params.set('limit', '50')

  const url = `/api/admin/config/history?${params.toString()}`
  if (process.env.NODE_ENV !== 'production') {
    console.log('[fetchConfigHistory] URL:', url)
  }

  const resp = await fetch(url, { credentials: 'include', cache: 'no-store' })
  if (!resp.ok) {
    const errorBody = await resp.json().catch(() => ({}))
    if (process.env.NODE_ENV !== 'production') {
      console.error('[fetchConfigHistory] Error:', resp.status, errorBody)
    }
    const error = new Error(errorBody?.error?.message || `Failed to fetch config history: ${resp.status}`) as Error & { status?: number; errorType?: string }
    error.status = resp.status
    error.errorType = errorBody?.error?.type
    throw error
  }
  return resp.json()
}

async function fetchConfigVersions(scope: string, tenantId?: string): Promise<ConfigVersionsResponse> {
  const params = new URLSearchParams()
  params.set('scope', scope)
  if (tenantId) params.set('tenant_id', tenantId)
  params.set('limit', '50')

  const url = `/api/admin/config/versions?${params.toString()}`
  if (process.env.NODE_ENV !== 'production') {
    console.log('[fetchConfigVersions] URL:', url)
  }

  const resp = await fetch(url, { credentials: 'include', cache: 'no-store' })
  if (!resp.ok) {
    const errorBody = await resp.json().catch(() => ({}))
    if (process.env.NODE_ENV !== 'production') {
      console.error('[fetchConfigVersions] Error:', resp.status, errorBody)
    }
    const error = new Error(errorBody?.error?.message || `Failed to fetch config versions: ${resp.status}`) as Error & { status?: number; errorType?: string }
    error.status = resp.status
    error.errorType = errorBody?.error?.type
    throw error
  }
  return resp.json()
}

function ConfigHistoryContent() {
  const [viewMode, setViewMode] = useState<'history' | 'diff'>('history')
  const [scope, setScope] = useState<'global' | 'tenant'>('global')
  const [tenantId, setTenantId] = useState('')
  
  const { data: tenants, isLoading: isLoadingTenants } = useTenants()
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const [applyDialogOpen, setApplyDialogOpen] = useState(false)
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null)

  const { data: historyData, isLoading: isLoadingHistory, error: historyError } = useQuery({
    queryKey: ['config-history', { scope, tenantId }],
    queryFn: () => fetchConfigHistory(scope, scope === 'tenant' ? tenantId : undefined),
    enabled: viewMode === 'history' && (scope === 'global' || !!tenantId),
    retry: false,
  })

  const { data: versionsData, isLoading: isLoadingVersions, error: versionsError } = useQuery({
    queryKey: ['config-versions', { scope, tenantId }],
    queryFn: () => fetchConfigVersions(scope, scope === 'tenant' ? tenantId : undefined),
    enabled: viewMode === 'history' && (scope === 'global' || !!tenantId),
    retry: false,
  })

  const applyMutation = useMutation({
    mutationFn: async (version: number) => {
      const res = await fetch('/api/admin/config/global/apply', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version }),
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        throw new Error(err?.error || `Failed to apply version (HTTP ${res.status})`)
      }
      return res.json()
    },
    onSuccess: async () => {
      setApplyDialogOpen(false)
      setSelectedVersion(null)
      toast({ title: 'Configuration version applied successfully.' })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['config-versions'] }),
        queryClient.invalidateQueries({ queryKey: ['config-history'] }),
      ])
    },
    onError: (error: Error) => {
      toast({ title: 'Failed to apply configuration version.', description: error.message, variant: 'destructive' })
    },
  })

  if (viewMode === 'diff') {
    return <ConfigDiffView onBack={() => setViewMode('history')} />
  }

  const showTenantSelector = scope === 'tenant'
  const canShowData = scope === 'global' || (scope === 'tenant' && tenantId)

  const isGlobalDenied =
    scope === 'global' &&
    (historyError || versionsError) &&
    ((historyError as Error & { status?: number })?.status === 400 ||
      (historyError as Error & { status?: number })?.status === 403 ||
      (versionsError as Error & { status?: number })?.status === 400 ||
      (versionsError as Error & { status?: number })?.status === 403)

  // Backend currently returns 403 for /admin/config/versions even for tenant scope when user is local_admin
  // This is a backend limitation - treat it as "versions not available for this tenant"
  const isTenantVersionsUnavailable =
    scope === 'tenant' &&
    versionsError &&
    ((versionsError as Error & { status?: number })?.status === 403)

  return (
    <div>
      <PageHeader
        title="Configuration History"
        description="Version timeline and change history"
        action={
          <Button onClick={() => setViewMode('diff')}>
            <GitCompare className="mr-2 h-4 w-4" />
            Diff Viewer
          </Button>
        }
      />

      {/* Scope Selector */}
      <SectionCard title="Scope">
        <div className="flex gap-4 items-end">
          <div className="flex gap-2">
            <Button
              variant={scope === 'global' ? 'default' : 'outline'}
              onClick={() => setScope('global')}
            >
              Global
            </Button>
            <Button
              variant={scope === 'tenant' ? 'default' : 'outline'}
              onClick={() => setScope('tenant')}
            >
              Tenant
            </Button>
          </div>

          {showTenantSelector && (
            <div className="flex-1 max-w-xs">
              {isLoadingTenants ? (
                <Skeleton className="h-10 w-full" />
              ) : (
                <select
                  value={tenantId}
                  onChange={(e) => setTenantId(e.target.value)}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="">Select a tenant</option>
                  {tenants?.map((tenant) => (
                    <option key={tenant.tenant_id} value={tenant.tenant_id}>
                      {tenant.tenant_id}
                    </option>
                  ))}
                </select>
              )}
            </div>
          )}
        </div>
      </SectionCard>

      {!canShowData && scope === 'tenant' ? (
        <SectionCard title="Tenant Required">
          <div className="text-sm text-muted-foreground">
            Select a tenant to view tenant configuration history.
          </div>
        </SectionCard>
      ) : isGlobalDenied ? (
        <SectionCard title="Access limited">
          <EmptyState
            icon={ShieldAlert}
            title="Insufficient permissions"
            description="Your role cannot view global configuration history. Global scope is not available for your role."
          />
        </SectionCard>
      ) : (
        <>
          {/* Current Version Card */}
          {isLoadingVersions ? (
            <Skeleton className="h-32" />
          ) : isTenantVersionsUnavailable ? (
            null
          ) : versionsError && !isGlobalDenied ? (
            <SectionCard title="Error">
              <div className="flex items-center gap-2 text-sm text-destructive">
                <AlertCircle className="h-4 w-4" />
                Failed to load configuration versions
              </div>
            </SectionCard>
          ) : versionsData ? (
            <div className="grid gap-4 md:grid-cols-3 mb-6">
              <StatCard
                title="Current Version"
                value={`v${versionsData.current_version}`}
                icon={CheckCircle}
                description={`${scope === 'global' ? 'Global' : 'Tenant'} config`}
              />
              <StatCard
                title="Scope"
                value={scope === 'global' ? 'Global' : 'Tenant'}
                icon={HistoryIcon}
                description={scope === 'tenant' && tenantId ? tenantId : '—'}
              />
              <StatCard
                title="Total Versions"
                value={versionsData.data.length.toString()}
                icon={Clock}
                description="Tracked versions"
              />
            </div>
          ) : null}

          {/* Version Timeline */}
          {!isTenantVersionsUnavailable && (
          <SectionCard title="Version Timeline">
            {isLoadingVersions ? (
              <Skeleton className="h-64" />
            ) : versionsError && !isGlobalDenied ? (
              <div className="text-sm text-muted-foreground">Failed to load versions</div>
            ) : !versionsData || versionsData.data.length === 0 ? (
              <div className="text-sm text-muted-foreground">No configuration versions found.</div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Version</TableHead>
                    <TableHead>Created At</TableHead>
                    <TableHead>Current</TableHead>
                    {scope === 'global' && <TableHead>Actions</TableHead>}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {versionsData.data.map((version) => (
                    <TableRow key={version.version}>
                      <TableCell className="font-medium">v{version.version}</TableCell>
                      <TableCell>{new Date(version.created_at).toLocaleString()}</TableCell>
                      <TableCell>
                        {version.is_current ? (
                          <Badge variant="default">Yes</Badge>
                        ) : (
                          <span className="text-muted-foreground text-sm">No</span>
                        )}
                      </TableCell>
                      {scope === 'global' && (
                        <TableCell>
                          {version.is_current ? (
                            <span className="text-muted-foreground">—</span>
                          ) : (
                            <Button
                              size="sm"
                              onClick={() => {
                                setSelectedVersion(version.version)
                                setApplyDialogOpen(true)
                              }}
                            >
                              Apply
                            </Button>
                          )}
                        </TableCell>
                      )}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </SectionCard>
          )}

          {/* Change History Table */}
          <SectionCard title="Change History">
            {isLoadingHistory ? (
              <Skeleton className="h-64" />
            ) : historyError && !isGlobalDenied ? (
              <div className="text-sm text-muted-foreground">Failed to load configuration history</div>
            ) : !historyData || historyData.data.length === 0 ? (
              <div className="text-sm text-muted-foreground">No configuration history found.</div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Changed At</TableHead>
                      <TableHead>Scope</TableHead>
                      <TableHead>Tenant</TableHead>
                      <TableHead>Changed By</TableHead>
                      <TableHead>Type</TableHead>
                      <TableHead className="text-right">From</TableHead>
                      <TableHead className="text-right">To</TableHead>
                      <TableHead>Rollback</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {historyData.data.map((entry, idx) => (
                      <TableRow key={idx}>
                        <TableCell>{new Date(entry.changed_at).toLocaleString()}</TableCell>
                        <TableCell className="capitalize">{entry.scope}</TableCell>
                        <TableCell>{entry.tenant_id || '—'}</TableCell>
                        <TableCell className="font-mono text-xs">{entry.changed_by}</TableCell>
                        <TableCell>
                          <Badge variant="secondary">{entry.change_type}</Badge>
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {entry.from_version === 0 ? 'initial' : `v${entry.from_version}`}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {entry.to_version === 0 ? 'initial' : `v${entry.to_version}`}
                        </TableCell>
                        <TableCell>
                          {entry.is_rollback ? (
                            <Badge variant="destructive">Yes</Badge>
                          ) : (
                            <span className="text-muted-foreground text-sm">No</span>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </SectionCard>
        </>
      )}

      <AlertDialog open={applyDialogOpen} onOpenChange={setApplyDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {`Apply configuration version v${selectedVersion ?? ''}?`}
            </AlertDialogTitle>
            <AlertDialogDescription>
              This will immediately replace the active global configuration.
              The selected version will be applied instantly without restarting the gateway.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={applyMutation.isPending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => selectedVersion != null && applyMutation.mutate(selectedVersion)}
              disabled={applyMutation.isPending}
            >
              {applyMutation.isPending ? 'Applying...' : 'Apply Version'}
            </AlertDialogAction>
          </AlertDialogFooter>
            </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

export default function ConfigHistoryPage() {
  return (
    <RequireAdminRole allowedRoles={['admin', 'local_admin', 'user']}>
      <ConfigHistoryContent />
    </RequireAdminRole>
  )
}
