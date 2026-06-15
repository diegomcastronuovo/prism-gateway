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
import { X } from 'lucide-react'
import { useCreateRouteGroup } from '../api/use-route-groups'

interface CreateRouteGroupDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  models: string[]
  tenantId: string
}

export function CreateRouteGroupDialog({
  open,
  onOpenChange,
  models,
  tenantId,
}: CreateRouteGroupDialogProps) {
  const createRouteGroup = useCreateRouteGroup(tenantId)
  const [id, setId] = useState('')
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    if (!id.trim()) {
      setError('Route group ID is required')
      return
    }

    try {
      await createRouteGroup.mutateAsync({
        id: id.trim(),
        models: selectedModels,
      })
      setId('')
      setSelectedModels([])
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Create Route Group</DialogTitle>
          <DialogDescription>
            Create a new route group with member models
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="id">
              Route Group ID <span className="text-destructive">*</span>
            </Label>
            <Input
              id="id"
              placeholder="e.g., cheap, premium, fast"
              value={id}
              onChange={(e) => setId(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Unique identifier for the route group
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

          {error && (
            <p className="text-sm text-destructive">{error}</p>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={createRouteGroup.isPending}>
              {createRouteGroup.isPending ? 'Creating...' : 'Create Route Group'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
