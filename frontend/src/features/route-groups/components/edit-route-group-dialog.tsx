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
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { X } from 'lucide-react'
import { useUpdateRouteGroup, type RouteGroup } from '../api/use-route-groups'

interface EditRouteGroupDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  routeGroup: RouteGroup | null
  models: string[]
  tenantId: string
}

export function EditRouteGroupDialog({
  open,
  onOpenChange,
  routeGroup,
  models,
  tenantId,
}: EditRouteGroupDialogProps) {
  const updateRouteGroup = useUpdateRouteGroup(tenantId)
  const [selectedModels, setSelectedModels] = useState<string[]>([])

  // Load current values when dialog opens
  useEffect(() => {
    if (open && routeGroup) {
      setSelectedModels(routeGroup.models || [])
    }
  }, [open, routeGroup])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!routeGroup) return

    const version = routeGroup.version || 1

    try {
      await updateRouteGroup.mutateAsync({
        routeGroupId: routeGroup.id,
        routeGroup: { models: selectedModels },
        version,
      })
      onOpenChange(false)
    } catch {
      // Error handled by mutation
    }
  }

  const toggleModel = (model: string) => {
    setSelectedModels((prev) =>
      prev.includes(model) ? prev.filter((m) => m !== model) : [...prev, model]
    )
  }

  if (!routeGroup) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Edit Route Group</DialogTitle>
          <DialogDescription>
            Update {routeGroup.id} member models
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label>Route Group ID</Label>
            <p className="text-sm font-medium">{routeGroup.id}</p>
            <p className="text-xs text-muted-foreground">
              Route group ID cannot be changed
            </p>
          </div>

          <div className="space-y-2">
            <Label>Member Models</Label>
            <p className="text-xs text-muted-foreground mb-2">
              Select models that belong to this group
            </p>
            <div className="flex flex-wrap gap-2 max-h-40 overflow-y-auto p-2 border rounded-md">
              {models.length === 0 ? (
                <span className="text-sm text-muted-foreground">
                  No models available
                </span>
              ) : (
                models.map((model) => (
                  <Badge
                    key={model}
                    variant={selectedModels.includes(model) ? 'default' : 'outline'}
                    className="cursor-pointer"
                    onClick={() => toggleModel(model)}
                  >
                    {selectedModels.includes(model) && (
                      <X className="h-3 w-3 mr-1" />
                    )}
                    {model}
                  </Badge>
                ))
              )}
            </div>
            {selectedModels.length > 0 && (
              <p className="text-xs text-muted-foreground">
                {selectedModels.length} model{selectedModels.length === 1 ? '' : 's'} selected
              </p>
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={updateRouteGroup.isPending}>
              {updateRouteGroup.isPending ? 'Saving...' : 'Save Changes'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
