'use client'

import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import * as z from 'zod'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { X, Loader2 } from 'lucide-react'
import { useUpdateTenantConfig, type TenantConfig } from '../api/use-tenants'

const routingSchema = z.object({
  strategy: z.enum(['smart', 'round_robin', 'latency', 'cost', 'header']),
})

type RoutingFormData = z.infer<typeof routingSchema>

interface EditRoutingDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantConfig: TenantConfig | null
}

const strategies = [
  { value: 'smart',       label: 'Smart',       detail: 'Stage-based routing with cost / latency / error weights' },
  { value: 'round_robin', label: 'Round Robin', detail: 'Round-robin distribution across allowed models' },
  { value: 'latency',     label: 'Latency',     detail: 'Ordered by EWMA latency' },
  { value: 'cost',        label: 'Cost',        detail: 'Ordered by estimated cost' },
  { value: 'header',      label: 'Header',      detail: 'No logic — uses candidate order from headers' },
]

const STRATEGY_DETAIL: Record<string, string> = Object.fromEntries(
  strategies.map((s) => [s.value, s.detail])
)

export function EditRoutingDialog({ open, onOpenChange, tenantConfig }: EditRoutingDialogProps) {
  const updateConfig = useUpdateTenantConfig()
  const [models, setModels] = useState<string[]>([])
  const [availableRouteGroups, setAvailableRouteGroups] = useState<string[]>([])
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [selectedRouteGroups, setSelectedRouteGroups] = useState<string[]>([])
  const [defaultRouteGroup, setDefaultRouteGroup] = useState<string | null>(null)
  const [loadingCatalogs, setLoadingCatalogs] = useState(false)

  const {
    handleSubmit,
    formState: { errors },
    reset,
    setValue,
    watch,
  } = useForm<RoutingFormData>({
    resolver: zodResolver(routingSchema),
    defaultValues: {
      strategy: 'smart',
    },
  })

  const selectedStrategy = watch('strategy')

  useEffect(() => {
    if (open && tenantConfig) {
      const rawStrategy = (
        (tenantConfig.config.routing?.strategy as RoutingFormData['strategy'])
        ?? (tenantConfig.config.routing_strategy as RoutingFormData['strategy'])
        ?? 'smart'
      )
      // Treat removed enterprise strategies as 'smart'
      const strategy = strategies.some(s => s.value === rawStrategy) ? rawStrategy : 'smart'
      reset({ strategy })
      setSelectedModels(tenantConfig.config.allowed_models || [])
      setSelectedRouteGroups(tenantConfig.config.route_groups || [])
      setDefaultRouteGroup(tenantConfig.config.routing?.route_group ?? null)

      setLoadingCatalogs(true)
      ;(async () => {
        try {
          const [modelsRes, routeGroupsRes] = await Promise.all([
            fetch('/api/models').then(r => r.json()),
            fetch(`/api/route-groups?tenantId=${encodeURIComponent(tenantConfig.tenant_id)}`).then(r => r.json()),
          ])

          const allModels = (modelsRes.data || []) as Array<{ id?: string; model_id?: string }>
          setModels(allModels.map(m => m.model_id || m.id || '').filter(Boolean))
          setAvailableRouteGroups(
            (routeGroupsRes.data || [])
              .map((rg: { id?: string; name?: string }) => rg.id || rg.name || '')
              .filter(Boolean)
          )
        } finally {
          setLoadingCatalogs(false)
        }
      })()
    }
  }, [open, tenantConfig, reset])

  const toggleModel = (model: string) => {
    setSelectedModels(prev =>
      prev.includes(model) ? prev.filter(m => m !== model) : [...prev, model]
    )
  }

  const toggleRouteGroup = (group: string) => {
    setSelectedRouteGroups(prev =>
      prev.includes(group) ? prev.filter(g => g !== group) : [...prev, group]
    )
  }

  const onSubmit = async (data: RoutingFormData) => {
    if (!tenantConfig) return

    try {
      const patch: Record<string, unknown> = {
        routing: {
          strategy: data.strategy,
          route_group: defaultRouteGroup ?? null,
        },
      }
      if (selectedModels.length > 0) {
        patch.allowed_models = selectedModels
      }
      patch.route_groups = selectedRouteGroups

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
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit Routing</DialogTitle>
          <DialogDescription>
            Update routing configuration for {tenantConfig?.tenant_id}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-6 py-4">
            {/* Strategy */}
            <div className="space-y-2">
              <Label htmlFor="strategy">Routing Strategy</Label>
              <Select
                value={selectedStrategy}
                onValueChange={(value: RoutingFormData['strategy']) => setValue('strategy', value)}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select strategy" />
                </SelectTrigger>
                <SelectContent>
                  {strategies.map((s) => (
                    <SelectItem key={s.value} value={s.value}>
                      {s.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {STRATEGY_DETAIL[selectedStrategy] && (
                <p className="text-xs text-muted-foreground">
                  <span className="font-medium text-foreground">Details:</span>{' '}
                  {STRATEGY_DETAIL[selectedStrategy]}
                </p>
              )}
              {errors.strategy && (
                <p className="text-sm text-destructive">{errors.strategy.message}</p>
              )}
            </div>

            {/* Default Route Group + Route Groups */}
            <div className="space-y-2">
              <Label htmlFor="default-route-group">
                Default Route Group
                <span className="ml-1 text-xs text-muted-foreground font-normal">(optional)</span>
              </Label>
              {loadingCatalogs ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Loading route groups...
                </div>
              ) : (
                <>
                  <Select
                    value={defaultRouteGroup ?? '__none__'}
                    onValueChange={(value) => setDefaultRouteGroup(value === '__none__' ? null : value)}
                    disabled={availableRouteGroups.length === 0}
                  >
                    <SelectTrigger id="default-route-group">
                      <SelectValue placeholder="None" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__none__">None</SelectItem>
                      {[...availableRouteGroups].sort().map((group) => (
                        <SelectItem key={group} value={group}>
                          {group}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {availableRouteGroups.length === 0 ? (
                    <p className="text-xs text-muted-foreground">No route groups defined for this tenant</p>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      If selected, the routing strategy will choose models only from this route group. If None, it will use all allowed models.
                    </p>
                  )}
                </>
              )}
            </div>

            {/* Route Groups */}
            <div className="space-y-2">
              <Label>Route Groups</Label>
              {loadingCatalogs ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Loading route groups...
                </div>
              ) : (
                <>
                  <div className="flex flex-wrap gap-2 mb-2">
                    {selectedRouteGroups.map((group) => (
                      <Badge key={group} variant="secondary" className="gap-1">
                        {group}
                        <button
                          type="button"
                          onClick={() => toggleRouteGroup(group)}
                          className="ml-1 hover:text-destructive"
                        >
                          <X className="h-3 w-3" />
                        </button>
                      </Badge>
                    ))}
                  </div>
                  <Select onValueChange={(value) => !selectedRouteGroups.includes(value) && toggleRouteGroup(value)}>
                    <SelectTrigger className="w-full">
                      <SelectValue placeholder="Add route group..." />
                    </SelectTrigger>
                    <SelectContent>
                      {availableRouteGroups
                        .filter((g) => !selectedRouteGroups.includes(g))
                        .map((group) => (
                          <SelectItem key={group} value={group}>
                            {group}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                </>
              )}
            </div>

            {/* Allowed Models */}
            <div className="space-y-2">
              <Label>Allowed Models</Label>
              {loadingCatalogs ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Loading models...
                </div>
              ) : (
                <>
                  <div className="flex flex-wrap gap-2 mb-2">
                    {selectedModels.map((model) => (
                      <Badge key={model} variant="secondary" className="gap-1">
                        {model}
                        <button
                          type="button"
                          onClick={() => toggleModel(model)}
                          className="ml-1 hover:text-destructive"
                        >
                          <X className="h-3 w-3" />
                        </button>
                      </Badge>
                    ))}
                  </div>
                  <Select onValueChange={(value) => !selectedModels.includes(value) && toggleModel(value)}>
                    <SelectTrigger className="w-full">
                      <SelectValue placeholder="Add model..." />
                    </SelectTrigger>
                    <SelectContent>
                      {models
                        .filter((m) => !selectedModels.includes(m))
                        .map((model) => (
                          <SelectItem key={model} value={model}>
                            {model}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                </>
              )}
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
            <Button type="submit" disabled={updateConfig.isPending}>
              {updateConfig.isPending ? 'Saving...' : 'Save Changes'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
