import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'

export interface ToolCatalogEntry {
  id: string
  provider: string
  display_name: string
  tool_type: string
  unit: string
  price_per_unit: number
  is_active: boolean
}

export interface CreateToolCatalogEntryInput {
  id: string
  provider: string
  display_name: string
  tool_type: string
  unit: string
  price_per_unit: number
  is_active: boolean
}

export type UpdateToolCatalogEntryInput = Partial<Omit<CreateToolCatalogEntryInput, 'id' | 'provider'>>

// Fetch tool catalog entries
async function fetchToolCatalog(provider?: string): Promise<ToolCatalogEntry[]> {
  const params = new URLSearchParams()
  if (provider) params.set('provider', provider)

  const query = params.toString()
  const url = query ? `/api/tool-catalog?${query}` : '/api/tool-catalog'

  const response = await fetch(url)

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch tool catalog')
  }

  const data = await response.json()
  return data.data || []
}

// Create tool catalog entry
async function createToolCatalogEntry(entry: CreateToolCatalogEntryInput): Promise<ToolCatalogEntry> {
  const response = await fetch('/api/tool-catalog', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(entry),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to create tool catalog entry')
  }

  return response.json()
}

// Update tool catalog entry
async function updateToolCatalogEntry({
  provider,
  id,
  data,
}: {
  provider: string
  id: string
  data: UpdateToolCatalogEntryInput
}): Promise<ToolCatalogEntry> {
  const response = await fetch(`/api/tool-catalog/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update tool catalog entry')
  }

  return response.json()
}

// Delete tool catalog entry
async function deleteToolCatalogEntry({
  provider,
  id,
}: {
  provider: string
  id: string
}): Promise<void> {
  const response = await fetch(`/api/tool-catalog/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })

  if (!response.ok && response.status !== 204) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to delete tool catalog entry')
  }
}

// Hook to get tool catalog entries
export function useToolCatalog(provider?: string, enabled = true) {
  return useQuery({
    queryKey: ['tool-catalog', provider],
    queryFn: () => fetchToolCatalog(provider),
    enabled,
  })
}

// Hook to create a tool catalog entry
export function useCreateToolCatalogEntry() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: createToolCatalogEntry,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tool-catalog'] })
      toast({
        title: 'Success',
        description: 'Tool catalog entry created successfully',
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

// Hook to update a tool catalog entry
export function useUpdateToolCatalogEntry() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: updateToolCatalogEntry,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tool-catalog'] })
      toast({
        title: 'Success',
        description: 'Tool catalog entry updated successfully',
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

// Hook to delete a tool catalog entry
export function useDeleteToolCatalogEntry() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: deleteToolCatalogEntry,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tool-catalog'] })
      toast({
        title: 'Success',
        description: 'Tool catalog entry deleted successfully',
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
