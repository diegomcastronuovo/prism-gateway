import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/hooks/use-toast'

export interface Model {
  id: string
  provider: string
  route_groups: string[]
  enabled?: boolean
  type?: string
  infrastructure_monthly_usd?: number
  /** Overrides the provider's base URL for this model (e.g. local LLMs on different ports). */
  base_url?: string
  execution?: {
    endpoint?: string
    protocol?: string
  }
  observable?: {
    fields?: Array<{
      path: string
      type: 'text' | 'json' | 'number'
      role: 'input' | 'output'
    }>
  }
  pricing?: {
    prompt_per_1m?: number
    cached_input_per_1m?: number
    completion_per_1m?: number
  }
  /** Percentage added on top of cost (e.g. 20 → +20%). Top-level field, not inside pricing. */
  markup_percentage?: number
  mock?: {
    enabled?: boolean
    delay_min_ms?: number
    delay_max_ms?: number
    error_rate?: number
    error_status?: number
    error_message?: string
    fixed_response?: string
  }
  [key: string]: unknown
}

export interface ModelBenchmark {
  model: string
  provider: string
  avg_latency_ms: number
  p95_latency_ms: number
  success_rate: number
  avg_cost_usd: number
  samples: number
}

// Fetch all models
async function fetchModels(): Promise<Model[]> {
  const response = await fetch('/api/models')
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch models')
  }
  
  const data = await response.json()
  return data.data || []
}

// Fetch single model
async function fetchModel(modelId: string): Promise<Model> {
  const response = await fetch(`/api/models/${modelId}`)
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch model')
  }
  
  const data = await response.json()
  return data.data
}

// Fetch model benchmarks
async function fetchModelBenchmarks(windowHours: number = 24): Promise<ModelBenchmark[]> {
  const response = await fetch(`/api/benchmarks/models?window_hours=${windowHours}`)
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to fetch benchmarks')
  }
  
  const data = await response.json()
  return data.data || []
}

// Create model
async function createModel(model: Model): Promise<{ message: string; model: Model }> {
  const response = await fetch('/api/models', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(model),
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to create model')
  }
  
  return response.json()
}

// Update model
async function updateModel(
  modelId: string,
  model: Partial<Model>,
  version: number
): Promise<{ message: string; model: Model }> {
  const response = await fetch(`/api/models/${modelId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...model, version }),
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to update model')
  }
  
  return response.json()
}

// Delete model
async function deleteModel(modelId: string): Promise<{ message: string }> {
  const response = await fetch(`/api/models/${modelId}`, {
    method: 'DELETE',
  })
  
  if (!response.ok) {
    const error = await response.json()
    throw new Error(error.error || 'Failed to delete model')
  }
  
  // Response might be 204 No Content or JSON
  if (response.status === 204) {
    return { message: 'Model deleted successfully' }
  }
  
  return response.json()
}

// Hook to get all models
export function useModels() {
  return useQuery({
    queryKey: ['models'],
    queryFn: fetchModels,
  })
}

// Hook to get single model
export function useModel(modelId: string | null) {
  return useQuery({
    queryKey: ['model', modelId],
    queryFn: () => fetchModel(modelId!),
    enabled: !!modelId,
  })
}

// Hook to get model benchmarks
export function useModelBenchmarks(windowHours: number = 24) {
  return useQuery({
    queryKey: ['modelBenchmarks', windowHours],
    queryFn: () => fetchModelBenchmarks(windowHours),
  })
}

// Hook to create model
export function useCreateModel() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: createModel,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['models'] })
      queryClient.invalidateQueries({ queryKey: ['globalConfig'] })
      toast({
        title: 'Success',
        description: 'Model created successfully',
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

// Hook to update model
export function useUpdateModel() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: ({ modelId, model, version }: { modelId: string; model: Partial<Model>; version: number }) =>
      updateModel(modelId, model, version),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['models'] })
      queryClient.invalidateQueries({ queryKey: ['model', variables.modelId] })
      queryClient.invalidateQueries({ queryKey: ['globalConfig'] })
      toast({
        title: 'Success',
        description: 'Model updated successfully',
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

// Hook to delete model
export function useDeleteModel() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  
  return useMutation({
    mutationFn: deleteModel,
    onSuccess: (_data, modelId) => {
      // Remove the specific model query from cache to prevent refetch attempts
      queryClient.removeQueries({ queryKey: ['model', modelId] })
      // Invalidate the models list
      queryClient.invalidateQueries({ queryKey: ['models'] })
      queryClient.invalidateQueries({ queryKey: ['globalConfig'] })
      toast({
        title: 'Success',
        description: 'Model deleted successfully',
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
