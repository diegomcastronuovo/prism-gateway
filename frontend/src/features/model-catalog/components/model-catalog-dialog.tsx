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
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  useCreateModelCatalogEntry,
  useUpdateModelCatalogEntry,
  type ModelCatalogEntry,
  type CreateModelCatalogEntryInput,
} from '../api/use-model-catalog'
import { useProviders } from '@/features/providers/api/use-providers'
import { catalogProviderIdsForModels } from '@/features/providers/lib/catalog-provider-ids'
import { ProviderIcon, providerLabel } from '@/features/providers/components/provider-icon'

interface ModelCatalogDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  entry?: ModelCatalogEntry
}

export function ModelCatalogDialog({ open, onOpenChange, entry }: ModelCatalogDialogProps) {
  const isEdit = !!entry

  const createEntry = useCreateModelCatalogEntry()
  const updateEntry = useUpdateModelCatalogEntry()
  const { data: runtimeProviders } = useProviders()
  const providerList = catalogProviderIdsForModels(runtimeProviders)

  const [id, setId] = useState('')
  const [provider, setProvider] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [type, setType] = useState('')
  const [promptPer1m, setPromptPer1m] = useState('0')
  const [cachedInputPer1m, setCachedInputPer1m] = useState('0')
  const [completionPer1m, setCompletionPer1m] = useState('0')
  const [infraMonthlyUsd, setInfraMonthlyUsd] = useState('0')
  const [isActive, setIsActive] = useState(true)
  const [longContext, setLongContext] = useState(false)
  const [longContextStartTokens, setLongContextStartTokens] = useState('272000')
  const [longContextPromptPer1m, setLongContextPromptPer1m] = useState('0')
  const [longContextCachedInputPer1m, setLongContextCachedInputPer1m] = useState('0')
  const [longContextCompletionPer1m, setLongContextCompletionPer1m] = useState('0')
  const [error, setError] = useState('')

  // Populate form fields when editing
  useEffect(() => {
    if (entry) {
      setId(entry.id)
      setProvider(entry.provider)
      setDisplayName(entry.display_name)
      setType(entry.type)
      setPromptPer1m(String(entry.prompt_per_1m))
      setCachedInputPer1m(String(entry.cached_input_per_1m))
      setCompletionPer1m(String(entry.completion_per_1m))
      setInfraMonthlyUsd(String(entry.infrastructure_monthly_usd))
      setIsActive(entry.is_active)
      setLongContext(entry.long_context)
      setLongContextStartTokens(String(entry.long_context_start_tokens))
      setLongContextPromptPer1m(String(entry.long_context_prompt_per_1m))
      setLongContextCachedInputPer1m(String(entry.long_context_cached_input_per_1m))
      setLongContextCompletionPer1m(String(entry.long_context_completion_per_1m))
    } else {
      setId('')
      setProvider('')
      setDisplayName('')
      setType('')
      setPromptPer1m('0')
      setCachedInputPer1m('0')
      setCompletionPer1m('0')
      setInfraMonthlyUsd('0')
      setIsActive(true)
      setLongContext(false)
      setLongContextStartTokens('272000')
      setLongContextPromptPer1m('0')
      setLongContextCachedInputPer1m('0')
      setLongContextCompletionPer1m('0')
    }
    setError('')
  }, [entry, open])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    if (!id.trim()) {
      setError('Model ID is required')
      return
    }
    if (!provider.trim()) {
      setError('Provider is required')
      return
    }
    if (!type) {
      setError('Type is required')
      return
    }

    const promptVal = parseFloat(promptPer1m)
    const cachedInputVal = parseFloat(cachedInputPer1m)
    const completionVal = parseFloat(completionPer1m)
    const infraVal = parseFloat(infraMonthlyUsd)

    if (!Number.isFinite(promptVal) || promptVal < 0) {
      setError('Prompt price must be 0 or greater')
      return
    }
    if (!Number.isFinite(cachedInputVal) || cachedInputVal < 0) {
      setError('Cached input price must be 0 or greater')
      return
    }
    if (!Number.isFinite(completionVal) || completionVal < 0) {
      setError('Completion price must be 0 or greater')
      return
    }
    if (!Number.isFinite(infraVal) || infraVal < 0) {
      setError('Infrastructure monthly cost must be 0 or greater')
      return
    }

    const longContextStartTokensVal = parseInt(longContextStartTokens, 10)
    const longContextPromptVal = parseFloat(longContextPromptPer1m)
    const longContextCachedInputVal = parseFloat(longContextCachedInputPer1m)
    const longContextCompletionVal = parseFloat(longContextCompletionPer1m)

    if (longContext) {
      if (!Number.isInteger(longContextStartTokensVal) || longContextStartTokensVal < 1) {
        setError('Long context start tokens must be a positive integer')
        return
      }
      if (!Number.isFinite(longContextPromptVal) || longContextPromptVal <= 0) {
        setError('Long context input price must be greater than 0')
        return
      }
      if (!Number.isFinite(longContextCompletionVal) || longContextCompletionVal <= 0) {
        setError('Long context output price must be greater than 0')
        return
      }
    }

    try {
      if (isEdit && entry) {
        await updateEntry.mutateAsync({
          provider: entry.provider,
          id: entry.id,
          data: {
            display_name: displayName.trim(),
            type,
            prompt_per_1m: promptVal,
            cached_input_per_1m: cachedInputVal,
            completion_per_1m: completionVal,
            infrastructure_monthly_usd: infraVal,
            is_active: isActive,
            long_context: longContext,
            long_context_start_tokens: longContextStartTokensVal,
            long_context_prompt_per_1m: longContextPromptVal,
            long_context_cached_input_per_1m: longContextCachedInputVal,
            long_context_completion_per_1m: longContextCompletionVal,
          },
        })
      } else {
        const payload: CreateModelCatalogEntryInput = {
          id: id.trim(),
          provider: provider.trim(),
          display_name: displayName.trim(),
          type,
          prompt_per_1m: promptVal,
          cached_input_per_1m: cachedInputVal,
          completion_per_1m: completionVal,
          infrastructure_monthly_usd: infraVal,
          is_active: isActive,
          long_context: longContext,
          long_context_start_tokens: longContextStartTokensVal,
          long_context_prompt_per_1m: longContextPromptVal,
          long_context_cached_input_per_1m: longContextCachedInputVal,
          long_context_completion_per_1m: longContextCompletionVal,
        }
        await createEntry.mutateAsync(payload)
      }
      onOpenChange(false)
    } catch {
      // Error handled by mutation
    }
  }

  const isPending = createEntry.isPending || updateEntry.isPending

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? 'Edit Model Catalog Entry' : 'Add Model Catalog Entry'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Update pricing and metadata for this model catalog entry.'
              : 'Add a new model to the pricing catalog.'}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="text-sm text-destructive bg-destructive/10 p-2 rounded">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="catalog-provider">Provider *</Label>
            <Select value={provider} onValueChange={setProvider} disabled={isEdit}>
              <SelectTrigger id="catalog-provider" className="h-11">
                <SelectValue placeholder="Select provider">
                  {provider && (
                    <span className="flex items-center gap-2">
                      <ProviderIcon providerId={provider} size="sm" />
                      <span>{providerLabel(provider)}</span>
                    </span>
                  )}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {providerList.map((p) => (
                  <SelectItem key={p} value={p} className="py-2">
                    <span className="flex items-center gap-3">
                      <ProviderIcon providerId={p} size="lg" />
                      <span className="font-medium text-sm">{providerLabel(p)}</span>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="catalog-id">Model ID *</Label>
              <Input
                id="catalog-id"
                placeholder="e.g., gpt-4o"
                value={id}
                onChange={(e) => setId(e.target.value)}
                disabled={isEdit}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="catalog-display-name">Display Name</Label>
              <Input
                id="catalog-display-name"
                placeholder="e.g., GPT-4o"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="catalog-type">Type *</Label>
            <Select value={type} onValueChange={setType}>
              <SelectTrigger id="catalog-type">
                <SelectValue placeholder="Select model type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="LLM">LLM</SelectItem>
                <SelectItem value="Embedding">Embedding</SelectItem>
                <SelectItem value="ML">ML</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="catalog-prompt-price">Input Price per 1M (USD)</Label>
              <Input
                id="catalog-prompt-price"
                type="number"
                min="0"
                step="0.000001"
                placeholder="0.0025"
                value={promptPer1m}
                onChange={(e) => setPromptPer1m(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="catalog-cached-input-price">Cached Input Price per 1M (USD)</Label>
              <Input
                id="catalog-cached-input-price"
                type="number"
                min="0"
                step="0.000001"
                placeholder="0"
                value={cachedInputPer1m}
                onChange={(e) => setCachedInputPer1m(e.target.value)}
              />
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="catalog-completion-price">Output Price per 1M (USD)</Label>
              <Input
                id="catalog-completion-price"
                type="number"
                min="0"
                step="0.000001"
                placeholder="0.010"
                value={completionPer1m}
                onChange={(e) => setCompletionPer1m(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="catalog-infra-cost">Infrastructure Monthly Cost (USD)</Label>
              <Input
                id="catalog-infra-cost"
                type="number"
                min="0"
                step="0.01"
                placeholder="0"
                value={infraMonthlyUsd}
                onChange={(e) => setInfraMonthlyUsd(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Fixed monthly infrastructure cost used for FinOps calculations.
              </p>
            </div>
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="catalog-is-active">Active</Label>
            <Switch id="catalog-is-active" checked={isActive} onCheckedChange={setIsActive} />
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="catalog-long-context">Long Context</Label>
            <Switch id="catalog-long-context" checked={longContext} onCheckedChange={setLongContext} />
          </div>

          {longContext && (
            <div className="space-y-4 rounded-md border p-4">
              <p className="text-xs text-muted-foreground">
                Long context pricing applies when the request exceeds the start token threshold.
              </p>

              <div className="space-y-2">
                <Label htmlFor="catalog-long-context-start-tokens">Long Context Start Tokens</Label>
                <Input
                  id="catalog-long-context-start-tokens"
                  type="number"
                  min="1"
                  step="1"
                  placeholder="272000"
                  value={longContextStartTokens}
                  onChange={(e) => setLongContextStartTokens(e.target.value)}
                />
              </div>

              <div className="grid gap-4 md:grid-cols-3">
                <div className="space-y-2">
                  <Label htmlFor="catalog-lc-prompt">Long Context Input Price per 1M (USD)</Label>
                  <Input
                    id="catalog-lc-prompt"
                    type="number"
                    min="0"
                    step="0.000001"
                    placeholder="0"
                    value={longContextPromptPer1m}
                    onChange={(e) => setLongContextPromptPer1m(e.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="catalog-lc-cached-input">Long Context Cached Input per 1M (USD)</Label>
                  <Input
                    id="catalog-lc-cached-input"
                    type="number"
                    min="0"
                    step="0.000001"
                    placeholder="0"
                    value={longContextCachedInputPer1m}
                    onChange={(e) => setLongContextCachedInputPer1m(e.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="catalog-lc-completion">Long Context Output Price per 1M (USD)</Label>
                  <Input
                    id="catalog-lc-completion"
                    type="number"
                    min="0"
                    step="0.000001"
                    placeholder="0"
                    value={longContextCompletionPer1m}
                    onChange={(e) => setLongContextCompletionPer1m(e.target.value)}
                  />
                </div>
              </div>
            </div>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isPending}>
              {isPending ? (isEdit ? 'Saving...' : 'Creating...') : (isEdit ? 'Save Changes' : 'Add Entry')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
