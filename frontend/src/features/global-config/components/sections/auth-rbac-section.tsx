'use client'

import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Edit } from 'lucide-react'
import { AuthRbacEditor } from '../editors/auth-rbac-editor'

interface AuthRbacSectionProps {
  config: Record<string, unknown>
  onUpdate?: (updatedAuth: Record<string, unknown>) => void
}

export function AuthRbacSection({ config, onUpdate }: AuthRbacSectionProps) {
  const [isEditing, setIsEditing] = useState(false)
  const auth = config.auth as Record<string, unknown> | undefined
  const jwt = auth?.jwt as Record<string, unknown> | undefined
  const requiredClaims = jwt?.required_claims as Record<string, unknown> | undefined
  const rbac = jwt?.rbac as Record<string, unknown> | undefined

  const handleSave = (updatedAuth: Record<string, unknown>) => {
    if (onUpdate) {
      onUpdate(updatedAuth)
    }
    setIsEditing(false)
  }

  const handleCancel = () => {
    setIsEditing(false)
  }

  if (!auth) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>No authentication configuration</p>
      </div>
    )
  }

  if (isEditing) {
    return (
      <AuthRbacEditor
        auth={auth}
        onSave={handleSave}
        onCancel={handleCancel}
      />
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setIsEditing(true)}>
          <Edit className="mr-2 h-4 w-4" />
          Edit Authentication
        </Button>
      </div>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {auth?.mode !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">Auth Mode</span>
            <Badge variant="outline" className="w-fit">{String(auth.mode)}</Badge>
          </div>
        )}

        {jwt?.issuer !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">JWT Issuer</span>
            <span className="font-medium text-sm break-all">{String(jwt.issuer)}</span>
          </div>
        )}

        {jwt?.audience !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">JWT Audience</span>
            <span className="font-medium text-sm break-all">{String(jwt.audience)}</span>
          </div>
        )}
      </div>

      {jwt?.jwks_url !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">JWKS URL</span>
          <span className="font-medium text-sm font-mono break-all">{String(jwt.jwks_url)}</span>
        </div>
      )}

      {requiredClaims && Object.keys(requiredClaims).length > 0 && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">Required Claims</span>
          <div className="grid gap-2 md:grid-cols-2">
            {requiredClaims.tenant_id !== undefined && (
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">Tenant ID:</span>
                <span className="font-medium text-sm font-mono">{String(requiredClaims.tenant_id)}</span>
              </div>
            )}
            {requiredClaims.roles !== undefined && (
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">Roles:</span>
                <span className="font-medium text-sm font-mono">{String(requiredClaims.roles)}</span>
              </div>
            )}
          </div>
        </div>
      )}

      {rbac && (
        <div className="flex flex-col gap-3 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">RBAC Configurable Roles (local_admin and audit roles are internal and not configurable)</span>
          <div className="space-y-3">
            {(() => {
              const roleGroups: Array<{ key: string; label: string }> = [
                { key: 'admin_roles', label: 'Admin Roles' },
                { key: 'local_admin_roles', label: 'Local Admin Roles' },
                { key: 'finance_roles', label: 'Finance Roles' },
                { key: 'audit_roles', label: 'Audit Roles' },
                { key: 'user_roles', label: 'User Roles' },
              ]
              return roleGroups
                .filter(({ key }) => rbac[key] !== undefined)
                .map(({ key, label }) => (
                  <div key={key} className="flex flex-col gap-2">
                    <span className="text-sm font-medium">{label}</span>
                    <div className="flex flex-wrap gap-2">
                      {Array.isArray(rbac[key]) ? (
                        (rbac[key] as unknown[]).map((role, idx) => (
                          <Badge key={idx} variant="secondary">{String(role)}</Badge>
                        ))
                      ) : (
                        <Badge variant="secondary">{String(rbac[key])}</Badge>
                      )}
                    </div>
                  </div>
                ))
            })()}
          </div>
        </div>
      )}
    </div>
  )
}
