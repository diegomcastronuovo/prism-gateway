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
import { useRotateTenantApiKey } from '../api/use-tenants'
import { ApiKeySecretModal } from './api-key-secret-modal'

interface RotateApiKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone: () => void
  tenantId: string
  keyId: string
  keyName: string
}

export function RotateApiKeyDialog({
  open,
  onOpenChange,
  onDone,
  tenantId,
  keyId,
  keyName,
}: RotateApiKeyDialogProps) {
  const rotateMutation = useRotateTenantApiKey()
  const [secretModalOpen, setSecretModalOpen] = useState(false)
  const [rotatedApiKey, setRotatedApiKey] = useState('')

  const handleRotate = () => {
    rotateMutation.mutate(
      { tenantId, keyId },
      {
        onSuccess: (data) => {
          const response = data as { new?: { key?: string }; key?: string; api_key?: string }
          const apiKey = response.new?.key || response.key || response.api_key || ''
          // First store the key, then swap dialogs
          setRotatedApiKey(apiKey)
          // Close confirm dialog without destroying this component tree
          onOpenChange(false)
          // Open secret modal — component is still mounted because rotateDialog state
          // is only cleared when secretModal closes
          setSecretModalOpen(true)
        },
      }
    )
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rotate API Key?</DialogTitle>
            <DialogDescription>
              Are you sure you want to rotate the API key <strong>{keyName}</strong>?
              The current key will stop working immediately and a new key will be generated.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => { onOpenChange(false); onDone() }}>
              Cancel
            </Button>
            <Button
              onClick={handleRotate}
              disabled={rotateMutation.isPending}
            >
              {rotateMutation.isPending ? 'Rotating...' : 'Rotate Key'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ApiKeySecretModal
        open={secretModalOpen}
        onOpenChange={(open) => {
          setSecretModalOpen(open)
          if (!open) onDone()
        }}
        apiKey={rotatedApiKey}
        isRotation
      />
    </>
  )
}
