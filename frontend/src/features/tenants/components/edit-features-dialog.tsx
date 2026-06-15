'use client'

import { useEffect, useMemo, useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { useGlobalConfig } from '@/features/global-config/api/use-global-config'
import { useUpdateTenantConfig, type TenantConfig } from '../api/use-tenants'

interface EditFeaturesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantConfig: TenantConfig | null
}

interface FeatureToggle {
  key: string
  name: string
  description: string
}

const features: FeatureToggle[] = [
  {
    key: 'semantic_routing',
    name: 'Semantic Routing',
    description: 'Route requests based on semantic similarity',
  },
  {
    key: 'semantic_cache',
    name: 'Semantic Cache',
    description: 'Cache responses based on semantic similarity',
  },
  {
    key: 'tool_routing',
    name: 'Tool Routing',
    description: 'Route tool-enabled requests to appropriate models',
  },
  {
    key: 'budget_enforcement',
    name: 'Budget Enforcement',
    description: 'Enforce budget limits on tenant usage',
  },
]

const defaultSemanticStage = {
  name: 'semantic_intent',
  rules: [
    {
      action: { use_anchor: true },
      
      when: { semantic_similarity: {} },
    },
  ],
}

function getSemanticRoutingEnabled(config: Record<string, unknown>): boolean {
  const routing = config.routing as Record<string, unknown> | undefined
  const smart = routing?.smart as Record<string, unknown> | undefined
  const stages = smart?.stages as Array<Record<string, unknown>> | undefined

  if (!Array.isArray(stages)) {
    return false
  }

  const semanticStage = stages.find((stage) => stage?.name === 'semantic_intent')
  if (!semanticStage) {
    return false
  }

  const rules = semanticStage.rules as unknown
  return Array.isArray(rules) && rules.length > 0
}

function getToolRoutingEnabled(config: Record<string, unknown>): boolean {
  const value = config.tool_routing_enabled as boolean | null | undefined
  return value === false ? false : true
}

function buildSemanticRoutingStages(
  config: Record<string, unknown>,
  enabled: boolean,
): Array<Record<string, unknown>> | null {
  const routing = config.routing as Record<string, unknown> | undefined
  const smart = routing?.smart as Record<string, unknown> | undefined
  const stages = smart?.stages as Array<Record<string, unknown>> | undefined

  if (!Array.isArray(stages)) {
    return enabled ? [defaultSemanticStage] : null
  }

  const hasSemanticStage = stages.some((stage) => stage?.name === 'semantic_intent')

  if (!enabled) {
    if (!hasSemanticStage) {
      return null
    }
    return stages.filter((stage) => stage?.name !== 'semantic_intent')
  }

  if (!hasSemanticStage) {
    return [...stages, defaultSemanticStage]
  }

  return stages.map((stage) => {
    if (stage?.name !== 'semantic_intent') {
      return stage
    }
    const rules = stage.rules as unknown
    if (Array.isArray(rules) && rules.length > 0) {
      return stage
    }
    return { ...stage, rules: defaultSemanticStage.rules }
  })
}

