'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/shared/empty-state'
import { Key, Plus } from 'lucide-react'
import { type TenantApiKey } from '../api/use-tenants'
import { ApiKeysTable } from './api-keys-table'
import { CreateApiKeyDialog } from './create-api-key-dialog'
import { ApiKeySecretModal } from './api-key-secret-modal'

interface TenantAccessSectionProps {
  tenantId: string
  apiKeys: TenantApiKey[] | undefined
  isLoading: boolean
  error: Error | null
}

export function TenantAccessSection({ tenantId, apiKeys, isLoading, error }: TenantAccessSectionProps) {
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [secretModal, setSecretModal] = useState<{ open: boolean; apiKey: string; isRotation: boolean } | null>(null)

  const handleCreateSuccess = (apiKey: string) => {
    console.log('handleCreateSuccess called with apiKey:', apiKey)
    console.log('apiKey type:', typeof apiKey)
    console.log('apiKey length:', apiKey?.length)
    setSecretModal({ open: true, apiKey, isRotation: false })
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold">API Keys</h3>
        <Button size="sm" onClick={() => setCreateDialogOpen(true)}>
          <Plus className="h-4 w-4 mr-2" />
          Create API Key
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : error ? (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">Failed to load API keys</p>
          <p className="text-xs text-muted-foreground mt-1">{error.message}</p>
        </div>
      ) : !apiKeys || apiKeys.length === 0 ? (
        <EmptyState
          icon={Key}
          title="No API keys yet"
          description="Create your first API key to allow applications to call the gateway"
        />
      ) : (
        <ApiKeysTable
          apiKeys={apiKeys}
          tenantId={tenantId}
        />
      )}

      <CreateApiKeyDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        tenantId={tenantId}
        onSuccess={handleCreateSuccess}
      />

      {secretModal && (
        <ApiKeySecretModal
          open={secretModal.open}
          onOpenChange={(open) => !open && setSecretModal(null)}
          apiKey={secretModal.apiKey}
          isRotation={secretModal.isRotation}
        />
      )}
    </div>
  )
}
