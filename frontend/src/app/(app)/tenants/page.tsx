'use client'

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Plus, Users, Search } from 'lucide-react'
import { useTenants, useTenantConfig, type TenantEnvironment } from '@/features/tenants/api/use-tenants'
import { CreateTenantDialog } from '@/features/tenants/components/create-tenant-dialog'
import { EditBudgetDialog } from '@/features/tenants/components/edit-budget-dialog'
import { EditRoutingDialog } from '@/features/tenants/components/edit-routing-dialog'
import { EditFeaturesDialog } from '@/features/tenants/components/edit-features-dialog'
import { EditPiiDialog } from '@/features/tenants/components/edit-pii-dialog'
import { EditTrafficSplitDialog } from '@/features/tenants/components/edit-traffic-split-dialog'
import { EditRateLimitDialog } from '@/features/tenants/components/edit-rate-limit-dialog'
import { EditOutputLimitDialog } from '@/features/tenants/components/edit-output-limit-dialog'
import { DeleteTenantDialog } from '@/features/tenants/components/delete-tenant-dialog'
import { TenantConfigDetail } from '@/features/tenants/components/tenant-config-detail'
import { cn } from '@/lib/utils/cn'

function TenantsContent() {
  const [selectedTenantId, setSelectedTenantId] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [environmentFilter, setEnvironmentFilter] = useState<'All' | TenantEnvironment>('All')
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [editBudgetDialogOpen, setEditBudgetDialogOpen] = useState(false)
  const [editRoutingDialogOpen, setEditRoutingDialogOpen] = useState(false)
  const [editFeaturesDialogOpen, setEditFeaturesDialogOpen] = useState(false)
  const [editPiiDialogOpen, setEditPiiDialogOpen] = useState(false)
  const [editTrafficSplitDialogOpen, setEditTrafficSplitDialogOpen] = useState(false)
  const [editRateLimitDialogOpen, setEditRateLimitDialogOpen] = useState(false)
  const [editOutputLimitDialogOpen, setEditOutputLimitDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  const { data: tenants, isLoading: tenantsLoading } = useTenants()
  const { data: tenantConfig, isLoading: configLoading } = useTenantConfig(selectedTenantId)

  const tenantIds = useMemo(() => (tenants ?? []).map((t) => t.tenant_id), [tenants])
  const environmentsQuery = useQuery({
    queryKey: ['tenantEnvironments', tenantIds.join(',')],
    enabled: tenantIds.length > 0,
    queryFn: async () => {
      const entries = await Promise.all(
        tenantIds.map(async (tenantId) => {
          try {
            const res = await fetch(`/api/tenants/${encodeURIComponent(tenantId)}`)
            if (!res.ok) return [tenantId, undefined] as const
            const data = await res.json()
            const env = (data.environment ?? data.config?.environment) as string | undefined
            return [tenantId, env] as const
          } catch {
            return [tenantId, undefined] as const
          }
        })
      )
      return new Map(entries)
    },
  })

  const normalizeEnvironment = (value?: string) => {
    const upper = value?.toUpperCase()
    return upper === 'DEV' || upper === 'STAGING' || upper === 'PROD' ? (upper as TenantEnvironment) : undefined
  }

  const resolveEnvironment = (tenant: { environment?: TenantEnvironment }) => {
    const record = tenant as unknown as Record<string, unknown>
    const configEnv = (record.config as Record<string, unknown> | undefined)?.environment
    const listEnv = tenant.environment ?? (typeof configEnv === 'string' ? configEnv : undefined)
    const envFromConfig = environmentsQuery.data?.get((tenant as { tenant_id: string }).tenant_id)
    const env = listEnv ?? envFromConfig
    return normalizeEnvironment(typeof env === 'string' ? env : undefined)
  }

  const filteredTenants = tenants
    ?.filter((tenant) => {
      const env = resolveEnvironment(tenant)
      return environmentFilter === 'All' ? true : env === environmentFilter
    })
    .filter((tenant) => tenant.tenant_id.toLowerCase().includes(searchQuery.toLowerCase()))

  const getEnvironmentClass = (env?: TenantEnvironment) => {
    switch (env) {
      case 'PROD':
        return 'bg-red-50 border-red-100'
      case 'STAGING':
        return 'bg-blue-50 border-blue-100'
      case 'DEV':
        return 'bg-green-50 border-green-100'
      default:
        return ''
    }
  }

  const handleCreateSuccess = (tenantId: string) => {
    setSelectedTenantId(tenantId)
  }

  const handleDeleteSuccess = () => {
    setSelectedTenantId(null)
  }

  return (
    <div>
      <PageHeader
        title="Tenants"
        description="Manage tenant configurations and access"
        action={
          <Button onClick={() => setCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Create Tenant
          </Button>
        }
      />

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Left: Tenant List */}
        <div className="lg:col-span-1">
          <SectionCard
            title="All Tenants"
            className="border-t-4 border-t-pink-500"
            action={
              <Select value={environmentFilter} onValueChange={(value) => setEnvironmentFilter(value as 'All' | TenantEnvironment)}>
                <SelectTrigger className="w-[140px]">
                  <SelectValue placeholder="Environment" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="All">All</SelectItem>
                  <SelectItem value="PROD">PROD</SelectItem>
                  <SelectItem value="STAGING">STAGING</SelectItem>
                  <SelectItem value="DEV">DEV</SelectItem>
                </SelectContent>
              </Select>
            }
          >
            <div className="space-y-4">
              {/* Search */}
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search tenants..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-9"
                />
              </div>

              {/* Tenant List */}
              {tenantsLoading ? (
                <div className="space-y-2">
                  {[...Array(5)].map((_, i) => (
                    <Skeleton key={i} className="h-12 w-full" />
                  ))}
                </div>
              ) : !filteredTenants || filteredTenants.length === 0 ? (
                <EmptyState
                  icon={Users}
                  title={searchQuery ? 'No tenants found' : 'No tenants configured'}
                  description={
                    searchQuery
                      ? 'Try a different search term'
                      : 'Create your first tenant to get started'
                  }
                  action={
                    !searchQuery ? (
                      <Button onClick={() => setCreateDialogOpen(true)}>
                        <Plus className="mr-2 h-4 w-4" />
                        Create Tenant
                      </Button>
                    ) : undefined
                  }
                />
              ) : (
                <div className="space-y-1">
                  {filteredTenants.map((tenant) => {
                    const env = resolveEnvironment(tenant)
                    return (
                      <button
                        key={tenant.tenant_id}
                        onClick={() => setSelectedTenantId(tenant.tenant_id)}
                        className={cn(
                          'w-full text-left px-4 py-3 rounded-lg border transition-colors',
                          getEnvironmentClass(env),
                          selectedTenantId === tenant.tenant_id
                            ? 'ring-2 ring-primary/40 border-primary'
                            : 'hover:opacity-90'
                        )}
                      >
                        <div className={cn('font-medium', env ? 'text-slate-900' : 'text-foreground')}>
                          {tenant.tenant_id}
                        </div>
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          </SectionCard>
        </div>

        {/* Right: Tenant Detail */}
        <div className="lg:col-span-2">
          <TenantConfigDetail
            config={tenantConfig}
            isLoading={configLoading}
            onEditBudget={() => setEditBudgetDialogOpen(true)}
            onEditRouting={() => setEditRoutingDialogOpen(true)}
            onEditFeatures={() => setEditFeaturesDialogOpen(true)}
            onEditSecurity={() => setEditPiiDialogOpen(true)}
            onEditTrafficSplit={() => setEditTrafficSplitDialogOpen(true)}
            onEditRateLimit={() => setEditRateLimitDialogOpen(true)}
            onEditOutputLimit={() => setEditOutputLimitDialogOpen(true)}
            onDelete={() => setDeleteDialogOpen(true)}
          />
        </div>
      </div>

      {/* Dialogs */}
      <CreateTenantDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        onSuccess={handleCreateSuccess}
      />

      <EditBudgetDialog
        open={editBudgetDialogOpen}
        onOpenChange={setEditBudgetDialogOpen}
        tenantConfig={tenantConfig || null}
      />

      <EditRoutingDialog
        open={editRoutingDialogOpen}
        onOpenChange={setEditRoutingDialogOpen}
        tenantConfig={
          tenantConfig
            ? {
                ...tenantConfig,
                environment: normalizeEnvironment(
                  tenantConfig.environment ??
                  (tenantConfig.config as Record<string, unknown>)?.environment as string | undefined ??
                  tenants?.find((t) => t.tenant_id === tenantConfig.tenant_id)?.environment ??
                  environmentsQuery.data?.get(tenantConfig.tenant_id)
                ),
              }
            : null
        }
      />

      <EditFeaturesDialog
        open={editFeaturesDialogOpen}
        onOpenChange={setEditFeaturesDialogOpen}
        tenantConfig={tenantConfig || null}
      />

      <EditPiiDialog
        open={editPiiDialogOpen}
        onOpenChange={setEditPiiDialogOpen}
        tenantConfig={tenantConfig || null}
      />

      <EditTrafficSplitDialog
        open={editTrafficSplitDialogOpen}
        onOpenChange={setEditTrafficSplitDialogOpen}
        tenantConfig={tenantConfig || null}
      />

      <EditRateLimitDialog
        open={editRateLimitDialogOpen}
        onOpenChange={setEditRateLimitDialogOpen}
        tenantConfig={tenantConfig || null}
      />

      <EditOutputLimitDialog
        open={editOutputLimitDialogOpen}
        onOpenChange={setEditOutputLimitDialogOpen}
        tenantConfig={tenantConfig || null}
      />

      <DeleteTenantDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        tenantId={selectedTenantId}
        onSuccess={handleDeleteSuccess}
      />
    </div>
  )
}

export default function TenantsPage() {
  return (
    <RequireAdminRole allowedRoles={['admin', 'local_admin', 'user']}>
      <TenantsContent />
    </RequireAdminRole>
  )
}
