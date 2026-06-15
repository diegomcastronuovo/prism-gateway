'use client'

import { useState, useEffect, useMemo, type ComponentProps } from 'react'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard as BaseSectionCard } from '@/components/shared/section-card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils/cn'
import { CheckCircle2, XCircle, Clock, ShieldAlert } from 'lucide-react'
import { useAuth } from '@/hooks/use-auth'
import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { useGlobalConfig, useUpdateGlobalConfig } from '@/features/global-config/api/use-global-config'
import { EmptyState } from '@/components/shared/empty-state'
import { Skeleton } from '@/components/ui/skeleton'

const SectionCard = (props: ComponentProps<typeof BaseSectionCard>) => (
  <BaseSectionCard
    {...props}
    className={cn('border-t-4 border-t-slate-500', props.className)}
  />
)

// SPEC_63: runtime endpoint storage
const NEW_STORAGE_KEY = 'router_api_url'
const LEGACY_STORAGE_KEY = 'gateway_api_endpoint'
const DEFAULT_ENDPOINT = process.env.NEXT_PUBLIC_API_BASE_URL || process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8000'
const ENDPOINT_COOKIE_KEY = 'router_api_url'

function normalizeEndpoint(raw: string): string {
  const value = raw.trim().replace(/\/+$/, '')
  if (!value) return ''
  if (/^\d+$/.test(value)) {
    return `http://localhost:${value}`
  }
  if (value.startsWith('localhost:') || value.startsWith('127.0.0.1:')) {
    return `http://${value}`
  }
  if (!/^https?:\/\//i.test(value)) {
    return `http://${value}`
  }
  return value
}

type ConnectionStatus = 'untested' | 'testing' | 'success' | 'error'

interface ConnectionState {
  status: ConnectionStatus
  lastTestAt?: string
  responseTimeMs?: number
  errorMessage?: string
}

