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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { X, Plus, Loader2, Trash2 } from 'lucide-react'
import { useUpdateTenantConfig, type TenantConfig } from '../api/use-tenants'

interface TrafficSplitEntry {
  model: string
  weight: number
}

interface TrafficSplitGroup {
  name: string
  entries: TrafficSplitEntry[]
}

interface EditTrafficSplitDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantConfig: TenantConfig | null
}

export function EditTrafficSplitDialog({ open, onOpenChange, tenantConfig }: EditTrafficSplitDialogProps) {
  const updateConfig = useUpdateTenantConfig()
  const [models, setModels] = useState<string[]>([])
  const [loadingModels, setLoadingModels] = useState(false)
  const [groups, setGroups] = useState<TrafficSplitGroup[]>([])
  const [newGroupName, setNewGroupName] = useState('')

  // Load models and current traffic split
  useEffect(() => {
    if (open && tenantConfig) {
      // Load models
      setLoadingModels(true)
      fetch('/api/models')
        .then(r => r.json())
        .then(d => setModels((d.data || []).map((m: { model_id?: string; id?: string }) => m.model_id || m.id || '').filter(Boolean)))
        .finally(() => setLoadingModels(false))

      // Parse current traffic split
      const trafficSplit = tenantConfig.config.traffic_split as Record<string, Array<{ model: string; weight: number }>> | undefined
      if (trafficSplit) {
        const parsedGroups: TrafficSplitGroup[] = Object.entries(trafficSplit).map(([name, entries]) => ({
          name,
          entries: entries.map(e => ({ model: e.model, weight: e.weight })),
        }))
        setGroups(parsedGroups)
      } else {
        setGroups([])
      }
    }
  }, [open, tenantConfig])

  const addGroup = () => {
    if (!newGroupName.trim()) return
    if (groups.some(g => g.name === newGroupName.trim())) return
    setGroups(prev => [...prev, { name: newGroupName.trim(), entries: [] }])
    setNewGroupName('')
  }

  const removeGroup = (name: string) => {
    setGroups(prev => prev.filter(g => g.name !== name))
  }

  const addEntry = (groupName: string) => {
    setGroups(prev =>
      prev.map(g =>
        g.name === groupName
          ? { ...g, entries: [...g.entries, { model: '', weight: 0 }] }
          : g
      )
    )
  }

  const removeEntry = (groupName: string, index: number) => {
    setGroups(prev =>
      prev.map(g =>
        g.name === groupName
          ? { ...g, entries: g.entries.filter((_, i) => i !== index) }
          : g
      )
    )
  }

  const updateEntry = (groupName: string, index: number, field: keyof TrafficSplitEntry, value: string | number) => {
    setGroups(prev =>
      prev.map(g =>
        g.name === groupName
          ? {
              ...g,
              entries: g.entries.map((e, i) =>
                i === index ? { ...e, [field]: value } : e
              ),
            }
          : g
      )
    )
  }

  const getTotalWeight = (entries: TrafficSplitEntry[]) => {
    return entries.reduce((sum, e) => sum + (Number(e.weight) || 0), 0)
  }

  const onSubmit = async () => {
    if (!tenantConfig) return

    try {
      // Validate
      for (const group of groups) {
        if (group.entries.length === 0) {
          throw new Error(`Group "${group.name}" has no models`)
        }
        const totalWeight = getTotalWeight(group.entries)
        if (totalWeight <= 0) {
          throw new Error(`Group "${group.name}" must have total weight > 0`)
        }
        const models = group.entries.map(e => e.model)
        if (new Set(models).size !== models.length) {
          throw new Error(`Group "${group.name}" has duplicate models`)
        }
      }

      // Build patch
      const trafficSplit: Record<string, Array<{ model: string; weight: number }>> = {}
      for (const group of groups) {
        trafficSplit[group.name] = group.entries.map(e => ({
          model: e.model,
          weight: Number(e.weight),
        }))
      }

      await updateConfig.mutateAsync({
        tenantId: tenantConfig.tenant_id,
        version: tenantConfig.version,
        patch: { traffic_split: trafficSplit },
      })
      onOpenChange(false)
    } catch (error) {
      // Error is handled by the mutation or shown as validation error
      if (error instanceof Error) {
        // Show validation error toast would go here
        console.error('Validation error:', error.message)
      }
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Edit Traffic Split</DialogTitle>
          <DialogDescription>
            Configure traffic split groups for {tenantConfig?.tenant_id}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-6 py-4">
          {/* Add new group */}
          <div className="flex gap-2">
            <Input
              placeholder="New split key name..."
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && addGroup()}
            />
            <Button type="button" onClick={addGroup} disabled={!newGroupName.trim()}>
              <Plus className="h-4 w-4 mr-1" />
              Add
            </Button>
          </div>

          {/* Groups */}
          <div className="space-y-4">
            {groups.length === 0 && (
              <p className="text-sm text-muted-foreground text-center py-4">
                No traffic split configured. Add a split key to get started.
              </p>
            )}

            {groups.map((group) => {
              const totalWeight = getTotalWeight(group.entries)
              return (
                <div key={group.name} className="rounded-lg border p-4 space-y-3">
                  <div className="flex items-center justify-between">
                    <h4 className="font-medium">{group.name}</h4>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => removeGroup(group.name)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>

                  <div className="space-y-2">
                    {group.entries.map((entry, index) => (
                      <div key={index} className="flex items-center gap-2">
                        <Select
                          value={entry.model}
                          onValueChange={(value) => updateEntry(group.name, index, 'model', value)}
                        >
                          <SelectTrigger className="flex-1">
                            <SelectValue placeholder="Select model" />
                          </SelectTrigger>
                          <SelectContent>
                            {loadingModels ? (
                              <div className="flex items-center gap-2 p-2 text-sm text-muted-foreground">
                                <Loader2 className="h-4 w-4 animate-spin" />
                                Loading...
                              </div>
                            ) : (
                              models
                                .filter(m => m === entry.model || !group.entries.some(e => e.model === m))
                                .map((model) => (
                                  <SelectItem key={model} value={model}>
                                    {model}
                                  </SelectItem>
                                ))
                            )}
                          </SelectContent>
                        </Select>
                        <Input
                          type="number"
                          min={0}
                          placeholder="Weight"
                          value={entry.weight || ''}
                          onChange={(e) => updateEntry(group.name, index, 'weight', parseInt(e.target.value) || 0)}
                          className="w-24"
                        />
                        <span className="text-sm text-muted-foreground w-12">%</span>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => removeEntry(group.name, index)}
                        >
                          <X className="h-4 w-4" />
                        </Button>
                      </div>
                    ))}
                  </div>

                  <div className="flex items-center justify-between pt-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => addEntry(group.name)}
                      disabled={loadingModels}
                    >
                      <Plus className="h-4 w-4 mr-1" />
                      Add Model
                    </Button>
                    {group.entries.length > 0 && (
                      <div className="text-sm">
                        <span className="text-muted-foreground">Total: </span>
                        <Badge variant={totalWeight > 0 ? 'default' : 'destructive'}>
                          {totalWeight}%
                        </Badge>
                      </div>
                    )}
                  </div>
                </div>
              )
            })}
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
