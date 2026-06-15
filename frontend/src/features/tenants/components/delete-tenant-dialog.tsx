'use client'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { useDeleteTenant } from '../api/use-tenants'

interface DeleteTenantDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantId: string | null
  onSuccess?: () => void
}

export function DeleteTenantDialog({
  open,
  onOpenChange,
  tenantId,
  onSuccess,
}: DeleteTenantDialogProps) {
  const deleteTenant = useDeleteTenant()

  const handleDelete = async () => {
    if (!tenantId) return

    try {
      await deleteTenant.mutateAsync(tenantId)
      onOpenChange(false)
      onSuccess?.()
    } catch {
      // Error is handled by the mutation
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete Tenant</AlertDialogTitle>
          <AlertDialogDescription>
            Are you sure you want to delete tenant <strong>{tenantId}</strong>?
            This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleDelete}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            disabled={deleteTenant.isPending}
          >
            {deleteTenant.isPending ? 'Deleting...' : 'Delete'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