function SettingsContent() {
  const { user, isRefreshingSession } = useAuth()
  const [apiEndpoint, setApiEndpoint] = useState('')
  const [connectionState, setConnectionState] = useState<ConnectionState>({ status: 'untested' })
  const [hasChanges, setHasChanges] = useState(false)

  // SPEC_79: Keycloak form state (prefilled from global config)
  const globalConfigQuery = useGlobalConfig(!!user)
  const globalConfigData = globalConfigQuery.isError ? undefined : globalConfigQuery.data
  const updateGlobalConfig = useUpdateGlobalConfig()

  const accessDeniedStatus =
    globalConfigQuery.error instanceof Error
      ? (globalConfigQuery.error as Error & { status?: number }).status
      : undefined
  const isAccessDenied = accessDeniedStatus === 403 || accessDeniedStatus === 401

  const authInitial = useMemo(() => {
    const cfg = (globalConfigData?.config || {}) as Record<string, any>
    const auth = (cfg.auth || {}) as Record<string, any>
    const jwt = (auth.jwt || {}) as Record<string, any>
    const rclaims = (jwt.required_claims || {}) as Record<string, any>
    const rbac = (jwt.rbac || {}) as Record<string, any>
    return {
      mode: (auth.mode as string) || 'both',
      issuer: (jwt.issuer as string) || 'http://localhost:8080/realms/router',
      audience: (jwt.audience as string) || 'account',
      jwks_url:
        (jwt.jwks_url as string) ||
        'http://host.docker.internal:8080/realms/router/protocol/openid-connect/certs',
      tenant_claim: (rclaims.tenant_id as string) || 'tenant_id',
      roles_claim: (rclaims.roles as string) || 'roles',
      user_roles: (rbac.user_roles as string[] | undefined) || ['user'],
      admin_roles: (rbac.admin_roles as string[] | undefined) || ['admin'],
      local_admin_roles: (rbac.local_admin_roles as string[] | undefined) || ['local_admin'],
      finance_roles: (rbac.finance_roles as string[] | undefined) || ['finance'],
      auditor_roles: (rbac.auditor_roles as string[] | undefined) || ['audit'],
    }
  }, [globalConfigData])

  const [authMode, setAuthMode] = useState<string>('both')
  const [issuer, setIssuer] = useState<string>('')
  const [audience, setAudience] = useState<string>('')
  const [jwksUrl, setJwksUrl] = useState<string>('')
  const [tenantClaim, setTenantClaim] = useState<string>('tenant_id')
  const [rolesClaim, setRolesClaim] = useState<string>('roles')
  const [userRoles, setUserRoles] = useState<string>('user')
  const [adminRoles, setAdminRoles] = useState<string>('admin')
  const [localAdminRoles, setLocalAdminRoles] = useState<string>('local_admin')
  const [financeRoles, setFinanceRoles] = useState<string>('finance')
  const [auditorRoles, setAuditorRoles] = useState<string>('audit')
  const [testStatus, setTestStatus] = useState<'idle' | 'testing' | 'success' | 'error'>('idle')
  const [testMessage, setTestMessage] = useState<string>('')

  useEffect(() => {
    // Prefill from loaded config
    if (!globalConfigData) return
    setAuthMode(authInitial.mode)
    setIssuer(authInitial.issuer)
    setAudience(authInitial.audience)
    setJwksUrl(authInitial.jwks_url)
    setTenantClaim(authInitial.tenant_claim)
    setRolesClaim(authInitial.roles_claim)
    setUserRoles(authInitial.user_roles.join(','))
    setAdminRoles(authInitial.admin_roles.join(','))
    setLocalAdminRoles(authInitial.local_admin_roles.join(','))
    setFinanceRoles(authInitial.finance_roles.join(','))
    setAuditorRoles(authInitial.auditor_roles.join(','))
  }, [globalConfigData, authInitial])

  useEffect(() => {
    // Prefer new key, fall back to legacy
    const saved = localStorage.getItem(NEW_STORAGE_KEY) || localStorage.getItem(LEGACY_STORAGE_KEY)
    const normalized = normalizeEndpoint(saved || DEFAULT_ENDPOINT)
    setApiEndpoint(normalized)
  }, [])

  const handleEndpointChange = (value: string) => {
    setApiEndpoint(value)
    setHasChanges(true)
  }

  const handleSave = () => {
    const normalized = normalizeEndpoint(apiEndpoint)
    if (!normalized) return
    // Save to new key and legacy for backward compatibility
    localStorage.setItem(NEW_STORAGE_KEY, normalized)
    localStorage.setItem(LEGACY_STORAGE_KEY, normalized)
    document.cookie = `${ENDPOINT_COOKIE_KEY}=${encodeURIComponent(normalized)}; path=/; max-age=31536000; samesite=lax`
    setApiEndpoint(normalized)
    setHasChanges(false)
    // Reload app so all services reinitialize with new endpoint
    if (typeof window !== 'undefined') {
      window.location.reload()
    }
  }

  const handleReset = () => {
    const normalizedDefault = normalizeEndpoint(DEFAULT_ENDPOINT)
    setApiEndpoint(normalizedDefault)
    localStorage.setItem(NEW_STORAGE_KEY, normalizedDefault)
    localStorage.setItem(LEGACY_STORAGE_KEY, normalizedDefault)
    document.cookie = `${ENDPOINT_COOKIE_KEY}=${encodeURIComponent(normalizedDefault)}; path=/; max-age=31536000; samesite=lax`
    setHasChanges(false)
    setConnectionState({ status: 'untested' })
    if (typeof window !== 'undefined') {
      window.location.reload()
    }
  }

  const handleTestConnection = async () => {
    const normalized = normalizeEndpoint(apiEndpoint)
    setConnectionState({ status: 'testing' })
    const startTime = Date.now()
    try {
      const res = await fetch(`/api/settings/test-connection?endpoint=${encodeURIComponent(normalized)}`)
      const responseTimeMs = Date.now() - startTime
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        setConnectionState({
          status: 'error',
          lastTestAt: new Date().toISOString(),
          errorMessage: err?.error || `HTTP ${res.status}`,
        })
        return
      }
      const data = await res.json()
      if (data?.ok) {
        setConnectionState({
          status: 'success',
          lastTestAt: new Date().toISOString(),
          responseTimeMs: data?.responseTimeMs ?? responseTimeMs,
        })
      } else {
        setConnectionState({
          status: 'error',
          lastTestAt: new Date().toISOString(),
          errorMessage: data?.error || 'Unavailable',
        })
      }
    } catch (error) {
      setConnectionState({
        status: 'error',
        lastTestAt: new Date().toISOString(),
        errorMessage: error instanceof Error ? error.message : 'Network error',
      })
    }
  }

  const formatTimestamp = (iso?: string) => {
    if (!iso) return '—'
    const date = new Date(iso)
    return date.toLocaleString()
  }

  if (user && globalConfigQuery.isError) {
    return (
      <div>
        <PageHeader
          title="Settings"
          description="Console connection and admin preferences"
        />
        <SectionCard title={isAccessDenied ? 'Access limited' : 'Error'}>
          {isAccessDenied ? (
            <EmptyState
              icon={ShieldAlert}
              title="Insufficient permissions"
              description="Your role cannot view this page. This page only shows data when the gateway allows access to this global area."
            />
          ) : (
            <div className="text-center py-8">
              <p className="text-destructive mb-2">Failed to load global configuration</p>
              <p className="text-sm text-muted-foreground">
                {globalConfigQuery.error instanceof Error
                  ? globalConfigQuery.error.message
                  : 'Unknown error'}
              </p>
            </div>
          )}
        </SectionCard>
      </div>
    )
  }

  if (user && (globalConfigQuery.isLoading || isRefreshingSession)) {
    return (
      <div>
        <PageHeader
          title="Settings"
          description="Console connection and admin preferences"
        />
        <div className="space-y-6">
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-48 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Settings"
        description="Console connection and admin preferences"
      />

      <div className="space-y-6">
        <SectionCard
          title="API Connection"
          description="Configure the backend API endpoint for the admin console"
        >
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="api-endpoint">API Endpoint</Label>
              <Input
                id="api-endpoint"
                placeholder="http://localhost:8080"
                value={apiEndpoint}
                onChange={(e) => handleEndpointChange(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Used by the admin console to connect to the router backend.
              </p>
            </div>

            <div className="flex gap-2">
              <Button onClick={handleSave} disabled={!hasChanges}>
                Save Changes
              </Button>
              <Button variant="outline" onClick={handleTestConnection} disabled={connectionState.status === 'testing'}>
                {connectionState.status === 'testing' ? 'Testing...' : 'Test Connection'}
              </Button>
              <Button variant="ghost" onClick={handleReset}>
                Reset to Default
              </Button>
            </div>
          </div>
        </SectionCard>

        <SectionCard
          title="Connection Status"
          description="Current backend connectivity status"
        >
          <div className="space-y-3">
            <div className="flex items-center gap-3">
              <span className="text-sm text-muted-foreground">Status:</span>
              {connectionState.status === 'success' && (
                <Badge variant="default" className="gap-1">
                  <CheckCircle2 className="h-3 w-3" />
                  Connected
                </Badge>
              )}
              {connectionState.status === 'error' && (
                <Badge variant="destructive" className="gap-1">
                  <XCircle className="h-3 w-3" />
                  Unavailable
                </Badge>
              )}
              {connectionState.status === 'testing' && (
                <Badge variant="secondary" className="gap-1">
                  <Clock className="h-3 w-3" />
                  Testing...
                </Badge>
              )}
              {connectionState.status === 'untested' && (
                <Badge variant="outline">Not tested yet</Badge>
              )}
            </div>

            {connectionState.lastTestAt && (
              <div className="text-sm">
                <span className="text-muted-foreground">Last checked:</span>{' '}
                <span className="font-medium">{formatTimestamp(connectionState.lastTestAt)}</span>
              </div>
            )}

            {connectionState.responseTimeMs !== undefined && (
              <div className="text-sm">
                <span className="text-muted-foreground">Response time:</span>{' '}
                <span className="font-medium">{connectionState.responseTimeMs} ms</span>
              </div>
            )}

            {connectionState.errorMessage && (
              <div className="text-sm text-destructive">
                {connectionState.errorMessage}
              </div>
            )}

            <div className="text-sm">
              <span className="text-muted-foreground">Current endpoint:</span>{' '}
              <span className="font-mono text-xs">{apiEndpoint || '—'}</span>
            </div>
          </div>
        </SectionCard>

        <SectionCard
          title="Authentication Providers"
          description="Configure admin authentication"
        >
          <div className="space-y-4">
            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between mb-2">
                <h4 className="font-medium">Keycloak</h4>
                <Badge variant="default">Active</Badge>
              </div>
              <p className="text-sm text-muted-foreground mb-4">
                OIDC via Keycloak (JWT). These settings map to the global auth config.
              </p>

              <div className="grid gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="auth-mode">Auth Mode</Label>
                  <select
                    id="auth-mode"
                    value={authMode}
                    onChange={(e) => setAuthMode(e.target.value)}
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  >
                    <option value="jwt">jwt</option>
                    <option value="both">both</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="issuer">JWT Issuer</Label>
                  <Input id="issuer" value={issuer} onChange={(e) => setIssuer(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="audience">JWT Audience</Label>
                  <Input id="audience" value={audience} onChange={(e) => setAudience(e.target.value)} />
                </div>
                <div className="space-y-2 md:col-span-2">
                  <Label htmlFor="jwks">JWKS URL</Label>
                  <Input id="jwks" value={jwksUrl} onChange={(e) => setJwksUrl(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="tenant-claim">Tenant Claim</Label>
                  <Input id="tenant-claim" value={tenantClaim} onChange={(e) => setTenantClaim(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="roles-claim">Roles Claim</Label>
                  <Input id="roles-claim" value={rolesClaim} onChange={(e) => setRolesClaim(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="user-roles">User Roles (comma-separated)</Label>
                  <Input id="user-roles" value={userRoles} onChange={(e) => setUserRoles(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="admin-roles">Admin Roles (comma-separated)</Label>
                  <Input id="admin-roles" value={adminRoles} onChange={(e) => setAdminRoles(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="local-admin-roles">Local Admin Roles (comma-separated)</Label>
                  <Input id="local-admin-roles" value={localAdminRoles} onChange={(e) => setLocalAdminRoles(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="finance-roles">Finance Roles (comma-separated)</Label>
                  <Input id="finance-roles" value={financeRoles} onChange={(e) => setFinanceRoles(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="auditor-roles">Auditor Roles (comma-separated)</Label>
                  <Input id="auditor-roles" value={auditorRoles} onChange={(e) => setAuditorRoles(e.target.value)} />
                </div>
              </div>

              <div className="mt-4 flex gap-2">
                <Button
                  variant="outline"
                  onClick={async () => {
                    setTestStatus('testing')
                    setTestMessage('')
                    try {
                      const res = await fetch('/api/settings/auth/keycloak/test', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ jwks_url: jwksUrl, issuer, audience }),
                      })
                      const data = await res.json().catch(() => ({}))
                      if (res.ok && data?.ok) {
                        setTestStatus('success')
                        setTestMessage(data?.message || 'Connection successful. JWKS endpoint is reachable.')
                      } else {
                        setTestStatus('error')
                        setTestMessage(data?.message || `Failed to connect (HTTP ${res.status})`)
                      }
                    } catch (e) {
                      setTestStatus('error')
                      setTestMessage(e instanceof Error ? e.message : 'Network error')
                    }
                  }}
                  disabled={!jwksUrl || !issuer || !audience || testStatus === 'testing'}
                >
                  {testStatus === 'testing' ? 'Testing…' : 'Test Connection'}
                </Button>

                <Button
                  onClick={() => {
                    if (!globalConfigData) return
                    const latestVersion = globalConfigData.version
                    if (process.env.NODE_ENV !== 'production') {
                      console.log('[GlobalConfig] Settings: sending version:', latestVersion)
                    }
                    const baseCfg = (globalConfigData.config || {}) as Record<string, any>
                    const nextCfg = { ...baseCfg }
                    nextCfg.auth = {
                      mode: authMode,
                      jwt: {
                        issuer,
                        audience,
                        jwks_url: jwksUrl,
                        required_claims: {
                          tenant_id: tenantClaim,
                          roles: rolesClaim,
                        },
                        rbac: {
                          user_roles: userRoles.split(',').map((s) => s.trim()).filter(Boolean),
                          admin_roles: adminRoles.split(',').map((s) => s.trim()).filter(Boolean),
                          local_admin_roles: localAdminRoles.split(',').map((s) => s.trim()).filter(Boolean),
                          finance_roles: financeRoles.split(',').map((s) => s.trim()).filter(Boolean),
                          auditor_roles: auditorRoles.split(',').map((s) => s.trim()).filter(Boolean),
                        },
                      },
                    }
                    updateGlobalConfig.mutate({ config: nextCfg, version: latestVersion })
                  }}
                  disabled={!issuer || !audience || !jwksUrl}
                >
                  Save Keycloak Configuration
                </Button>
              </div>

              {testStatus !== 'idle' && (
                <div className={`mt-3 text-sm ${testStatus === 'success' ? 'text-green-600' : testStatus === 'error' ? 'text-destructive' : 'text-muted-foreground'}`}>
                  {testMessage}
                </div>
              )}
            </div>
            
          </div>
        </SectionCard>
      </div>
    </div>
  )
}

export default function SettingsPage() {
  return (
    <RequireAdminRole allowedRoles={['admin', 'local_admin', 'user']}>
      <SettingsContent />
    </RequireAdminRole>
  )
}
