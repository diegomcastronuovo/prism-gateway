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
import { useDeleteModel, type Model } from '../api/use-models'

interface DeleteModelDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  model: Model | null
  onSuccess?: () => void
}

export function DeleteModelDialog({
  open,
  onOpenChange,
  model,
  onSuccess,
}: DeleteModelDialogProps) {
  const deleteModel = useDeleteModel()

  const handleDelete = async () => {
    if (!model) return

    try {
      await deleteModel.mutateAsync(model.id)
      onSuccess?.()
      onOpenChange(false)
    } catch {
      // Error handled by mutation
    }
  }

  if (!model) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle className="h-5 w-5" />
            Delete Model
          </DialogTitle>
          <DialogDescription className="pt-2">
            Are you sure you want to delete <strong>{model.id}</strong>?
          </DialogDescription>
        </DialogHeader>

        <div className="bg-muted/50 p-3 rounded-lg text-sm space-y-2">
          <p className="font-medium">This action may affect:</p>
          <ul className="list-disc list-inside text-muted-foreground space-y-1">
            <li>Routing decisions</li>
            <li>Route group assignments</li>
            <li>Tenant configurations that reference this model</li>
          </ul>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={handleDelete}
            disabled={deleteModel.isPending}
          >
            {deleteModel.isPending ? 'Deleting...' : 'Delete Model'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
