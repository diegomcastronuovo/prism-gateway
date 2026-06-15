'use client'

import { useState, useEffect } from 'react'
import { useSearchParams } from 'next/navigation'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { GitCompare, AlertCircle, ArrowLeft } from 'lucide-react'
import { useTenants } from '@/features/tenants/api/use-tenants'

type ConfigDiffResponse = {
  object: string
  scope: string
  tenant_id: string
  from_version: number
  to_version: number
  before: Record<string, unknown>
  after: Record<string, unknown>
  changed_fields: string[]
}

async function fetchConfigDiff(
  scope: string,
  tenantId: string,
  fromVersion: string,
  toVersion: string
): Promise<ConfigDiffResponse> {
  const params = new URLSearchParams()
  params.set('scope', scope)
  if (tenantId) {
    params.set('tenant_id', tenantId)
  }
  params.set('from_version', fromVersion)
  params.set('to_version', toVersion)

  const resp = await fetch(`/api/admin/config/diff?${params.toString()}`)
  if (!resp.ok) {
    throw new Error(`Failed to fetch config diff: ${resp.status}`)
  }

  const data = await resp.json()
  return {
    object: data.object || 'config_diff',
    scope: data.scope || 'global',
    tenant_id: data.tenant_id || '',
    from_version: data.from_version || 0,
    to_version: data.to_version || 0,
    before: data.before || {},
    after: data.after || {},
    changed_fields: data.changed_fields || [],
  }
}

export function ConfigDiffView({ onBack }: { onBack: () => void }) {
  const searchParams = useSearchParams()
  const [scope, setScope] = useState(searchParams.get('scope') || 'global')
  const [tenantId, setTenantId] = useState(searchParams.get('tenant_id') || '')
  const [fromVersion, setFromVersion] = useState(searchParams.get('from_version') || '')
  const [toVersion, setToVersion] = useState(searchParams.get('to_version') || '')
  const [shouldFetch, setShouldFetch] = useState(false)
  
  const { data: tenants, isLoading: isLoadingTenants } = useTenants()

  useEffect(() => {
    const hasParams = searchParams.get('from_version') && searchParams.get('to_version')
    if (hasParams) {
      setShouldFetch(true)
    }
  }, [searchParams])

  const { data, isLoading, error } = useQuery({
    queryKey: ['config-diff', scope, tenantId, fromVersion, toVersion],
    queryFn: () => fetchConfigDiff(scope, tenantId, fromVersion, toVersion),
    enabled: shouldFetch && !!fromVersion && !!toVersion,
  })

  const handleCompare = () => {
    if (fromVersion && toVersion) {
      setShouldFetch(true)
    }
  }

  return (
    <div>
      <PageHeader
        title="Configuration Diff Viewer"
        description="Compare config versions and inspect changes"
        action={
          <Button variant="outline" onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to History
          </Button>
        }
      />

      {/* Version Selection Form */}
      <SectionCard title="Select Versions to Compare">
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-5">
          <div className="space-y-2">
            <Label htmlFor="scope">Scope</Label>
            <select
              id="scope"
              value={scope}
              onChange={(e) => setScope(e.target.value)}
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            >
              <option value="global">Global</option>
              <option value="tenant">Tenant</option>
            </select>
          </div>

          {scope === 'tenant' && (
            <div className="space-y-2">
              <Label htmlFor="tenant_id">Tenant ID</Label>
              {isLoadingTenants ? (
                <Skeleton className="h-10 w-full" />
              ) : (
                <select
                  id="tenant_id"
                  value={tenantId}
                  onChange={(e) => setTenantId(e.target.value)}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="">Select a tenant</option>
                  {tenants?.map((tenant) => (
                    <option key={tenant.tenant_id} value={tenant.tenant_id}>
                      {tenant.tenant_id}
                    </option>
                  ))}
                </select>
              )}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="from_version">From Version</Label>
            <Input
              id="from_version"
              type="number"
              value={fromVersion}
              onChange={(e) => setFromVersion(e.target.value)}
              placeholder="17"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="to_version">To Version</Label>
            <Input
              id="to_version"
              type="number"
              value={toVersion}
              onChange={(e) => setToVersion(e.target.value)}
              placeholder="18"
            />
          </div>

          <div className="flex items-end">
            <Button onClick={handleCompare} disabled={!fromVersion || !toVersion} className="w-full">
              <GitCompare className="mr-2 h-4 w-4" />
              Compare
            </Button>
          </div>
        </div>
      </SectionCard>

      {/* Results */}
      {shouldFetch && (
        <>
          {isLoading ? (
            <div className="space-y-6">
              <Skeleton className="h-32" />
              <Skeleton className="h-64" />
              <div className="grid gap-6 md:grid-cols-2">
                <Skeleton className="h-96" />
                <Skeleton className="h-96" />
              </div>
            </div>
          ) : error ? (
            <SectionCard title="Error">
              <div className="flex items-center gap-2 text-sm text-destructive">
                <AlertCircle className="h-4 w-4" />
                Failed to load config diff
              </div>
            </SectionCard>
          ) : data ? (
            <>
              {/* Summary Header */}
              <SectionCard title="Comparison Summary">
                <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
                  <div>
                    <div className="text-sm text-muted-foreground">Scope</div>
                    <div className="font-medium capitalize">{data.scope}</div>
                  </div>
                  {data.tenant_id && (
                    <div>
                      <div className="text-sm text-muted-foreground">Tenant</div>
                      <div className="font-medium">{data.tenant_id}</div>
                    </div>
                  )}
                  <div>
                    <div className="text-sm text-muted-foreground">From Version</div>
                    <div className="font-medium">v{data.from_version}</div>
                  </div>
                  <div>
                    <div className="text-sm text-muted-foreground">To Version</div>
                    <div className="font-medium">v{data.to_version}</div>
                  </div>
                </div>
              </SectionCard>

              {/* Changed Fields */}
              <SectionCard title="Changed Fields">
                {data.changed_fields.length === 0 ? (
                  <div className="text-sm text-muted-foreground">No changed fields.</div>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    {data.changed_fields.map((field, idx) => (
                      <Badge key={idx} variant="secondary">
                        {field}
                      </Badge>
                    ))}
                  </div>
                )}
              </SectionCard>

              {/* Before / After JSON */}
              <div className="grid gap-6 md:grid-cols-2">
                <SectionCard title="Before">
                  <pre className="overflow-x-auto rounded-md bg-muted p-4 text-xs">
                    <code>{JSON.stringify(data.before, null, 2)}</code>
                  </pre>
                </SectionCard>

                <SectionCard title="After">
                  <pre className="overflow-x-auto rounded-md bg-muted p-4 text-xs">
                    <code>{JSON.stringify(data.after, null, 2)}</code>
                  </pre>
                </SectionCard>
              </div>
            </>
          ) : null}
        </>
      )}
    </div>
  )
}
