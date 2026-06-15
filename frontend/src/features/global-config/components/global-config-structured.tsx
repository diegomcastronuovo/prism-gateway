import { SectionCard } from '@/components/shared/section-card'
import { VersionStatusSection } from './sections/version-status-section'
import { ProvidersSection } from './sections/providers-section'
import { ModelsSection } from './sections/models-section'
import { RoutingInfraSection } from './sections/routing-infra-section'
import { AuthRbacSection } from './sections/auth-rbac-section'
import { DecisionOpsSection } from './sections/decision-ops-section'
import { useUpdateGlobalConfig, type GlobalConfig } from '../api/use-global-config'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '@/components/ui/sheet'
import { useQueryClient } from '@tanstack/react-query'
import { getLatestGlobalConfigRecord } from '@/features/global-config/lib/global-config-merge-base'

interface GlobalConfigStructuredProps {
  config: Record<string, unknown>
  version: number
}

export function GlobalConfigStructured({ config, version }: GlobalConfigStructuredProps) {
  const hasBenchmarking = config.benchmarking !== undefined
  const updateMutation = useUpdateGlobalConfig()
  const queryClient = useQueryClient()
  const [authInfoOpen, setAuthInfoOpen] = useState(false)

  const getLatestVersion = (): number => {
    const cachedData = queryClient.getQueryData<GlobalConfig>(['globalConfig'])
    const latestVersion = cachedData?.version ?? version
    if (process.env.NODE_ENV !== 'production') {
      console.log('[GlobalConfig] sending version:', latestVersion)
      console.log('[GlobalConfig] current query version:', cachedData?.version ?? 'not in cache')
    }
    return latestVersion
  }

  const handleProvidersUpdate = (updatedProviders: Record<string, unknown>) => {
    const base = getLatestGlobalConfigRecord(queryClient, config)
    base.providers = updatedProviders
    updateMutation.mutate({ config: base, version: getLatestVersion() })
  }

  const handleModelsUpdate = (updatedModels: unknown[]) => {
    const base = getLatestGlobalConfigRecord(queryClient, config)
    base.models = updatedModels
    updateMutation.mutate({ config: base, version: getLatestVersion() })
  }

  const handleRoutingInfraUpdate = (updated: Record<string, unknown>) => {
    const base = getLatestGlobalConfigRecord(queryClient, config)
    updateMutation.mutate({ config: { ...base, ...updated }, version: getLatestVersion() })
  }

  const handleDecisionOpsUpdate = (updated: Record<string, unknown>) => {
    const base = getLatestGlobalConfigRecord(queryClient, config)
    updateMutation.mutate({ config: { ...base, ...updated }, version: getLatestVersion() })
  }

  const handleAuthUpdate = (updatedAuth: Record<string, unknown>) => {
    const base = getLatestGlobalConfigRecord(queryClient, config)
    base.auth = updatedAuth
    updateMutation.mutate({ config: base, version: getLatestVersion() })
  }

  return (
    <div className="space-y-6">
      {/* 1. Version / Status Summary */}
      <SectionCard
        title="Configuration Overview"
        description="Active version and platform configuration summary"
        className="border-t-4 border-t-pink-500"
      >
        <VersionStatusSection config={config} version={version} />
      </SectionCard>

      {/* 2. Providers */}
      <SectionCard
        title="Providers"
        description="Configured LLM providers and their credentials"
        className="border-t-4 border-t-cyan-400"
      >
        <ProvidersSection config={config} onUpdate={handleProvidersUpdate} />
      </SectionCard>

      {/* 3. Model Catalog */}
      <SectionCard
        title="Model Catalog"
        description="Platform-wide model definitions, pricing, and mock configuration"
        className="border-t-4 border-t-purple-500"
      >
        <ModelsSection config={config} onUpdate={handleModelsUpdate} />
      </SectionCard>

      {/* 4. Routing Infrastructure */}
      <SectionCard
        title="Routing Infrastructure"
        description="Circuit breaker, rate limiting, and smart routing metrics store"
        className="border-t-4 border-t-amber-400"
      >
        <RoutingInfraSection config={config} onUpdate={handleRoutingInfraUpdate} />
      </SectionCard>

      {/* 5. DecisionOps */}
      <SectionCard
        title="DecisionOps"
        description="Workflow-based routing settings — conversation TTL and tier degradation policy"
        className="border-t-4 border-t-violet-500"
      >
        <DecisionOpsSection config={config} onUpdate={handleDecisionOpsUpdate} />
      </SectionCard>

      {/* 6. Authentication & RBAC */}
      {!!config.auth && (
        <>
          <SectionCard
            title="Authentication & RBAC"
            description="Authentication mode, JWT configuration, and role-based access control"
            className="border-t-4 border-t-emerald-500"
            action={
              <Button size="sm" variant="outline" onClick={() => setAuthInfoOpen(true)}>
                Information
              </Button>
            }
          >
            <AuthRbacSection config={config} onUpdate={handleAuthUpdate} />
          </SectionCard>

          {/* Information Drawer */}
          <Sheet open={authInfoOpen} onOpenChange={setAuthInfoOpen}>
            <SheetContent side="right" className="sm:max-w-[420px] w-full overflow-y-auto">
              <SheetHeader>
                <SheetTitle>Authentication & RBAC Guide</SheetTitle>
                <SheetDescription>
                  How authentication, JWT validation and role-based access control work in the gateway.
                </SheetDescription>
              </SheetHeader>

              <div className="mt-4 space-y-6 text-sm">
                <div>
                  <h3 className="text-base font-semibold mb-2">Authentication Mode</h3>
                  <p className="mb-2">Defines how requests authenticate with the gateway.</p>
                  <div className="space-y-2">
                    <div>
                      <p className="font-medium">API Key</p>
                      <p className="text-muted-foreground">Requests must include an API key.</p>
                      <pre className="mt-1 rounded bg-muted p-2 font-mono text-xs overflow-x-auto">Authorization: Bearer rk_live_xxxxxxxxx</pre>
                    </div>
                    <div>
                      <p className="font-medium">JWT</p>
                      <p className="text-muted-foreground">Requests must include a JWT token. The gateway validates the token using a JWKS endpoint.</p>
                      <pre className="mt-1 rounded bg-muted p-2 font-mono text-xs overflow-x-auto">Authorization: Bearer eyJhbGciOi...</pre>
                    </div>
                    <div>
                      <p className="font-medium">Both</p>
                      <p className="text-muted-foreground">Accepts either API keys or JWT tokens. Recommended when migrating systems.</p>
                    </div>
                  </div>
                </div>

                <div>
                  <h3 className="text-base font-semibold mb-2">JWT Configuration</h3>
                  <p className="text-muted-foreground mb-2">JWT settings are used when authentication mode includes JWT.</p>
                  <ul className="list-disc pl-5 space-y-1">
                    <li><span className="font-medium">Issuer</span> – identifies who issued the token (e.g., Auth0, Keycloak, AWS Cognito).</li>
                    <li><span className="font-medium">Audience</span> – defines the intended recipient of the token (e.g., <code className="font-mono">router</code>).</li>
                    <li><span className="font-medium">JWKS URL</span> – endpoint where the gateway retrieves public keys to validate JWT signatures.</li>
                  </ul>
                  <pre className="mt-2 rounded bg-muted p-2 font-mono text-xs overflow-x-auto">https://example.com/.well-known/jwks.json</pre>
                </div>

                <div>
                  <h3 className="text-base font-semibold mb-2">Required Claims</h3>
                  <p className="text-muted-foreground mb-2">Defines which fields inside the JWT contain important routing information.</p>
                  <pre className="rounded bg-muted p-2 font-mono text-xs overflow-x-auto">{`{
  "sub": "user_123",
  "tenant_id": "tenant_a",
  "roles": ["admin"]
}`}</pre>
                  <p className="mt-2">Mapping:</p>
                  <ul className="list-disc pl-5">
                    <li>Tenant ID Claim → <code className="font-mono">tenant_id</code></li>
                    <li>Roles Claim → <code className="font-mono">roles</code></li>
                  </ul>
                </div>

                <div>
                  <h3 className="text-base font-semibold mb-2">RBAC Roles</h3>
                  <p className="text-muted-foreground mb-2">RBAC = Role Based Access Control. Roles determine which administrative features a caller can access.</p>
                  <div className="space-y-2">
                    <div>
                      <p className="font-medium">User Roles</p>
                      <p className="text-muted-foreground">Typical: API access, model inference, standard usage (e.g., <code className="font-mono">user</code>).</p>
                    </div>
                    <div>
                      <p className="font-medium">Admin Roles</p>
                      <p className="text-muted-foreground">Access to admin features (e.g., <code className="font-mono">/admin/config</code>, <code className="font-mono">/admin/tenants</code>).</p>
                    </div>
                    <div>
                      <p className="font-medium">Finance Roles</p>
                      <p className="text-muted-foreground">Billing and cost monitoring (e.g., <code className="font-mono">/admin/billing</code>, <code className="font-mono">/admin/costs</code>).</p>
                    </div>
                  </div>
                </div>
              </div>
            </SheetContent>
          </Sheet>
        </>
      )}

      {/* 6. Benchmarking - Only show if present in backend */}
      {!!hasBenchmarking && (
        <SectionCard
          title="Benchmarking"
          description="Platform benchmarking configuration"
          className="border-t-4 border-t-blue-500"
        >
          <div className="text-center py-8 text-muted-foreground">
            <p>Benchmarking configuration available</p>
          </div>
        </SectionCard>
      )}
    </div>
  )
}
