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
import { useRevokeTenantApiKey } from '../api/use-tenants'

interface RevokeApiKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantId: string
  keyId: string
  keyName: string
}

export function RevokeApiKeyDialog({
  open,
  onOpenChange,
  tenantId,
  keyId,
  keyName,
}: RevokeApiKeyDialogProps) {
  const revokeMutation = useRevokeTenantApiKey()

  const handleRevoke = () => {
    revokeMutation.mutate(
      { tenantId, keyId },
      {
        onSuccess: () => {
          onOpenChange(false)
        },
      }
    )
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Revoke API Key?</AlertDialogTitle>
          <AlertDialogDescription>
            Are you sure you want to revoke the API key <strong>{keyName}</strong>?
            <br />
            <br />
            This key will stop working immediately and cannot be recovered.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleRevoke}
            disabled={revokeMutation.isPending}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {revokeMutation.isPending ? 'Revoking...' : 'Revoke Key'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
