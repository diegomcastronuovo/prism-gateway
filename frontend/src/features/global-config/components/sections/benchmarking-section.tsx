import { Badge } from '@/components/ui/badge'

interface BenchmarkingSectionProps {
  config: Record<string, unknown>
}

export function BenchmarkingSection({ config }: BenchmarkingSectionProps) {
  const benchmarking = config.benchmarking as Record<string, unknown> | undefined
  const storage = benchmarking?.storage as Record<string, unknown> | undefined
  const routingIntegration = benchmarking?.routing_integration as Record<string, unknown> | undefined
  const request = benchmarking?.request as Record<string, unknown> | undefined

  if (!benchmarking) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>Benchmarking not configured</p>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {benchmarking.enabled !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">Enabled</span>
            <Badge variant={benchmarking.enabled ? 'default' : 'secondary'} className="w-fit">
              {benchmarking.enabled ? 'Yes' : 'No'}
            </Badge>
          </div>
        )}

        {benchmarking.interval_minutes !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">Interval</span>
            <span className="font-medium text-sm">{Number(benchmarking.interval_minutes)} min</span>
          </div>
        )}

        {benchmarking.timeout_ms !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">Timeout</span>
            <span className="font-medium text-sm">{Number(benchmarking.timeout_ms)} ms</span>
          </div>
        )}

        {benchmarking.max_concurrency !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">Max Concurrency</span>
            <span className="font-medium text-sm">{Number(benchmarking.max_concurrency)}</span>
          </div>
        )}

        {benchmarking.fail_open !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">Fail Open</span>
            <Badge variant={benchmarking.fail_open ? 'default' : 'secondary'} className="w-fit">
              {benchmarking.fail_open ? 'Yes' : 'No'}
            </Badge>
          </div>
        )}

        {storage?.retain_days !== undefined && (
          <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
            <span className="text-xs text-muted-foreground">Retain Days</span>
            <span className="font-medium text-sm">{Number(storage.retain_days)} days</span>
          </div>
        )}
      </div>

      {routingIntegration && (
        <div className="flex flex-col gap-3 p-4 rounded-lg border bg-card">
          <h4 className="font-semibold">Routing Integration</h4>
          <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
            {routingIntegration.enabled !== undefined && (
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Enabled</span>
                <Badge variant={routingIntegration.enabled ? 'default' : 'secondary'} className="w-fit">
                  {routingIntegration.enabled ? 'Yes' : 'No'}
                </Badge>
              </div>
            )}
            {routingIntegration.weight_latency !== undefined && (
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Weight Latency</span>
                <span className="font-medium text-sm">{Number(routingIntegration.weight_latency)}</span>
              </div>
            )}
            {routingIntegration.weight_error_rate !== undefined && (
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Weight Error Rate</span>
                <span className="font-medium text-sm">{Number(routingIntegration.weight_error_rate)}</span>
              </div>
            )}
          </div>
        </div>
      )}

      {request && (
        <div className="flex flex-col gap-3 p-4 rounded-lg border bg-card">
          <h4 className="font-semibold">Benchmark Request Preview</h4>
          <pre className="text-xs font-mono bg-muted p-3 rounded overflow-x-auto">
            {JSON.stringify(request, null, 2)}
          </pre>
        </div>
      )}
    </div>
  )
}
