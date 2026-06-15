import type { ReactNode } from 'react'
import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { getFirstValue } from '@/features/dashboard/lib/config-utils'

interface SystemCorePanelProps {
  config?: Record<string, unknown>
  isLoading?: boolean
  error?: Error | null
}

function ValueText({ value }: { value: unknown }) {
  if (value === undefined) {
    return <span className="text-sm text-muted-foreground">Not available</span>
  }
  if (value === null || value === '') {
    return <span className="text-sm text-muted-foreground">Not configured</span>
  }
  return <span className="text-sm font-medium">{String(value)}</span>
}

function BooleanBadge({ value, yesLabel, noLabel }: { value: unknown; yesLabel: string; noLabel: string }) {
  if (value === undefined) {
    return <span className="text-sm text-muted-foreground">Not available</span>
  }
  if (value === null || value === '') {
    return <span className="text-sm text-muted-foreground">Not configured</span>
  }
  const isEnabled = Boolean(value)
  return (
    <Badge variant={isEnabled ? 'default' : 'secondary'} className="w-fit">
      {isEnabled ? yesLabel : noLabel}
    </Badge>
  )
}

function InfoRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-xs text-muted-foreground">{label}</span>
      {value}
    </div>
  )
}

export function SystemCorePanel({ config, isLoading, error }: SystemCorePanelProps) {
  return (
    <SectionCard
      title="System Core"
      description="Global runtime systems and infrastructure status"
      className="border-t-4 border-t-purple-500"
    >
      {isLoading ? (
        <div className="space-y-4">
          <Skeleton className="h-36" />
          <Skeleton className="h-36" />
        </div>
      ) : error ? (
        <div className="flex items-center justify-between rounded-md border p-4">
          <div>
            <p className="text-sm text-destructive">Failed to load system core</p>
            <p className="text-xs text-muted-foreground">{error.message}</p>
          </div>
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="rounded-lg border bg-card p-4 space-y-3">
            <h4 className="font-semibold">Circuit Breaker</h4>
            <div className="space-y-2">
              <InfoRow
                label="Enabled"
                value={
                  <BooleanBadge
                    value={getFirstValue(config, [
                      'circuit_breaker.Defaults.Enabled',
                      'circuit_breaker.defaults.enabled',
                      'circuit_breaker.enabled',
                      'CircuitBreaker.Defaults.Enabled',
                    ])}
                    yesLabel="Enabled"
                    noLabel="Disabled"
                  />
                }
              />
              <InfoRow
                label="Backend"
                value={
                  <ValueText
                    value={getFirstValue(config, [
                      'circuit_breaker.Backend',
                      'circuit_breaker.backend',
                      'CircuitBreaker.Backend',
                    ])}
                  />
                }
              />
              <InfoRow
                label="Fail Open"
                value={
                  <BooleanBadge
                    value={getFirstValue(config, [
                      'circuit_breaker.fail_open',
                      'circuit_breaker.FailOpen',
                      'circuit_breaker.Defaults.FailOpen',
                      'circuit_breaker.Redis.fail_open',
                      'circuit_breaker.Redis.FailOpen',
                      'CircuitBreaker.fail_open',
                      'CircuitBreaker.FailOpen',
                      'CircuitBreaker.Defaults.FailOpen',
                    ])}
                    yesLabel="Yes"
                    noLabel="No"
                  />
                }
              />
              <InfoRow
                label="Redis Address"
                value={
                  <ValueText
                    value={getFirstValue(config, [
                      'circuit_breaker.Redis.Addr',
                      'circuit_breaker.Redis.addr',
                      'circuit_breaker.redis.addr',
                      'CircuitBreaker.Redis.Addr',
                      'CircuitBreaker.redis.addr',
                    ])}
                  />
                }
              />
            </div>
          </div>

          <div className="rounded-lg border bg-card p-4 space-y-3">
            <h4 className="font-semibold">Auth</h4>
            <div className="space-y-2">
              <InfoRow
                label="Mode"
                value={
                  <ValueText value={getFirstValue(config, ['auth.mode', 'auth.Mode', 'Auth.Mode'])} />
                }
              />
              <InfoRow
                label="JWT Issuer"
                value={
                  <ValueText value={getFirstValue(config, ['auth.jwt.issuer', 'auth.jwt.Issuer'])} />
                }
              />
              <InfoRow
                label="Audience"
                value={
                  <ValueText value={getFirstValue(config, ['auth.jwt.audience', 'auth.jwt.Audience'])} />
                }
              />
              <InfoRow
                label="Auth Role Model"
                value={
                  <BooleanBadge
                    value={getFirstValue(config, ['auth.jwt.rbac', 'auth.jwt.required_claims'])}
                    yesLabel="Available"
                    noLabel="Not Available"
                  />
                }
              />
            </div>
          </div>

          <div className="rounded-lg border bg-card p-4 space-y-3">
            <h4 className="font-semibold">Rate Limit</h4>
            <div className="space-y-2">
              <InfoRow
                label="Backend"
                value={
                  <ValueText
                    value={getFirstValue(config, [
                      'rate_limit.Backend',
                      'rate_limit.backend',
                      'RateLimit.Backend',
                    ])}
                  />
                }
              />
              <InfoRow
                label="Redis"
                value={
                  <ValueText
                    value={getFirstValue(config, [
                      'rate_limit.Redis.Addr',
                      'rate_limit.redis.addr',
                      'RateLimit.Redis.Addr',
                    ])}
                  />
                }
              />
              <InfoRow
                label="Key Prefix"
                value={
                  <ValueText
                    value={getFirstValue(config, [
                      'rate_limit.Redis.KeyPrefix',
                      'rate_limit.redis.key_prefix',
                      'RateLimit.Redis.KeyPrefix',
                    ])}
                  />
                }
              />
              <InfoRow
                label="Fail Open"
                value={
                  <BooleanBadge
                    value={getFirstValue(config, [
                      'rate_limit.Redis.FailOpen',
                      'rate_limit.Redis.fail_open',
                      'rate_limit.redis.fail_open',
                      'RateLimit.Redis.FailOpen',
                    ])}
                    yesLabel="Yes"
                    noLabel="No"
                  />
                }
              />
            </div>
          </div>
        </div>
      )}
    </SectionCard>
  )
}
