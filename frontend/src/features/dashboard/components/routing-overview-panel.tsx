import type { ReactNode } from 'react'
import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { getFirstValue, formatDurationMs } from '@/features/dashboard/lib/config-utils'

interface RoutingOverviewPanelProps {
  config?: Record<string, unknown>
  isLoading?: boolean
  error?: Error | null
}

function ValueText({ value }: { value: unknown }) {
  if (value === undefined || value === null || value === '') {
    return <span className="text-sm text-muted-foreground">Not available</span>
  }
  return <span className="text-sm font-medium">{String(value)}</span>
}

function InfoRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-xs text-muted-foreground">{label}</span>
      {value}
    </div>
  )
}

function formatWeight(value: unknown) {
  if (value === undefined || value === null) return undefined
  const n = Number(value)
  if (!Number.isFinite(n)) return undefined
  return n.toFixed(2)
}

export function RoutingOverviewPanel({ config, isLoading, error }: RoutingOverviewPanelProps) {
  const strategy = getFirstValue(config, [
    'routing.strategy',
    'routing_strategy',
    'routing.Strategy',
  ])
  const fallbackEnabled = getFirstValue(config, [
    'routing.fallback.enabled',
    'routing.fallback_enabled',
    'routing.fallback.Enabled',
  ])
  const fallbackTimeout = getFirstValue(config, [
    'routing.fallback.timeout_ms',
    'routing.fallback_timeout_ms',
    'routing.fallback.TimeoutMs',
  ])
  const maxAttempts = getFirstValue(config, [
    'routing.fallback.max_attempts',
    'routing.max_attempts',
    'routing.fallback.MaxAttempts',
  ])

  const weightCost = getFirstValue(config, [
    'routing.smart.weights.cost',
    'routing.smart.weights.Cost',
    'routing.Smart.Weights.Cost',
    'smart_routing.weights.cost',
    'smart_routing.weight_cost',
    'benchmarking.routing_integration.weight_cost',
  ])
  const weightLatency = getFirstValue(config, [
    'routing.smart.weights.latency',
    'routing.smart.weights.Latency',
    'routing.Smart.Weights.Latency',
    'smart_routing.weights.latency',
    'smart_routing.weight_latency',
    'benchmarking.routing_integration.weight_latency',
  ])
  const weightErrors = getFirstValue(config, [
    'routing.smart.weights.errors',
    'routing.smart.weights.Errors',
    'routing.Smart.Weights.Errors',
    'smart_routing.weights.errors',
    'smart_routing.weight_error_rate',
    'benchmarking.routing_integration.weight_error_rate',
  ])

  const stages = getFirstValue(config, [
    'routing.smart.stages',
    'routing.smart.Stages',
    'routing.Smart.Stages',
    'smart_routing.stages',
    'routing.stages',
  ]) as string[] | undefined

  return (
    <SectionCard
      title="Routing Overview"
      description="Global routing configuration snapshot"
    >
      {isLoading ? (
        <Skeleton className="h-48" />
      ) : error ? (
        <div className="flex items-center justify-between rounded-md border p-4">
          <div>
            <p className="text-sm text-destructive">Failed to load routing overview</p>
            <p className="text-xs text-muted-foreground">{error.message}</p>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            <div className="rounded-lg border bg-card p-4 space-y-2">
              <h4 className="text-sm font-semibold">Strategy</h4>
              <ValueText value={strategy} />
            </div>
            <div className="rounded-lg border bg-card p-4 space-y-2">
              <h4 className="text-sm font-semibold">Smart Routing Weights</h4>
              <div className="space-y-2">
                <InfoRow label="Cost" value={<ValueText value={formatWeight(weightCost)} />} />
                <InfoRow label="Latency" value={<ValueText value={formatWeight(weightLatency)} />} />
                <InfoRow label="Errors" value={<ValueText value={formatWeight(weightErrors)} />} />
              </div>
            </div>
            <div className="rounded-lg border bg-card p-4 space-y-2">
              <h4 className="text-sm font-semibold">Fallback</h4>
              <div className="space-y-2">
                <InfoRow
                  label="Enabled"
                  value={
                    fallbackEnabled === undefined || fallbackEnabled === null ? (
                      <span className="text-sm text-muted-foreground">Not available</span>
                    ) : (
                      <Badge variant={fallbackEnabled ? 'default' : 'secondary'}>
                        {fallbackEnabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    )
                  }
                />
                <InfoRow label="Timeout" value={<ValueText value={formatDurationMs(fallbackTimeout)} />} />
                <InfoRow label="Max Attempts" value={<ValueText value={maxAttempts} />} />
              </div>
            </div>
          </div>

          <div className="rounded-lg border bg-card p-4 space-y-2">
            <h4 className="text-sm font-semibold">Smart Routing Stages</h4>
            {Array.isArray(stages) && stages.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {stages.map((stage) => (
                  <Badge key={stage} variant="outline">
                    {stage}
                  </Badge>
                ))}
              </div>
            ) : (
              <span className="text-sm text-muted-foreground">Not available</span>
            )}
          </div>
        </div>
      )}
    </SectionCard>
  )
}
