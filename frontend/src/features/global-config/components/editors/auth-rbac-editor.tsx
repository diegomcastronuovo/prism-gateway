'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface AuthRbacEditorProps {
  auth: Record<string, unknown>
  onSave: (updatedAuth: Record<string, unknown>) => void
  onCancel: () => void
}

export function AuthRbacEditor({ auth, onSave, onCancel }: AuthRbacEditorProps) {
  const jwt = auth.jwt as Record<string, unknown> | undefined
  const requiredClaims = jwt?.required_claims as Record<string, unknown> | undefined
  const rbac = jwt?.rbac as Record<string, unknown> | undefined

  const [mode, setMode] = useState<string>(String(auth.mode || 'api_key'))
  const [issuer, setIssuer] = useState<string>(String(jwt?.issuer || ''))
  const [audience, setAudience] = useState<string>(String(jwt?.audience || ''))
  const [jwksUrl, setJwksUrl] = useState<string>(String(jwt?.jwks_url || ''))
  const [tenantIdClaim, setTenantIdClaim] = useState<string>(String(requiredClaims?.tenant_id || 'tenant_id'))
  const [rolesClaim, setRolesClaim] = useState<string>(String(requiredClaims?.roles || 'roles'))
  const [userRoles, setUserRoles] = useState<string>(
    Array.isArray(rbac?.user_roles) ? rbac.user_roles.join(', ') : String(rbac?.user_roles || 'user')
  )
  const [adminRoles, setAdminRoles] = useState<string>(
    Array.isArray(rbac?.admin_roles) ? rbac.admin_roles.join(', ') : String(rbac?.admin_roles || 'admin')
  )
  const [financeRoles, setFinanceRoles] = useState<string>(
    Array.isArray(rbac?.finance_roles) ? rbac.finance_roles.join(', ') : String(rbac?.finance_roles || 'finance')
  )

  const isJwtEnabled = mode === 'jwt' || mode === 'both'

  const handleSave = () => {
    const updatedAuth: Record<string, unknown> = {
      mode,
    }

    if (isJwtEnabled) {
      updatedAuth.jwt = {
        issuer,
        audience,
        jwks_url: jwksUrl,
        required_claims: {
          tenant_id: tenantIdClaim,
          roles: rolesClaim,
        },
        rbac: {
          user_roles: userRoles.split(',').map(r => r.trim()).filter(r => r.length > 0),
          admin_roles: adminRoles.split(',').map(r => r.trim()).filter(r => r.length > 0),
          finance_roles: financeRoles.split(',').map(r => r.trim()).filter(r => r.length > 0),
        },
      }
    }

    onSave(updatedAuth)
  }

  return (
    <div className="space-y-6">
      {/* Authentication Mode */}
      <div className="p-4 rounded-lg border bg-card space-y-3">
        <h3 className="font-semibold">Authentication Mode</h3>
        <div className="space-y-1">
          <Label htmlFor="auth-mode" className="text-xs">
            Mode
          </Label>
          <select
            id="auth-mode"
            value={mode}
            onChange={(e) => setMode(e.target.value)}
            className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="api_key">API Key only</option>
            <option value="jwt">JWT only</option>
            <option value="both">Both</option>
          </select>
        </div>
      </div>

      {/* JWT Configuration */}
      {isJwtEnabled && (
        <>
          <div className="p-4 rounded-lg border bg-card space-y-3">
            <h3 className="font-semibold">JWT Configuration</h3>
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="jwt-issuer" className="text-xs">
                  Issuer *
                </Label>
                <Input
                  id="jwt-issuer"
                  value={issuer}
                  onChange={(e) => setIssuer(e.target.value)}
                  className="h-9 text-sm"
                  placeholder="dev"
                  required
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="jwt-audience" className="text-xs">
                  Audience *
                </Label>
                <Input
                  id="jwt-audience"
                  value={audience}
                  onChange={(e) => setAudience(e.target.value)}
                  className="h-9 text-sm"
                  placeholder="router"
                  required
                />
              </div>
            </div>
            <div className="space-y-1">
              <Label htmlFor="jwt-jwks-url" className="text-xs">
                JWKS URL *
              </Label>
              <Input
                id="jwt-jwks-url"
                type="url"
                value={jwksUrl}
                onChange={(e) => setJwksUrl(e.target.value)}
                className="h-9 text-sm font-mono"
                placeholder="http://host.docker.internal:9009/.well-known/jwks.json"
                required
              />
            </div>
          </div>

          {/* Required Claims */}
          <div className="p-4 rounded-lg border bg-card space-y-3">
            <h3 className="font-semibold">Required Claims</h3>
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="tenant-id-claim" className="text-xs">
                  Tenant ID Claim
                </Label>
                <Input
                  id="tenant-id-claim"
                  value={tenantIdClaim}
                  onChange={(e) => setTenantIdClaim(e.target.value)}
                  className="h-9 text-sm font-mono"
                  placeholder="tenant_id"
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="roles-claim" className="text-xs">
                  Roles Claim
                </Label>
                <Input
                  id="roles-claim"
                  value={rolesClaim}
                  onChange={(e) => setRolesClaim(e.target.value)}
                  className="h-9 text-sm font-mono"
                  placeholder="roles"
                />
              </div>
            </div>
          </div>

          {/* RBAC Roles */}
          <div className="p-4 rounded-lg border bg-card space-y-3">
            <h3 className="font-semibold">RBAC Roles</h3>
            <p className="text-xs text-muted-foreground">Enter roles separated by commas</p>
            <div className="space-y-3">
              <div className="space-y-1">
                <Label htmlFor="user-roles" className="text-xs">
                  User Roles
                </Label>
                <Input
                  id="user-roles"
                  value={userRoles}
                  onChange={(e) => setUserRoles(e.target.value)}
                  className="h-9 text-sm"
                  placeholder="user, viewer"
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="admin-roles" className="text-xs">
                  Admin Roles
                </Label>
                <Input
                  id="admin-roles"
                  value={adminRoles}
                  onChange={(e) => setAdminRoles(e.target.value)}
                  className="h-9 text-sm"
                  placeholder="admin, superadmin"
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="finance-roles" className="text-xs">
                  Finance Roles
                </Label>
                <Input
                  id="finance-roles"
                  value={financeRoles}
                  onChange={(e) => setFinanceRoles(e.target.value)}
                  className="h-9 text-sm"
                  placeholder="finance, billing"
                />
              </div>
            </div>
          </div>
        </>
      )}

      <div className="flex gap-2 pt-4 border-t">
        <Button onClick={handleSave}>Save Changes</Button>
        <Button variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </div>
  )
}
