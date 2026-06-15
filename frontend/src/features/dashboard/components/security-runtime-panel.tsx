import { SectionCard } from '@/components/shared/section-card'
import { Skeleton } from '@/components/ui/skeleton'
import { getFirstValue } from '@/features/dashboard/lib/config-utils'

interface SecurityRuntimePanelProps {
  config?: Record<string, unknown>
  isLoading?: boolean
  error?: Error | null
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border bg-card p-4 space-y-1">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="text-sm font-medium">{value}</div>
    </div>
  )
}

function formatAuthSummary(mode?: string) {
  if (!mode) return 'Not available'
  const normalized = mode.toLowerCase()
  if (normalized === 'both') return 'JWT + API Keys'
  if (normalized === 'jwt') return 'JWT'
  if (normalized === 'api_key' || normalized === 'api-key') return 'API Keys'
  return mode
}

function formatBackendSummary(backend?: string) {
  if (!backend) return 'Not available'
  const normalized = backend.toLowerCase()
  if (normalized.includes('redis')) return 'Redis-backed'
  return backend
}

export function SecurityRuntimePanel({ config, isLoading, error }: SecurityRuntimePanelProps) {
  const authMode = getFirstValue(config, ['auth.mode', 'auth.Mode', 'Auth.Mode'])
  const rateLimitBackend = getFirstValue(config, [
    'rate_limit.Backend',
    'rate_limit.backend',
    'RateLimit.Backend',
  ])
  const circuitBackend = getFirstValue(config, [
    'circuit_breaker.Backend',
    'circuit_breaker.backend',
    'CircuitBreaker.Backend',
  ])
  const circuitEnabled = getFirstValue(config, [
    'circuit_breaker.Defaults.Enabled',
    'circuit_breaker.defaults.enabled',
    'circuit_breaker.enabled',
    'CircuitBreaker.Defaults.Enabled',
  ])

  const circuitSummary =
    circuitEnabled === undefined || circuitEnabled === null
      ? 'Not available'
      : `${circuitEnabled ? 'Enabled' : 'Disabled'}${
          circuitBackend ? `, ${String(circuitBackend)}` : ''
        }`

  
  return (
    <SectionCard
      title="Security & Runtime"
      description="Authentication, runtime coordination, and safety systems"
      className="border-t-4 border-t-emerald-500"
    >
      {isLoading ? (
        <Skeleton className="h-48" />
      ) : error ? (
        <div className="flex items-center justify-between rounded-md border p-4">
          <div>
            <p className="text-sm text-destructive">Failed to load security & runtime</p>
            <p className="text-xs text-muted-foreground">{error.message}</p>
          </div>
        </div>
      ) : (
        <div className="grid gap-4">
          <SummaryCard label="Auth" value={formatAuthSummary(authMode ? String(authMode) : '')} />
          <SummaryCard
            label="Rate Limit"
            value={formatBackendSummary(rateLimitBackend ? String(rateLimitBackend) : '')}
          />
          <SummaryCard label="Circuit Breaker" value={circuitSummary} />
        </div>
      )}
    </SectionCard>
  )
}
