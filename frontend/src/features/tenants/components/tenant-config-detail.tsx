'use client'

import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Edit, Trash2, ChevronDown, ChevronUp, Code2 } from 'lucide-react'
import { type TenantConfig, useTenantApiKeys, useTenantUsage, useTenantBudgetStatus, useUpdateTenantConfig } from '../api/use-tenants'
import { TenantRoutingSection } from './tenant-routing-section'
import { TenantFeaturesSection } from './tenant-features-section'
import { TenantSecuritySection } from './tenant-security-section'
import { TenantAccessSection } from './tenant-access-section'
import { TenantUsageSection } from './tenant-usage-section'
import { TenantRateLimitSection } from './tenant-rate-limit-section'

const CC_DEFAULT_TIMEZONE = 'America/Buenos_Aires'

const timezones = [
  'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'America/Mexico_City', 'America/Bogota', 'America/Lima', 'America/Santiago',
  'America/Buenos_Aires', 'America/Sao_Paulo', 'America/Montevideo', 'America/Asuncion',
  'America/La_Paz', 'America/Guayaquil', 'America/Caracas', 'America/Guatemala',
  'America/Costa_Rica', 'America/Panama', 'America/El_Salvador', 'America/Havana',
  'America/Santo_Domingo', 'America/Puerto_Rico',
  'Europe/London', 'Europe/Paris', 'Asia/Tokyo', 'UTC',
]

function getEnvironmentBadgeClass(env?: string) {
  switch ((env || '').toUpperCase()) {
    case 'DEV':
      return 'bg-green-100 text-green-800 border-green-200'
    case 'STAGING':
      return 'bg-blue-100 text-blue-800 border-blue-200'
    case 'PROD':
      return 'bg-red-100 text-red-800 border-red-200'
    default:
      return ''
  }
}

interface TenantConfigDetailProps {
  config: TenantConfig | undefined
  isLoading: boolean
  onEditBudget: () => void
  onEditRouting: () => void
  onEditFeatures: () => void
  onEditSecurity: () => void
  onEditTrafficSplit: () => void
  onEditRateLimit: () => void
  onEditOutputLimit: () => void
  onDelete: () => void
}

