import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'

export interface RouteGroup {
  id: string
  models?: string[]
  version?: number
}

// Fetch all route groups
async function fetchRouteGroups(tenantId: string): Promise<RouteGroup[]> {
  const response = await fetch(
    `/api/route-groups?tenantId=${encodeURIComponent(tenantId)}`
  )

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch route groups')
  }

  const data = await response.json()
  return data.data || []
}

// Fetch single route group
async function fetchRouteGroup(
  tenantId: string,
  routeGroupId: string
): Promise<RouteGroup> {
  const response = await fetch(
    `/api/route-groups/${encodeURIComponent(routeGroupId)}?tenantId=${encodeURIComponent(tenantId)}`
  )

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch route group')
  }

  const data = await response.json()
  return data.data
}

// Create route group
async function createRouteGroup(
  tenantId: string,
  routeGroup: {
    id: string
    models: string[]
  }
): Promise<{ message: string; routeGroup: RouteGroup }> {
  const response = await fetch('/api/route-groups', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...routeGroup, tenantId }),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to create route group')
  }

  return response.json()
}

// Update route group
async function updateRouteGroup(
  tenantId: string,
  routeGroupId: string,
  routeGroup: { models: string[] },
  version: number
): Promise<{ message: string; routeGroup: RouteGroup }> {
  const response = await fetch(
    `/api/route-groups/${encodeURIComponent(routeGroupId)}?tenantId=${encodeURIComponent(tenantId)}`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...routeGroup, version }),
    }
  )

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update route group')
  }

  return response.json()
}

// Delete route group
async function deleteRouteGroup(
  tenantId: string,
  routeGroupId: string
): Promise<{ message: string }> {
  const response = await fetch(
    `/api/route-groups/${encodeURIComponent(routeGroupId)}?tenantId=${encodeURIComponent(tenantId)}`,
    {
      method: 'DELETE',
    }
  )

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to delete route group')
  }

  if (response.status === 204) {
    return { message: 'Route group deleted successfully' }
  }

  return response.json()
}

// Hook to fetch all route groups
export function useRouteGroups(tenantId: string) {
  return useQuery({
    queryKey: ['routeGroups', tenantId],
    queryFn: () => fetchRouteGroups(tenantId),
    enabled: !!tenantId,
  })
}

// Hook to fetch single route group
export function useRouteGroup(tenantId: string, routeGroupId: string | null) {
  return useQuery({
    queryKey: ['routeGroup', tenantId, routeGroupId],
    queryFn: () => fetchRouteGroup(tenantId, routeGroupId!),
    enabled: !!tenantId && !!routeGroupId,
  })
}

// Hook to create route group
export function useCreateRouteGroup(tenantId: string) {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: (vars: { id: string; models: string[] }) =>
      createRouteGroup(tenantId, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['routeGroups', tenantId] })
      toast({
        title: 'Success',
        description: 'Route group created successfully',
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

// Hook to update route group
export function useUpdateRouteGroup(tenantId: string) {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: ({
      routeGroupId,
      routeGroup,
      version,
    }: {
      routeGroupId: string
      routeGroup: { models: string[] }
      version: number
    }) => updateRouteGroup(tenantId, routeGroupId, routeGroup, version),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['routeGroups', tenantId] })
      queryClient.invalidateQueries({
        queryKey: ['routeGroup', tenantId, variables.routeGroupId],
      })
      toast({
        title: 'Success',
        description: 'Route group updated successfully',
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}

// Hook to delete route group
export function useDeleteRouteGroup(tenantId: string) {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: (routeGroupId: string) =>
      deleteRouteGroup(tenantId, routeGroupId),
    onSuccess: (_data, routeGroupId) => {
      queryClient.removeQueries({
        queryKey: ['routeGroup', tenantId, routeGroupId],
      })
      queryClient.invalidateQueries({ queryKey: ['routeGroups', tenantId] })
      toast({
        title: 'Success',
        description: 'Route group deleted successfully',
      })
    },
    onError: (error: Error) => {
      toast({
        title: 'Error',
        description: error.message,
        variant: 'destructive',
      })
    },
  })
}