export function EditFeaturesDialog({ open, onOpenChange, tenantConfig }: EditFeaturesDialogProps) {
  const updateConfig = useUpdateTenantConfig()
  const [toggles, setToggles] = useState<Record<string, boolean>>({})
  const [embeddingModel, setEmbeddingModel] = useState('')
  const [cacheTTL, setCacheTTL] = useState<number>(86400)
  const globalConfigQuery = useGlobalConfig(open)
  const embeddingModels = useMemo(() => {
    const config = globalConfigQuery.data?.config as Record<string, unknown> | undefined
    const models = (config?.models as Array<Record<string, unknown>>) || []
    const allowedModels = (tenantConfig?.config?.allowed_models || []).filter(
      (model) => typeof model === 'string'
    )
    const allowedSet = new Set(allowedModels)
    return models
      .filter((model) => String(model?.Type ?? '') === 'embedding')
      .map((model) => ({
        name: String(model?.Name ?? ''),
        provider: String(model?.Provider ?? ''),
      }))
      .filter((model) => model.name.length > 0 && allowedSet.has(model.name))
  }, [globalConfigQuery.data, tenantConfig])
  const embeddingModelOptions = embeddingModels.map((model) => model.name)
  const embeddingModelValid = embeddingModel === '' || embeddingModelOptions.includes(embeddingModel)

  // Load current values
  useEffect(() => {
    if (open && tenantConfig) {
      const config = tenantConfig.config as Record<string, unknown>
      const routing = config.routing as Record<string, unknown> | undefined
      const semantic = routing?.semantic as Record<string, unknown> | undefined
      const initialToggles: Record<string, boolean> = {}
      initialToggles.semantic_routing = getSemanticRoutingEnabled(tenantConfig.config)
      const scCache = tenantConfig.config.semantic_cache as Record<string, unknown> | undefined
      initialToggles.semantic_cache = scCache?.enabled === true
      const ttlVal = scCache?.ttl_seconds
      setCacheTTL(typeof ttlVal === 'number' && ttlVal > 0 ? ttlVal : 86400)
      initialToggles.tool_routing = getToolRoutingEnabled(tenantConfig.config)
      initialToggles.budget_enforcement =
        (tenantConfig.config.budget_enforcement as Record<string, unknown> | undefined)?.enabled === true
      setToggles(initialToggles)
      const currentEmbedding = semantic?.embedding_model
      setEmbeddingModel(typeof currentEmbedding === 'string' ? currentEmbedding : '')
    }
  }, [open, tenantConfig])

  const handleToggle = (key: string, value: boolean) => {
    setToggles(prev => ({ ...prev, [key]: value }))
  }

  const onSubmit = async () => {
    if (!tenantConfig) return

    try {
      const patch: Record<string, unknown> = {}
      const patchRouting: Record<string, unknown> = {
        semantic: {
          embedding_model: embeddingModel,
        },
      }

      patch.semantic_cache = { enabled: toggles.semantic_cache, embedding_model: embeddingModel, ttl_seconds: cacheTTL }
      patch.budget_enforcement = { enabled: toggles.budget_enforcement }
      patch.tool_routing_enabled = toggles.tool_routing

      const updatedStages = buildSemanticRoutingStages(tenantConfig.config, toggles.semantic_routing)
      if (updatedStages) {
        patchRouting.smart = { stages: updatedStages }
      }
      patch.routing = patchRouting

      await updateConfig.mutateAsync({
        tenantId: tenantConfig.tenant_id,
        version: tenantConfig.version,
        patch,
      })
      onOpenChange(false)
    } catch {
      // Error is handled by the mutation
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Edit Features</DialogTitle>
          <DialogDescription>
            Toggle features for {tenantConfig?.tenant_id}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          {features.map((feature) => (
            <div
              key={feature.key}
              className="rounded-lg border p-3 space-y-3"
            >
              <div className="flex items-start justify-between gap-4">
                <div className="space-y-0.5">
                  <Label htmlFor={feature.key} className="font-medium">
                    {feature.name}
                  </Label>
                  <p className="text-sm text-muted-foreground">{feature.description}</p>
                </div>
                <Switch
                  id={feature.key}
                  checked={toggles[feature.key] || false}
                  onCheckedChange={(checked: boolean) => handleToggle(feature.key, checked)}
                />
              </div>
              {feature.key === 'semantic_cache' && (
                <div className="space-y-1">
                  <Label htmlFor="cache_ttl" className="text-sm">
                    Cache TTL (seconds)
                  </Label>
                  <Input
                    id="cache_ttl"
                    type="number"
                    min={1}
                    value={cacheTTL}
                    onChange={(e) => {
                      const v = parseInt(e.target.value, 10)
                      setCacheTTL(isNaN(v) || v < 1 ? 86400 : v)
                    }}
                    className="h-8 w-36"
                  />
                  <p className="text-xs text-muted-foreground">
                    How long cached responses are valid. Default: 86400 (24 h).
                  </p>
                </div>
              )}
            </div>
          ))}
          <div className="rounded-lg border p-3 space-y-3">
            <div className="space-y-0.5">
              <Label className="font-medium">Embedding model for tenant</Label>
              <p className="text-sm text-muted-foreground">
                Used by Semantic Routing and Semantic Cache for this tenant.
              </p>
            </div>
            <Select
              value={embeddingModel ? embeddingModel : '__empty__'}
              onValueChange={(value) => setEmbeddingModel(value === '__empty__' ? '' : value)}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select an embedding model" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__empty__">Clear selection</SelectItem>
                {!embeddingModelValid && embeddingModel && (
                  <SelectItem value={embeddingModel} disabled>
                    {embeddingModel} (not in catalog)
                  </SelectItem>
                )}
                {embeddingModels.map((model) => (
                  <SelectItem key={model.name} value={model.name}>
                    {model.name} {model.provider ? `(${model.provider})` : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {!embeddingModelValid && (
              <p className="text-xs text-destructive">
                Current model is not in the global embedding catalog. Please select another or clear it.
              </p>
            )}
            {embeddingModel === '' && (
              <p className="text-xs text-muted-foreground">
                Leave empty to remove tenant-level embedding model configuration.
              </p>
            )}
            <p className="text-xs text-muted-foreground">
              Only models of type "embedding" are shown here.
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button onClick={onSubmit} disabled={updateConfig.isPending}>
            {updateConfig.isPending ? 'Saving...' : 'Save Changes'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