export function TenantConfigDetail({
  config,
  isLoading,
  onEditBudget,
  onEditRouting,
  onEditFeatures,
  onEditSecurity,
  onEditTrafficSplit,
  onEditRateLimit,
  onEditOutputLimit,
  onDelete,
}: TenantConfigDetailProps) {
  const [showRawJson, setShowRawJson] = useState(false)
  const actionButtonClass = 'min-w-[140px] h-9 inline-flex items-center justify-center gap-2'

  const currentMonth = new Date().toISOString().slice(0, 7)
  const { data: apiKeys, isLoading: apiKeysLoading, error: apiKeysError } = useTenantApiKeys(config?.tenant_id || null)
  const { data: usage, isLoading: usageLoading, error: usageError } = useTenantUsage(config?.tenant_id || null, currentMonth)
  const {
    data: budgetStatus,
    isLoading: budgetStatusLoading,
    error: budgetStatusError,
  } = useTenantBudgetStatus(config?.tenant_id || null)

  // Claude Code license check — 403 = not licensed (expected product state)
  const claudeCodeLicenseQuery = useQuery({
    queryKey: ['claude-code-provider'],
    queryFn: async () => {
      const res = await fetch('/api/providers/claude-code')
      if (res.status === 403) return { licensed: false }
      if (!res.ok) return { licensed: false }
      return { licensed: true }
    },
    staleTime: 30_000,
  })
  const isClaudeCodeLicensed = claudeCodeLicenseQuery.data?.licensed === true

  const updateTenantConfig = useUpdateTenantConfig()

  // Claude Code local form state
  const [ccEnabled, setCcEnabled] = useState(false)
  const [ccBudget, setCcBudget] = useState('0')
  const [ccTimezone, setCcTimezone] = useState(CC_DEFAULT_TIMEZONE)

  // Load Claude Code config from tenant config whenever it changes
  useEffect(() => {
    const cc = (config?.config as Record<string, unknown> | undefined)?.claude_code as
      | { enabled?: boolean; monthly_budget?: number; timezone?: string }
      | undefined
    setCcEnabled(cc?.enabled ?? false)
    setCcBudget(String(cc?.monthly_budget ?? 0))
    setCcTimezone(cc?.timezone ?? CC_DEFAULT_TIMEZONE)
  }, [config])

  const handleClaudeCodeSave = async () => {
    if (!config) return
    const budgetVal = parseFloat(ccBudget)
    await updateTenantConfig.mutateAsync({
      tenantId: config.tenant_id,
      version: config.version,
      patch: {
        claude_code: {
          enabled: ccEnabled,
          monthly_budget: isNaN(budgetVal) || budgetVal < 0 ? 0 : budgetVal,
          timezone: ccTimezone,
        },
      },
    })
  }

  if (isLoading) {
    return (
      <SectionCard title="Tenant Configuration" className="border-t-4 border-t-purple-500">
        <div className="space-y-4">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      </SectionCard>
    )
  }

  if (!config) {
    return (
      <SectionCard title="Tenant Configuration" className="border-t-4 border-t-purple-500">
        <div className="text-center py-8 text-muted-foreground">
          Select a tenant to view configuration
        </div>
      </SectionCard>
    )
  }

  return (
    <SectionCard title="Tenant Configuration" className="border-t-4 border-t-purple-500">
      <div className="space-y-6">
        {/* Summary Section */}
        <div className="space-y-4">
          <div className="flex items-start justify-between">
            <div>
              <h3 className="text-lg font-semibold">Summary</h3>
              <div className="mt-2 space-y-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-muted-foreground">Tenant ID:</span>
                  <Badge variant="outline">{config.tenant_id}</Badge>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-muted-foreground">Config Version:</span>
                  <Badge variant="outline">{config.version}</Badge>
                </div>
              </div>
            </div>
            <div className="flex gap-2">
              <Button variant="destructive" onClick={onDelete} className={actionButtonClass}>
                <span className="inline-flex items-center gap-2">
                  <Trash2 className="h-4 w-4" />
                  <span>Delete</span>
                </span>
              </Button>
            </div>
          </div>
        </div>

        <div className="border-t" />
        {/* Environment Section */}
        <div className="space-y-4">
          <h3 className="text-lg font-semibold">Environment</h3>
          <div className="flex items-center gap-2">
            <Badge
              variant="outline"
              className={getEnvironmentBadgeClass(config.environment || (config.config as Record<string, unknown>)?.environment as string | undefined)}
            >
              {config.environment || ((config.config as Record<string, unknown>)?.environment as string | undefined) || '—'}
            </Badge>
          </div>
        </div>

        <div className="border-t" />
        {/* Budget Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Budget</h3>
            <Button variant="outline" onClick={onEditBudget} className={actionButtonClass}>
              <span className="inline-flex items-center gap-2">
                <Edit className="h-4 w-4" />
                <span>Edit</span>
              </span>
            </Button>
          </div>
          <div className="space-y-2">
            {config.config.budgets?.monthly_usd !== undefined ? (
              <div className="flex justify-between items-center">
                <span className="text-sm text-muted-foreground">Monthly Budget</span>
                <Badge variant="outline">${config.config.budgets.monthly_usd}</Badge>
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">No budget configured</p>
            )}
            {config.config.budgets?.timezone && (
              <div className="flex justify-between items-center">
                <span className="text-sm text-muted-foreground">Timezone</span>
                <Badge variant="outline">{config.config.budgets.timezone}</Badge>
              </div>
            )}
            <div className="border-t my-2" />
            <div className="space-y-1">
              <p className="text-sm font-medium">Budget Enforcement</p>
              <div className="flex justify-between items-center">
                <span className="text-sm text-muted-foreground">Enabled</span>
                <Badge variant={config.config.budget_enforcement?.enabled ? 'default' : 'secondary'}>
                  {config.config.budget_enforcement?.enabled ? 'Enabled' : 'Disabled'}
                </Badge>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-sm text-muted-foreground">Mode</span>
                <Badge variant="outline">{config.config.budget_enforcement?.mode ?? 'report_only'}</Badge>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-sm text-muted-foreground">Warn %</span>
                <Badge variant="outline">
                  {((config.config.budget_enforcement?.thresholds?.warn_pct ?? 0.8) * 100).toFixed(0)}%
                </Badge>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-sm text-muted-foreground">Hard %</span>
                <Badge variant="outline">
                  {((config.config.budget_enforcement?.thresholds?.hard_pct ?? 1) * 100).toFixed(0)}%
                </Badge>
              </div>
            </div>
            <div className="border-t my-2" />
            <div className="space-y-1">
              <p className="text-sm font-medium">Runtime Status</p>
              {budgetStatusLoading ? (
                <Skeleton className="h-8 w-48" />
              ) : budgetStatusError ? (
                <p className="text-xs text-muted-foreground">Could not load budget runtime status</p>
              ) : budgetStatus ? (
                <>
                  <p className="text-sm text-muted-foreground">
                    ${budgetStatus.spend_usd.toFixed(2)} / ${budgetStatus.budget_usd.toFixed(2)} ({(budgetStatus.pct * 100).toFixed(1)}%)
                  </p>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline">Mode: {budgetStatus.enforcement_mode ?? '—'}</Badge>
                    <Badge variant="outline">Warn: {budgetStatus.warn_pct == null ? '—' : `${(budgetStatus.warn_pct * 100).toFixed(0)}%`}</Badge>
                    <Badge variant="outline">Hard: {budgetStatus.hard_pct == null ? '—' : `${(budgetStatus.hard_pct * 100).toFixed(0)}%`}</Badge>
                  </div>
                </>
              ) : (
                <p className="text-xs text-muted-foreground">No runtime status available</p>
              )}
            </div>
          </div>
        </div>

        <div className="border-t" />

        {/* Routing Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Routing</h3>
            <Button variant="outline" onClick={onEditRouting} className={actionButtonClass}>
              <span className="inline-flex items-center gap-2">
                <Edit className="h-4 w-4" />
                <span>Edit</span>
              </span>
            </Button>
          </div>
          <TenantRoutingSection
            routingStrategy={
              config.config.routing?.strategy
              ?? (config.config.routing_strategy as string)
            }
            defaultRouteGroup={config.config.routing?.route_group}
            allowedModels={config.config.allowed_models}
            routeGroups={config.config.route_groups}
            trafficSplit={config.config.traffic_split as Record<string, Array<{ model: string; weight: number }>> | undefined}
          />
        </div>

        <div className="border-t" />

        {/* Features Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Features</h3>
            <Button variant="outline" onClick={onEditFeatures} className={actionButtonClass}>
              <span className="inline-flex items-center gap-2">
                <Edit className="h-4 w-4" />
                <span>Edit</span>
              </span>
            </Button>
          </div>
          <TenantFeaturesSection config={config.config} />
        </div>

        <div className="border-t" />

        {/* Security Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Security</h3>
            <Button variant="outline" onClick={onEditSecurity} className={actionButtonClass}>
              <span className="inline-flex items-center gap-2">
                <Edit className="h-4 w-4" />
                <span>Edit Security</span>
              </span>
            </Button>
          </div>
          <TenantSecuritySection externalPii={config.config.hooks?.global?.external_pii} />
        </div>

        <div className="border-t" />

        {/* Rate Limit (tenant-level RPM / burst — not global limiter backend) */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Rate Limit</h3>
            <Button variant="outline" onClick={onEditRateLimit} className={actionButtonClass}>
              <span className="inline-flex items-center gap-2">
                <Edit className="h-4 w-4" />
                <span>Edit</span>
              </span>
            </Button>
          </div>
          <p className="text-sm text-muted-foreground">
            Controls how request-per-minute limits are applied for this tenant.
          </p>
          <TenantRateLimitSection rateLimit={config.config.rate_limit} />
          <p className="text-xs text-muted-foreground leading-relaxed">
            tenant: one shared limit for the whole tenant · api_key: one limit per API key within the
            tenant · jwt_sub: one limit per JWT user within the tenant
          </p>
        </div>

        <div className="border-t" />

        {/* Output Limit Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Output Limit</h3>
            <Button variant="outline" onClick={onEditOutputLimit} className={actionButtonClass}>
              <span className="inline-flex items-center gap-2">
                <Edit className="h-4 w-4" />
                <span>Edit</span>
              </span>
            </Button>
          </div>
          <div className="flex justify-between items-center">
            <span className="text-sm text-muted-foreground">LLM maximum output tokens:</span>
            <Badge variant="outline">
              {((config.config as Record<string, unknown>).max_output_tokens ?? 0) === 0
                ? 'Unlimited'
                : String((config.config as Record<string, unknown>).max_output_tokens)}
            </Badge>
          </div>
          <p className="text-xs text-muted-foreground">
            0 means unlimited. Applies only to LLM requests for this tenant.
          </p>
        </div>

        <div className="border-t" />

        {/* Traffic Split Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Traffic Split</h3>
            <Button variant="outline" onClick={onEditTrafficSplit} className={actionButtonClass}>
              <span className="inline-flex items-center gap-2">
                <Edit className="h-4 w-4" />
                <span>Edit</span>
              </span>
            </Button>
          </div>
          {!config.config.traffic_split && (
            <p className="text-sm text-muted-foreground">No traffic split configured</p>
          )}
        </div>

        <div className="border-t" />

        {/* Claude Code Section — only visible when licensed */}
        {isClaudeCodeLicensed && (
          <>
            <div className="space-y-4">
              <div className="flex items-center gap-2">
                <Code2 className="h-4 w-4 text-primary" />
                <h3 className="text-lg font-semibold">Claude Code</h3>
              </div>

              {/* Enable toggle */}
              <div className="flex items-center justify-between rounded-lg border p-3">
                <div className="space-y-0.5">
                  <p className="font-medium">Enable Claude Code</p>
                  <p className="text-sm text-muted-foreground">
                    {ccEnabled
                      ? 'Claude Code is enabled for this tenant'
                      : 'Claude Code is disabled for this tenant'}
                  </p>
                </div>
                <Switch
                  checked={ccEnabled}
                  onCheckedChange={setCcEnabled}
                  disabled={updateTenantConfig.isPending}
                />
              </div>

              {/* Monthly Budget */}
              <div className="space-y-1.5">
                <Label htmlFor="cc-monthly-budget">Monthly Budget (USD)</Label>
                <Input
                  id="cc-monthly-budget"
                  type="number"
                  min="0"
                  step="0.01"
                  value={ccBudget}
                  onChange={(e) => setCcBudget(e.target.value)}
                  disabled={updateTenantConfig.isPending}
                />
                <p className="text-xs text-muted-foreground">
                  Claude Code budget only. Independent from the normal LLM budget. 0 = unlimited.
                </p>
              </div>

              {/* Timezone */}
              <div className="space-y-1.5">
                <Label htmlFor="cc-timezone">Timezone</Label>
                <Select
                  value={ccTimezone}
                  onValueChange={setCcTimezone}
                  disabled={updateTenantConfig.isPending}
                >
                  <SelectTrigger id="cc-timezone">
                    <SelectValue placeholder="Select timezone" />
                  </SelectTrigger>
                  <SelectContent>
                    {timezones.map((tz) => (
                      <SelectItem key={tz} value={tz}>
                        {tz}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <Button
                onClick={handleClaudeCodeSave}
                disabled={updateTenantConfig.isPending}
                size="sm"
              >
                {updateTenantConfig.isPending ? 'Saving…' : 'Save Claude Code Settings'}
              </Button>
            </div>
            <div className="border-t" />
          </>
        )}

        {/* Access Section */}
        <TenantAccessSection
          tenantId={config.tenant_id}
          apiKeys={apiKeys}
          isLoading={apiKeysLoading}
          error={apiKeysError}
        />

        <div className="border-t" />

        {/* Usage Section */}
        <TenantUsageSection
          usage={usage}
          isLoading={usageLoading}
          error={usageError}
        />

        <div className="border-t" />

        {/* Raw JSON Viewer - Advanced Section */}
        <div>
          <h3 className="text-lg font-semibold mb-3">Advanced</h3>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setShowRawJson(!showRawJson)}
            className="mb-2"
          >
            {showRawJson ? (
              <>
                <ChevronUp className="h-4 w-4 mr-2" />
                Hide Raw JSON
              </>
            ) : (
              <>
                <ChevronDown className="h-4 w-4 mr-2" />
                Show Raw JSON
              </>
            )}
          </Button>
          {showRawJson && (
            <pre className="bg-muted p-4 rounded-lg overflow-auto text-xs max-h-96">
              {JSON.stringify(config, null, 2)}
            </pre>
          )}
        </div>
      </div>
    </SectionCard>
  )
}
