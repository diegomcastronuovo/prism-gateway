'use client'

import { useEffect, useState } from 'react'
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
import { useUpdateModel, type Model } from '../api/use-models'

interface EditModelDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  model: Model | null
  providers: string[]
}

export function EditModelDialog({
  open,
  onOpenChange,
  model,
  providers,
}: EditModelDialogProps) {
  const updateModel = useUpdateModel()
  const [provider, setProvider] = useState('')
  /** Same field as Create "Model ID"; for Bedrock editable and persisted as name + provider_model_id. */
  const [modelIdField, setModelIdField] = useState('')
  const [typeSelection, setTypeSelection] = useState('')
  const [infraMonthlyCost, setInfraMonthlyCost] = useState('0')
  const [baseUrl, setBaseUrl] = useState('')
  const [executionEndpoint, setExecutionEndpoint] = useState('')
  const [observableFields, setObservableFields] = useState<ObservableFieldForm[]>([])
  const [promptPrice, setPromptPrice] = useState('')
  const [cachedInputPrice, setCachedInputPrice] = useState('')
  const [completionPrice, setCompletionPrice] = useState('')
  const [markupPercentage, setMarkupPercentage] = useState('0')
  const [enabled, setEnabled] = useState(true)
  const [error, setError] = useState('')
  // Mock configuration
  const [mockEnabled, setMockEnabled] = useState(false)
  const [mockDelayMin, setMockDelayMin] = useState('')
  const [mockDelayMax, setMockDelayMax] = useState('')
  const [mockErrorRate, setMockErrorRate] = useState('')
  const [mockErrorStatus, setMockErrorStatus] = useState('')
  const [mockErrorMessage, setMockErrorMessage] = useState('')
  const [mockFixedResponse, setMockFixedResponse] = useState('')

  // Load current values when dialog opens
  useEffect(() => {
    if (open && model) {
      const rawEnabled = (model as Record<string, unknown>)['Enabled'] ?? (model as Record<string, unknown>)['enabled']
      setEnabled(rawEnabled === undefined || rawEnabled === null ? true : Boolean(rawEnabled))
      setProvider(model.provider || '')
      const pmid = (model as { provider_model_id?: string | null }).provider_model_id
      if (model.provider === 'bedrock') {
        setModelIdField(
          pmid != null && String(pmid).trim() !== '' ? String(pmid).trim() : model.id
        )
      } else {
        setModelIdField(model.id)
      }
      setTypeSelection(
        model.type === 'embedding' ? 'Embedding' : model.type === 'ml' ? 'ML' : 'LLM'
      )
      setInfraMonthlyCost(
        model.infrastructure_monthly_usd !== undefined
          ? String(model.infrastructure_monthly_usd)
          : '0'
      )
      setBaseUrl(model.base_url || '')
      setExecutionEndpoint(model.execution?.endpoint || '')
      const fields = model.observable?.fields
      if (Array.isArray(fields)) {
        setObservableFields(
          fields.map((field) => ({
            path: field.path,
            type: field.type,
            role: field.role,
          }))
        )
      } else {
        setObservableFields([])
      }
      const pr = model.pricing as Record<string, unknown> | undefined
      setPromptPrice(
        (pr?.prompt_per_1m ?? pr?.PromptPer1M ?? pr?.promptPer1M)?.toString?.() || ''
      )
      setCachedInputPrice(
        (pr?.cached_input_per_1m ?? pr?.CachedInputPer1M ?? pr?.cachedInputPer1M)?.toString?.() || ''
      )
      setCompletionPrice(
        model.type === 'embedding'
          ? '0'
          : (pr?.completion_per_1m ?? pr?.CompletionPer1M ?? pr?.completionPer1M)?.toString?.() || ''
      )
      const mp = model.markup_percentage
      setMarkupPercentage(
        mp != null && Number.isFinite(Number(mp)) ? String(mp) : '0'
      )
      // Load mock config
      setMockEnabled(model.mock?.enabled ?? false)
      setMockDelayMin(model.mock?.delay_min_ms?.toString() || '')
      setMockDelayMax(model.mock?.delay_max_ms?.toString() || '')
      setMockErrorRate(model.mock?.error_rate?.toString() || '')
      setMockErrorStatus(model.mock?.error_status?.toString() || '')
      setMockErrorMessage(model.mock?.error_message || '')
      setMockFixedResponse(model.mock?.fixed_response || '')
    }
  }, [open, model])

  const handleProviderChange = (p: string) => {
    setProvider(p)
    if (!model) return
    if (p === 'bedrock') {
      const pmid = (model as { provider_model_id?: string | null }).provider_model_id
      setModelIdField(
        pmid != null && String(pmid).trim() !== '' ? String(pmid).trim() : model.id
      )
    } else {
      setModelIdField(model.id)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!model) return
    setError('')

    const version = (model as { version?: number }).version || 1

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

    if (typeSelection === 'ML' && !executionEndpoint.trim()) {
      setError('Execution endpoint is required for ML models')
      return
    }

    if (provider === 'bedrock' && !modelIdField.trim()) {
      setError('Model ID is required for Bedrock models')
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

    const mappedType =
      typeSelection === 'LLM'
        ? ''
        : typeSelection === 'Embedding'
        ? 'embedding'
        : 'ml'

    const update: Partial<Model> = {
      provider,
      Enabled: enabled,
      type: mappedType,
      infrastructure_monthly_usd: infraValue,
      markup_percentage: markupValue,
    }

    if (provider === 'bedrock') {
      const v = modelIdField.trim()
      update.provider_model_id = v
      ;(update as Record<string, unknown>).name = v
    }

    if (typeSelection !== 'ML') {
      update.base_url = baseUrl.trim()
    }

    if (typeSelection === 'ML') {
      update.execution = { endpoint: executionEndpoint.trim() }
      update.observable = {
        fields: observableFields.map((field) => ({
          path: field.path.trim(),
          type: field.type,
          role: field.role,
        })),
      }
    }

    const isEmbedding = typeSelection === 'Embedding'
    if (promptPrice || cachedInputPrice || completionPrice || isEmbedding) {
      update.pricing = {
        prompt_per_1m: promptPrice ? parseFloat(promptPrice) : undefined,
        cached_input_per_1m: cachedInputPrice ? parseFloat(cachedInputPrice) : undefined,
        completion_per_1m: isEmbedding ? 0 : (completionPrice ? parseFloat(completionPrice) : undefined),
      }
    }

    // Add mock configuration if enabled, has values, or turning off previously-enabled mock
    const hadMockEnabled = Boolean(model.mock?.enabled)
    const shouldPatchMock =
      mockEnabled ||
      mockDelayMin ||
      mockDelayMax ||
      mockErrorRate ||
      mockErrorStatus ||
      mockErrorMessage ||
      mockFixedResponse ||
      (hadMockEnabled && !mockEnabled)

    if (shouldPatchMock) {
      update.mock = {
        enabled: mockEnabled,
        delay_min_ms: mockDelayMin ? parseInt(mockDelayMin, 10) : undefined,
        delay_max_ms: mockDelayMax ? parseInt(mockDelayMax, 10) : undefined,
        error_rate: mockErrorRate ? parseFloat(mockErrorRate) : undefined,
        error_status: mockErrorStatus ? parseInt(mockErrorStatus, 10) : undefined,
        error_message: mockErrorMessage || undefined,
        fixed_response: mockFixedResponse || undefined,
      }
    }

    try {
      await updateModel.mutateAsync({ modelId: model.id, model: update, version })
      onOpenChange(false)
    } catch {
      // Error handled by mutation
    }
  }

  if (!model) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Edit Model</DialogTitle>
          <DialogDescription>
            Update {model.id} configuration
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="text-sm text-destructive bg-destructive/10 p-2 rounded">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="edit-model-id">Model ID *</Label>
            <Input
              id="edit-model-id"
              placeholder={
                provider === 'bedrock'
                  ? 'e.g., anthropic.claude-3-5-sonnet-20240620-v1:0'
                  : 'e.g., gpt-4o-mini'
              }
              value={modelIdField}
              onChange={(e) => setModelIdField(e.target.value)}
              readOnly={provider !== 'bedrock'}
            />
          </div>

          <div className="space-y-2">
            <Label>Provider *</Label>
            <div className="flex flex-wrap gap-2">
              {providers.map((p) => (
                <Badge
                  key={p}
                  variant={provider === p ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => handleProviderChange(p)}
                >
                  {p}
                </Badge>
              ))}
            </div>
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="edit-enabled" title="If disabled, the model cannot be used and will return a 403 error.">
              Enabled
            </Label>
            <Switch id="edit-enabled" checked={enabled} onCheckedChange={setEnabled} />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="type">Type</Label>
              <Select value={typeSelection} onValueChange={(v) => { setTypeSelection(v); if (v === 'Embedding') setCompletionPrice('0') }}>
                <SelectTrigger id="type">
                  <SelectValue placeholder="Select model type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="LLM">LLM</SelectItem>
                  <SelectItem value="Embedding">Embedding</SelectItem>
                  <SelectItem value="ML">ML</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">Controls how the router treats this model.</p>
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
              />
              <p className="text-xs text-muted-foreground">
                Monthly fixed infrastructure cost used for FinOps calculations.
              </p>
            </div>
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
              disabled={updateModel.isPending}
            />
          )}

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
              />
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
              />
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
                disabled={typeSelection === 'Embedding'}
              />
              {typeSelection === 'Embedding' && (
                <p className="text-xs text-muted-foreground">Embedding models have no completion cost.</p>
              )}
            </div>
          </div>

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

          {/* Mock Configuration Section */}
          <div className="border-t pt-4">
            <div className="flex items-center justify-between mb-4">
              <Label htmlFor="mockEnabled" className="font-medium">Mock Configuration</Label>
              <Switch id="mockEnabled" checked={mockEnabled} onCheckedChange={setMockEnabled} />
            </div>
            
            {mockEnabled && (
              <div className="space-y-4 pl-4 border-l-2 border-muted">
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="mockDelayMin">Delay Min (ms)</Label>
                    <Input
                      id="mockDelayMin"
                      type="number"
                      placeholder="100"
                      value={mockDelayMin}
                      onChange={(e) => setMockDelayMin(e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="mockDelayMax">Delay Max (ms)</Label>
                    <Input
                      id="mockDelayMax"
                      type="number"
                      placeholder="500"
                      value={mockDelayMax}
                      onChange={(e) => setMockDelayMax(e.target.value)}
                    />
                  </div>
                </div>
                
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="mockErrorRate">Error Rate (0-1)</Label>
                    <Input
                      id="mockErrorRate"
                      type="number"
                      step="0.01"
                      min="0"
                      max="1"
                      placeholder="0.0"
                      value={mockErrorRate}
                      onChange={(e) => setMockErrorRate(e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="mockErrorStatus">Error Status Code</Label>
                    <Input
                      id="mockErrorStatus"
                      type="number"
                      placeholder="500"
                      value={mockErrorStatus}
                      onChange={(e) => setMockErrorStatus(e.target.value)}
                    />
                  </div>
                </div>
                
                <div className="space-y-2">
                  <Label htmlFor="mockErrorMessage">Error Message</Label>
                  <Input
                    id="mockErrorMessage"
                    placeholder="Mock error message"
                    value={mockErrorMessage}
                    onChange={(e) => setMockErrorMessage(e.target.value)}
                  />
                </div>
                
                <div className="space-y-2">
                  <Label htmlFor="mockFixedResponse">Fixed Response (JSON)</Label>
                  <textarea
                    id="mockFixedResponse"
                    className="w-full min-h-[80px] rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                    placeholder='{"choices": [...]}'
                    value={mockFixedResponse}
                    onChange={(e) => setMockFixedResponse(e.target.value)}
                  />
                </div>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={updateModel.isPending}>
              {updateModel.isPending ? 'Saving...' : 'Save Changes'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
