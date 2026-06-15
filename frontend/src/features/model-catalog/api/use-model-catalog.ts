import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'

export interface ModelCatalogEntry {
  id: string
  provider: string
  display_name: string
  type: string
  prompt_per_1m: number
  cached_input_per_1m: number
  completion_per_1m: number
  infrastructure_monthly_usd: number
  is_active: boolean
  long_context: boolean
  long_context_start_tokens: number
  long_context_prompt_per_1m: number
  long_context_cached_input_per_1m: number
  long_context_completion_per_1m: number
  created_at: string
  updated_at: string
}

export interface ModelCatalogFilters {
  provider?: string
  type?: string
  active?: boolean
}

export interface CreateModelCatalogEntryInput {
  id: string
  provider: string
  display_name: string
  type: string
  prompt_per_1m: number
  cached_input_per_1m?: number
  completion_per_1m: number
  infrastructure_monthly_usd: number
  is_active: boolean
  long_context?: boolean
  long_context_start_tokens?: number
  long_context_prompt_per_1m?: number
  long_context_cached_input_per_1m?: number
  long_context_completion_per_1m?: number
}

export type UpdateModelCatalogEntryInput = Partial<Omit<CreateModelCatalogEntryInput, 'id' | 'provider'>>

// Fetch model catalog entries
async function fetchModelCatalog(filters?: ModelCatalogFilters): Promise<ModelCatalogEntry[]> {
  const params = new URLSearchParams()
  if (filters?.provider) params.set('provider', filters.provider)
  if (filters?.type) params.set('type', filters.type)
  if (filters?.active !== undefined) params.set('active', String(filters.active))

  const query = params.toString()
  const url = query ? `/api/model-catalog?${query}` : '/api/model-catalog'

  const response = await fetch(url)

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch model catalog')
  }

  const data = await response.json()
  return data.data || []
}

// Create model catalog entry
async function createModelCatalogEntry(entry: CreateModelCatalogEntryInput): Promise<ModelCatalogEntry> {
  const response = await fetch('/api/model-catalog', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(entry),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to create model catalog entry')
  }

  return response.json()
}

// Update model catalog entry
async function updateModelCatalogEntry({
  provider,
  id,
  data,
}: {
  provider: string
  id: string
  data: UpdateModelCatalogEntryInput
}): Promise<ModelCatalogEntry> {
  const response = await fetch(`/api/model-catalog/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })

  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update model catalog entry')
  }

  return response.json()
}

// Delete model catalog entry
async function deleteModelCatalogEntry({
  provider,
  id,
}: {
  provider: string
  id: string
}): Promise<void> {
  const response = await fetch(`/api/model-catalog/${encodeURIComponent(provider)}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })

  if (!response.ok && response.status !== 204) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to delete model catalog entry')
  }
}

// Hook to get model catalog entries
export function useModelCatalog(filters?: ModelCatalogFilters, enabled = true) {
  return useQuery({
    queryKey: ['model-catalog', filters],
    queryFn: () => fetchModelCatalog(filters),
    enabled,
  })
}

// Hook to create a model catalog entry
export function useCreateModelCatalogEntry() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: createModelCatalogEntry,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['model-catalog'] })
      toast({
        title: 'Success',
        description: 'Model catalog entry created successfully',
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

// Hook to update a model catalog entry
export function useUpdateModelCatalogEntry() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: updateModelCatalogEntry,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['model-catalog'] })
      toast({
        title: 'Success',
        description: 'Model catalog entry updated successfully',
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

// Hook to delete a model catalog entry
export function useDeleteModelCatalogEntry() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  return useMutation({
    mutationFn: deleteModelCatalogEntry,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['model-catalog'] })
      toast({
        title: 'Success',
        description: 'Model catalog entry deleted successfully',
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
