'use client'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { AlertTriangle } from 'lucide-react'
import { useDeleteRouteGroup, type RouteGroup } from '../api/use-route-groups'

interface DeleteRouteGroupDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  routeGroup: RouteGroup | null
  tenantId: string
}

export function DeleteRouteGroupDialog({
  open,
  onOpenChange,
  routeGroup,
  tenantId,
}: DeleteRouteGroupDialogProps) {
  const deleteRouteGroup = useDeleteRouteGroup(tenantId)

  const handleDelete = async () => {
    if (!routeGroup) return

    try {
      await deleteRouteGroup.mutateAsync(routeGroup.id)
      onOpenChange(false)
    } catch {
      // Error handled by mutation
    }
  }

  if (!routeGroup) return null

  const models = routeGroup.models || []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle className="h-5 w-5" />
            Delete Route Group
          </DialogTitle>
          <DialogDescription className="space-y-2">
            <p>
              Are you sure you want to delete the route group{' '}
              <strong>{routeGroup.id}</strong>?
            </p>
            <p>
              This route group contains {models.length} model
              {models.length === 1 ? '' : 's'}:
            </p>
            {models.length > 0 && (
              <ul className="list-disc list-inside text-sm text-muted-foreground">
                {models.slice(0, 5).map((model) => (
                  <li key={model}>{model}</li>
                ))}
                {models.length > 5 && (
                  <li>...and {models.length - 5} more</li>
                )}
              </ul>
            )}
            <p className="text-sm text-destructive">
              This action cannot be undone. If this route group is still
              referenced by tenants or routing policies, the backend will
              reject the deletion.
            </p>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={handleDelete}
            disabled={deleteRouteGroup.isPending}
          >
            {deleteRouteGroup.isPending ? 'Deleting...' : 'Delete Route Group'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
