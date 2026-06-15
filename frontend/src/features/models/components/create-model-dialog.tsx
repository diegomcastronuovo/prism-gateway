'use client'

import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  ObservableFieldsEditor,
  type ObservableFieldForm,
} from './observable-fields-editor'
import { Plus } from 'lucide-react'
import { useCreateModel, type Model } from '../api/use-models'
import { useModelCatalog } from '@/features/model-catalog/api/use-model-catalog'
import { ProviderIcon, providerLabel } from '@/features/providers/components/provider-icon'

interface CreateModelDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  providers: string[]
  routeGroups: string[]
  existingModelIds: string[]
}

export function CreateModelDialog({
  open,
  onOpenChange,
  providers,
  routeGroups,
  existingModelIds,
}: CreateModelDialogProps) {
  const createModel = useCreateModel()
  const [modelId, setModelId] = useState('')
  const [provider, setProvider] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [selectedRouteGroups, setSelectedRouteGroups] = useState<string[]>([])
  const [typeSelection, setTypeSelection] = useState('')
  const [infraMonthlyCost, setInfraMonthlyCost] = useState('0')
  const [baseUrl, setBaseUrl] = useState('')
  const [executionEndpoint, setExecutionEndpoint] = useState('')
  const [observableFields, setObservableFields] = useState<ObservableFieldForm[]>([])
  const [promptPrice, setPromptPrice] = useState('')
  const [cachedInputPrice, setCachedInputPrice] = useState('')
  const [completionPrice, setCompletionPrice] = useState('')
  const [markupPercentage, setMarkupPercentage] = useState('0')
  const [error, setError] = useState('')

  // Catalog-driven model selection
  const [catalogModelId, setCatalogModelId] = useState<string>('')
  const { data: catalogEntries } = useModelCatalog(
    { provider, active: true },
    !!provider
  )
  const hasCatalogEntries = !!provider && !!catalogEntries && catalogEntries.length > 0

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    // Validation
    if (!modelId.trim()) {
      setError('Model ID is required')
      return
    }
    if (!provider) {
      setError('Provider is required')
      return
    }
    if (!typeSelection) {
      setError('Type is required')
      return
    }
    if (typeSelection === 'ML' && !executionEndpoint.trim()) {
      setError('Execution endpoint is required for ML models')
      return
    }

    if (typeSelection === 'ML') {
      const normalizedFields = observableFields.map((field) => ({
        path: field.path.trim(),
        type: field.type,
        role: field.role,
      }))
      const invalidField = normalizedFields.find(
        (field) => !field.path || !['text', 'json', 'number'].includes(field.type) || !['input', 'output'].includes(field.role)
      )
      if (invalidField) {
        setError('Observable fields must include path, type, and role')
        return
      }
      const uniqueKeys = new Set<string>()
      for (const field of normalizedFields) {
        const key = `${field.path}::${field.type}::${field.role}`
        if (uniqueKeys.has(key)) {
          setError('Observable fields contain duplicate rows')
          return
        }
        uniqueKeys.add(key)
      }
    }
    if (existingModelIds.includes(modelId.trim())) {
      setError('A model with this ID already exists')
      return
    }

    const infraValue = Number(infraMonthlyCost)
    if (!Number.isFinite(infraValue) || infraValue < 0) {
      setError('Infrastructure monthly cost must be 0 or greater')
      return
    }

    const markupValue = markupPercentage.trim() === '' ? 0 : Number(markupPercentage)
    if (!Number.isFinite(markupValue) || markupValue < 0) {
      setError('Model Markup % must be a number ≥ 0')
      return
    }

    const mappedType =
      typeSelection === 'LLM'
        ? ''
        : typeSelection === 'Embedding'
        ? 'embedding'
        : 'ml'

    const model: Model = {
      id: modelId.trim(),
      provider,
      route_groups: selectedRouteGroups,
      Enabled: enabled,
      type: mappedType,
      infrastructure_monthly_usd: infraValue,
      markup_percentage: markupValue,
    }

    if (provider === 'bedrock') {
      const v = modelId.trim()
      model.provider_model_id = v
      ;(model as Record<string, unknown>).name = v
    }

    if (typeSelection !== 'ML') {
      model.base_url = baseUrl.trim()
    }

    if (typeSelection === 'ML') {
      model.execution = { endpoint: executionEndpoint.trim() }
      model.observable = {
        fields: observableFields.map((field) => ({
          path: field.path.trim(),
          type: field.type,
          role: field.role,
        })),
      }
    }

    if (typeSelection !== 'ML') {
      const isEmbedding = typeSelection === 'Embedding'
      if (promptPrice || cachedInputPrice || completionPrice || isEmbedding) {
        model.pricing = {
          prompt_per_1m: promptPrice ? parseFloat(promptPrice) : undefined,
          cached_input_per_1m: cachedInputPrice ? parseFloat(cachedInputPrice) : undefined,
          completion_per_1m: isEmbedding ? 0 : (completionPrice ? parseFloat(completionPrice) : undefined),
        }
      }
    }

    try {
      await createModel.mutateAsync(model)
      // Reset form
      setModelId('')
      setProvider('')
      setEnabled(true)
      setSelectedRouteGroups([])
      setTypeSelection('')
      setInfraMonthlyCost('0')
      setBaseUrl('')
      setExecutionEndpoint('')
      setObservableFields([])
      setPromptPrice('')
      setCachedInputPrice('')
      setCompletionPrice('')
      setMarkupPercentage('0')
      setCatalogModelId('')
      onOpenChange(false)
    } catch {
      // Error handled by mutation
    }
  }

  const toggleRouteGroup = (group: string) => {
    setSelectedRouteGroups((prev) =>
      prev.includes(group) ? prev.filter((g) => g !== group) : [...prev, group]
    )
  }

  const handleProviderSelect = (p: string) => {
    setProvider(p)
    // Reset catalog selection when provider changes
    setCatalogModelId('')
    setModelId('')
    setPromptPrice('')
    setCachedInputPrice('')
    setCompletionPrice('')
    setInfraMonthlyCost('0')
  }

  const handleCatalogEntrySelect = (entryId: string) => {
    setCatalogModelId(entryId)
    if (!entryId) {
      setModelId('')
      setPromptPrice('')
      setCachedInputPrice('')
      setCompletionPrice('')
      setInfraMonthlyCost('0')
      return
    }
    const entry = catalogEntries?.find((e) => e.id === entryId)
    if (!entry) return
    setModelId(entry.id)
    setPromptPrice(String(entry.prompt_per_1m))
    setCachedInputPrice(String(entry.cached_input_per_1m))
    setCompletionPrice(String(entry.completion_per_1m))
    setInfraMonthlyCost(String(entry.infrastructure_monthly_usd))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Create Model</DialogTitle>
          <DialogDescription>
            Add a new model to the gateway catalog
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="text-sm text-destructive bg-destructive/10 p-2 rounded">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label>Provider *</Label>
            <div className="flex flex-wrap gap-2">
              {providers.map((p) => (
                <button
                  key={p}
                  type="button"
                  onClick={() => handleProviderSelect(p)}
                  className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                    provider === p
                      ? 'border-primary bg-primary text-primary-foreground'
                      : 'border-border bg-background hover:bg-accent hover:text-accent-foreground'
                  }`}
                >
                  <ProviderIcon providerId={p} size="sm" />
                  <span>{providerLabel(p)}</span>
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="modelId">Model ID *</Label>
            {hasCatalogEntries ? (
              <Select value={catalogModelId} onValueChange={handleCatalogEntrySelect}>
                <SelectTrigger id="modelId">
                  <SelectValue placeholder="Select from catalog..." />
                </SelectTrigger>
                <SelectContent>
                  {catalogEntries!.map((entry) => (
                    <SelectItem key={entry.id} value={entry.id}>
                      {entry.display_name ? `${entry.display_name} (${entry.id})` : entry.id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                id="modelId"
                placeholder={
                  provider === 'bedrock'
                    ? 'e.g., anthropic.claude-3-5-sonnet-20240620-v1:0'
                    : 'e.g., gpt-4o-mini'
                }
                value={modelId}
                onChange={(e) => setModelId(e.target.value)}
              />
            )}
          </div>

          <div className="space-y-2">
            <Label>Route Groups</Label>
            <div className="flex flex-wrap gap-2">
              {routeGroups.map((group) => (
                <Badge
                  key={group}
                  variant={selectedRouteGroups.includes(group) ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => toggleRouteGroup(group)}
                >
                  {selectedRouteGroups.includes(group) && <Plus className="h-3 w-3 mr-1" />}
                  {group}
                </Badge>
              ))}
            </div>
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="enabled" title="If disabled, the model cannot be used and will return a 403 error.">
              Enabled
            </Label>
            <Switch id="enabled" checked={enabled} onCheckedChange={setEnabled} />
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="type">Type</Label>
              <Select value={typeSelection} onValueChange={setTypeSelection}>
                <SelectTrigger id="type">
                  <SelectValue placeholder="Select model type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="LLM">LLM</SelectItem>
                  <SelectItem value="Embedding">Embedding</SelectItem>
                  <SelectItem value="ML">ML</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">Defines how the router treats this model.</p>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="infra-cost">Infrastructure monthly cost (USD)</Label>
            <Input
              id="infra-cost"
              type="number"
              min="0"
              step="0.01"
              value={infraMonthlyCost}
              onChange={(e) => setInfraMonthlyCost(e.target.value)}
              disabled={!!catalogModelId}
            />
            <p className="text-xs text-muted-foreground">
              {catalogModelId
                ? 'Set by catalog entry.'
                : 'Fixed monthly infrastructure cost used for FinOps calculations.'}
            </p>
          </div>

          {(typeSelection === 'LLM' || typeSelection === 'Embedding') && (
            <div className="space-y-2">
              <Label htmlFor="base-url">Base URL (override)</Label>
              <Input
                id="base-url"
                placeholder="http://localhost:11434/v1"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Optional. Overrides the provider&apos;s base URL for this model. Use this for local models running on different endpoints.
              </p>
            </div>
          )}

          {typeSelection === 'ML' && (
            <div className="space-y-2">
              <Label htmlFor="execution-endpoint">Execution endpoint (URL)</Label>
              <Input
                id="execution-endpoint"
                placeholder="http://localhost:9001/predict"
                value={executionEndpoint}
                onChange={(e) => setExecutionEndpoint(e.target.value)}
              />
            </div>
          )}

          {typeSelection === 'ML' && (
            <ObservableFieldsEditor
              value={observableFields}
              onChange={setObservableFields}
              disabled={createModel.isPending}
            />
          )}

          {typeSelection !== 'ML' && (
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="promptPrice">Prompt Price (per 1M)</Label>
                <Input
                  id="promptPrice"
                  type="number"
                  step="0.01"
                  placeholder="0.15"
                  value={promptPrice}
                  onChange={(e) => setPromptPrice(e.target.value)}
                  disabled={!!catalogModelId}
                />
                {!!catalogModelId && (
                  <p className="text-xs text-muted-foreground">Set by catalog entry.</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="cachedInputPrice">Cached Input Price (per 1M)</Label>
                <Input
                  id="cachedInputPrice"
                  type="number"
                  step="0.01"
                  placeholder="0.075"
                  value={cachedInputPrice}
                  onChange={(e) => setCachedInputPrice(e.target.value)}
                  disabled={!!catalogModelId}
                />
                {!!catalogModelId && (
                  <p className="text-xs text-muted-foreground">Set by catalog entry.</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="completionPrice">Completion Price (per 1M)</Label>
                <Input
                  id="completionPrice"
                  type="number"
                  step="0.01"
                  placeholder="0.60"
                  value={typeSelection === 'Embedding' ? '0' : completionPrice}
                  onChange={(e) => setCompletionPrice(e.target.value)}
                  disabled={typeSelection === 'Embedding' || !!catalogModelId}
                />
                {typeSelection === 'Embedding' && (
                  <p className="text-xs text-muted-foreground">Embedding models have no completion cost.</p>
                )}
                {!!catalogModelId && typeSelection !== 'Embedding' && (
                  <p className="text-xs text-muted-foreground">Set by catalog entry.</p>
                )}
              </div>
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="markupPercentage" title="Percentage added on top of model cost to calculate final price.">
              Model Markup %
            </Label>
            <Input
              id="markupPercentage"
              type="number"
              min={0}
              step={0.1}
              value={markupPercentage}
              onChange={(e) => setMarkupPercentage(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Percentage added on top of model cost to calculate final price. Example: Cost $10 → Price $12 (20%
              markup).
            </p>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={createModel.isPending}>
              {createModel.isPending ? 'Creating...' : 'Create Model'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
