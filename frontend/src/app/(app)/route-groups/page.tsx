'use client'

import { useEffect, useState } from 'react'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { GitBranch, Plus, Search } from 'lucide-react'
import {
  useRouteGroups,
  useRouteGroup,
} from '@/features/route-groups/api/use-route-groups'
import { useModels } from '@/features/models/api/use-models'
import { useTenants } from '@/features/tenants/api/use-tenants'
import { RouteGroupsTable } from '@/features/route-groups/components/route-groups-table'
import { RouteGroupDetailPanel } from '@/features/route-groups/components/route-group-detail-panel'
import { CreateRouteGroupDialog } from '@/features/route-groups/components/create-route-group-dialog'
import { EditRouteGroupDialog } from '@/features/route-groups/components/edit-route-group-dialog'
import { DeleteRouteGroupDialog } from '@/features/route-groups/components/delete-route-group-dialog'

function RouteGroupsContent() {
  const tenantsQuery = useTenants()
  const tenants = tenantsQuery.data ?? []
  const [selectedTenantId, setSelectedTenantId] = useState<string | null>(null)

  const [searchQuery, setSearchQuery] = useState('')
  const [selectedRouteGroupId, setSelectedRouteGroupId] = useState<string | null>(null)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  useEffect(() => {
    if (!selectedTenantId && tenantsQuery.data?.length) {
      setSelectedTenantId(tenantsQuery.data[0].tenant_id)
    }
  }, [selectedTenantId, tenantsQuery.data])

  useEffect(() => {
    setSelectedRouteGroupId(null)
  }, [selectedTenantId])

  const tenantId = selectedTenantId ?? ''

  const { data: routeGroups, isLoading: routeGroupsLoading, error: routeGroupsError } =
    useRouteGroups(tenantId)
  const { data: selectedRouteGroup, isLoading: routeGroupLoading } = useRouteGroup(
    tenantId,
    selectedRouteGroupId
  )
  const { data: models } = useModels()

  // Filter route groups based on search query
  const filteredRouteGroups = routeGroups?.filter((group) =>
    group.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
    (group.models || []).some((model) =>
      model.toLowerCase().includes(searchQuery.toLowerCase())
    )
  )

  // Get all model IDs for the create/edit dialogs
  const allModelIds = models?.map((model) => model.id) || []

  const tenantSelector = (
    <div className="flex w-full max-w-full flex-col gap-3 sm:w-auto sm:max-w-none sm:flex-row sm:items-end sm:justify-end sm:gap-3">
      <div className="w-full min-w-0 sm:w-60">
        <Label htmlFor="route-groups-tenant" className="text-xs text-muted-foreground">
          Tenant
        </Label>
        <Select
          value={selectedTenantId ?? ''}
          onValueChange={(value) => setSelectedTenantId(value)}
        >
          <SelectTrigger id="route-groups-tenant" className="mt-1">
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
      </div>
    </div>
  )

  if (routeGroupsError) {
    return (
      <div>
        <PageHeader
          title="Route Groups"
          description="Routing groups configured in the gateway. This configuration is tenant-specific."
          action={tenantSelector}
        />
        <SectionCard title="Error">
          <div className="text-center py-8">
            <p className="text-destructive mb-2">Failed to load route groups</p>
            <p className="text-sm text-muted-foreground">{routeGroupsError.message}</p>
          </div>
        </SectionCard>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Route Groups"
        description="Routing groups configured in the gateway. This configuration is tenant-specific."
        action={tenantSelector}
      />

      {!selectedTenantId ? (
        <SectionCard title="Route Groups" className="border-t-4 border-t-pink-500">
          <EmptyState
            icon={GitBranch}
            title="Select a tenant"
            description="Choose a tenant to view and manage route groups."
          />
        </SectionCard>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Left side - Route Groups List */}
          <SectionCard
            title="All Route Groups"
            description={routeGroups ? `${routeGroups.length} route groups configured` : undefined}
            className="border-t-4 border-t-pink-500"
          >
            <div className="space-y-4">
              {/* Search and Create */}
              <div className="flex items-center gap-2">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Search route groups..."
                    className="pl-9"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                  />
                </div>
                <Button
                  onClick={() => setCreateDialogOpen(true)}
                  disabled={!selectedTenantId}
                >
                  <Plus className="h-4 w-4 mr-1" />
                  Create
                </Button>
              </div>

              {/* Route Groups Table */}
              {routeGroupsLoading ? (
                <div className="space-y-2">
                  {[...Array(5)].map((_, i) => (
                    <Skeleton key={i} className="h-16 w-full" />
                  ))}
                </div>
              ) : !filteredRouteGroups || filteredRouteGroups.length === 0 ? (
                <EmptyState
                  icon={GitBranch}
                  title="No route groups found"
                  description={
                    searchQuery
                      ? 'No route groups match your search'
                      : 'Route groups will appear here once configured in the gateway'
                  }
                />
              ) : (
                <RouteGroupsTable
                  routeGroups={filteredRouteGroups}
                  selectedRouteGroupId={selectedRouteGroupId}
                  onSelectRouteGroup={setSelectedRouteGroupId}
                />
              )}
            </div>
          </SectionCard>

          {/* Right side - Route Group Details */}
          <RouteGroupDetailPanel
            routeGroup={selectedRouteGroup}
            isLoading={routeGroupLoading}
            onEdit={() => setEditDialogOpen(true)}
            onDelete={() => setDeleteDialogOpen(true)}
          />
        </div>
      )}

      {/* Dialogs */}
      <CreateRouteGroupDialog
        open={createDialogOpen && !!selectedTenantId}
        onOpenChange={setCreateDialogOpen}
        models={allModelIds}
        tenantId={tenantId}
      />

      <EditRouteGroupDialog
        open={editDialogOpen && !!selectedTenantId}
        onOpenChange={setEditDialogOpen}
        routeGroup={selectedRouteGroup || null}
        models={allModelIds}
        tenantId={tenantId}
      />

      <DeleteRouteGroupDialog
        open={deleteDialogOpen && !!selectedTenantId}
        onOpenChange={setDeleteDialogOpen}
        routeGroup={selectedRouteGroup || null}
        tenantId={tenantId}
      />
    </div>
  )
}

export default function RouteGroupsPage() {
  return (
    <RequireAdminRole>
      <RouteGroupsContent />
    </RequireAdminRole>
  )
}
