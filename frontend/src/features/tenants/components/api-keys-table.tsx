'use client'

import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { RotateCw, XCircle } from 'lucide-react'
import { type TenantApiKey } from '../api/use-tenants'
import { RotateApiKeyDialog } from './rotate-api-key-dialog'
import { RevokeApiKeyDialog } from './revoke-api-key-dialog'

interface ApiKeysTableProps {
  apiKeys: TenantApiKey[]
  tenantId: string
}

export function ApiKeysTable({ apiKeys, tenantId }: ApiKeysTableProps) {
  const [rotateDialog, setRotateDialog] = useState<{ keyId: string; keyName: string } | null>(null)
  const [rotateDialogOpen, setRotateDialogOpen] = useState(false)
  const [revokeDialog, setRevokeDialog] = useState<{ open: boolean; keyId: string; keyName: string } | null>(null)

  const getStatus = (key: TenantApiKey): 'active' | 'revoked' | 'expired' => {
    if (key.revoked_at) return 'revoked'
    if (key.expires_at && new Date(key.expires_at) < new Date()) return 'expired'
    return 'active'
  }

  const formatDate = (dateString?: string) => {
    if (!dateString) return 'Never'
    const date = new Date(dateString)
    const now = new Date()
    const diffMs = now.getTime() - date.getTime()
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMs / 3600000)
    const diffDays = Math.floor(diffMs / 86400000)

    if (diffMins < 1) return 'Just now'
    if (diffMins < 60) return `${diffMins} min ago`
    if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`
    if (diffDays < 7) return `${diffDays} day${diffDays > 1 ? 's' : ''} ago`
    return date.toLocaleDateString()
  }

  const formatExpiration = (dateString?: string | null) => {
    if (!dateString) return 'Never'
    return new Date(dateString).toLocaleDateString()
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Prefix</TableHead>
            <TableHead>Scopes</TableHead>
            <TableHead>Created</TableHead>
            <TableHead>Last Used</TableHead>
            <TableHead>Expires</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {apiKeys.map((key) => {
            const status = getStatus(key)
            const keyId = (key as { id?: string }).id || ''
            
            return (
              <TableRow key={keyId}>
                <TableCell className="font-medium">{key.name}</TableCell>
                <TableCell>
                  <code className="text-xs bg-muted px-2 py-1 rounded">
                    {key.prefix}***
                  </code>
                </TableCell>
                <TableCell>
                  {key.scopes && key.scopes.length > 0 ? (
                    <div className="flex flex-wrap gap-1">
                      {key.scopes.map((scope) => (
                        <Badge key={scope} variant="outline" className="text-xs">
                          {scope}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <span className="text-sm text-muted-foreground">All</span>
                  )}
                </TableCell>
                <TableCell className="text-sm">{formatDate(key.created_at)}</TableCell>
                <TableCell className="text-sm">{formatDate(key.last_used_at)}</TableCell>
                <TableCell className="text-sm">{formatExpiration(key.expires_at)}</TableCell>
                <TableCell>
                  <Badge
                    variant={
                      status === 'active'
                        ? 'default'
                        : status === 'revoked'
                        ? 'destructive'
                        : 'secondary'
                    }
                  >
                    {status}
                  </Badge>
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-2">
                    {status === 'active' && (
                      <>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => {
                            setRotateDialog({ keyId, keyName: key.name })
                            setRotateDialogOpen(true)
                          }}
                        >
                          <RotateCw className="h-4 w-4 mr-1" />
                          Rotate
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() =>
                            setRevokeDialog({ open: true, keyId, keyName: key.name })
                          }
                        >
                          <XCircle className="h-4 w-4 mr-1" />
                          Revoke
                        </Button>
                      </>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>

      {rotateDialog && (
        <RotateApiKeyDialog
          open={rotateDialogOpen}
          onOpenChange={setRotateDialogOpen}
          onDone={() => { setRotateDialog(null); setRotateDialogOpen(false) }}
          tenantId={tenantId}
          keyId={rotateDialog.keyId}
          keyName={rotateDialog.keyName}
        />
      )}

      {revokeDialog && (
        <RevokeApiKeyDialog
          open={revokeDialog.open}
          onOpenChange={(open) => !open && setRevokeDialog(null)}
          tenantId={tenantId}
          keyId={revokeDialog.keyId}
          keyName={revokeDialog.keyName}
        />
      )}
    </>
  )
}
