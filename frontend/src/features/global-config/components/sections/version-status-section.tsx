import { Badge } from '@/components/ui/badge'

interface VersionStatusSectionProps {
  config: Record<string, unknown>
  version: number
}

export function VersionStatusSection({ config, version }: VersionStatusSectionProps) {
  const providers = config.providers as Record<string, unknown> | undefined
  const models = config.models as Array<unknown> | undefined
  const circuitBreaker = config.circuit_breaker as Record<string, unknown> | undefined
  const rateLimit = config.rate_limit as Record<string, unknown> | undefined

  const providersCount = providers ? Object.keys(providers).length : 0
  const modelsCount = models ? models.length : 0
  const circuitBreakerBackend = (circuitBreaker?.Backend ?? circuitBreaker?.backend) ? String(circuitBreaker?.Backend ?? circuitBreaker?.backend) : '—'
  const rateLimitBackend = (rateLimit?.Backend ?? rateLimit?.backend) ? String(rateLimit?.Backend ?? rateLimit?.backend) : '—'

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-5">
      <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
        <span className="text-xs text-muted-foreground">Active Version</span>
        <Badge variant="outline" className="w-fit text-base">v{version}</Badge>
      </div>

      <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
        <span className="text-xs text-muted-foreground">Providers</span>
        <span className="text-2xl font-semibold">{providersCount}</span>
      </div>

      <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
        <span className="text-xs text-muted-foreground">Models</span>
        <span className="text-2xl font-semibold">{modelsCount}</span>
      </div>

      <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
        <span className="text-xs text-muted-foreground">Circuit Breaker</span>
        <Badge variant="secondary" className="w-fit">{circuitBreakerBackend}</Badge>
      </div>

      <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
        <span className="text-xs text-muted-foreground">Rate Limit</span>
        <Badge variant="secondary" className="w-fit">{rateLimitBackend}</Badge>
      </div>
    </div>
  )
}
