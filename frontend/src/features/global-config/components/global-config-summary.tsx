import { Badge } from '@/components/ui/badge'

interface GlobalConfigSummaryProps {
  config: Record<string, unknown>
}

export function GlobalConfigSummary({ config }: GlobalConfigSummaryProps) {
  const getNestedValue = (obj: Record<string, unknown>, path: string): unknown => {
    const keys = path.split('.')
    let current: unknown = obj
    
    for (const key of keys) {
      if (current && typeof current === 'object' && key in current) {
        current = (current as Record<string, unknown>)[key]
      } else {
        return undefined
      }
    }
    
    return current
  }

  const routing = getNestedValue(config, 'routing') as Record<string, unknown> | undefined
  const benchmarking = getNestedValue(config, 'benchmarking') as Record<string, unknown> | undefined
  const semantic = getNestedValue(config, 'semantic') as Record<string, unknown> | undefined
  const toolRouting = getNestedValue(config, 'tool_routing') as Record<string, unknown> | undefined
  const budget = getNestedValue(config, 'budget') as Record<string, unknown> | undefined

  return (
    <div className="space-y-6">
      {/* Routing Section */}
      {routing && (
        <div className="space-y-3">
          <h3 className="text-lg font-semibold">Routing</h3>
          <div className="grid gap-3 md:grid-cols-2">
            {routing.strategy !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Strategy</span>
                <Badge variant="outline">{String(routing.strategy)}</Badge>
              </div>
            )}
            {routing.fallback_enabled !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Fallback Enabled</span>
                <Badge variant={routing.fallback_enabled ? 'default' : 'secondary'}>
                  {routing.fallback_enabled ? 'Yes' : 'No'}
                </Badge>
              </div>
            )}
            {routing.cost_optimizer_enabled !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Cost Optimizer</span>
                <Badge variant={routing.cost_optimizer_enabled ? 'default' : 'secondary'}>
                  {routing.cost_optimizer_enabled ? 'Enabled' : 'Disabled'}
                </Badge>
              </div>
            )}
            {routing.max_attempts !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Max Attempts</span>
                <Badge variant="outline">{String(routing.max_attempts)}</Badge>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Benchmarking Section */}
      {benchmarking && (
        <div className="space-y-3">
          <h3 className="text-lg font-semibold">Benchmarking</h3>
          <div className="grid gap-3 md:grid-cols-2">
            {benchmarking.enabled !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Enabled</span>
                <Badge variant={benchmarking.enabled ? 'default' : 'secondary'}>
                  {benchmarking.enabled ? 'Yes' : 'No'}
                </Badge>
              </div>
            )}
            {benchmarking.interval_minutes !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Interval</span>
                <Badge variant="outline">{String(benchmarking.interval_minutes)} min</Badge>
              </div>
            )}
            {benchmarking.timeout_ms !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Timeout</span>
                <Badge variant="outline">{String(benchmarking.timeout_ms)} ms</Badge>
              </div>
            )}
            {benchmarking.max_concurrency !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Max Concurrency</span>
                <Badge variant="outline">{String(benchmarking.max_concurrency)}</Badge>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Semantic / Embeddings Section */}
      {semantic && (
        <div className="space-y-3">
          <h3 className="text-lg font-semibold">Semantic / Embeddings</h3>
          <div className="grid gap-3 md:grid-cols-2">
            {semantic.enabled !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Enabled</span>
                <Badge variant={semantic.enabled ? 'default' : 'secondary'}>
                  {semantic.enabled ? 'Yes' : 'No'}
                </Badge>
              </div>
            )}
            {semantic.embedding_model !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Embedding Model</span>
                <Badge variant="outline">{String(semantic.embedding_model)}</Badge>
              </div>
            )}
            {semantic.similarity_threshold !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Similarity Threshold</span>
                <Badge variant="outline">{String(semantic.similarity_threshold)}</Badge>
              </div>
            )}
            {semantic.cache_enabled !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Cache Enabled</span>
                <Badge variant={semantic.cache_enabled ? 'default' : 'secondary'}>
                  {semantic.cache_enabled ? 'Yes' : 'No'}
                </Badge>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Tool Routing Section */}
      {toolRouting && (
        <div className="space-y-3">
          <h3 className="text-lg font-semibold">Tool Routing</h3>
          <div className="grid gap-3 md:grid-cols-2">
            {toolRouting.enabled !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Enabled</span>
                <Badge variant={toolRouting.enabled ? 'default' : 'secondary'}>
                  {toolRouting.enabled ? 'Yes' : 'No'}
                </Badge>
              </div>
            )}
            {toolRouting.threshold !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Threshold</span>
                <Badge variant="outline">{String(toolRouting.threshold)}</Badge>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Budget / FinOps Section */}
      {budget && (
        <div className="space-y-3">
          <h3 className="text-lg font-semibold">Budget / FinOps</h3>
          <div className="grid gap-3 md:grid-cols-2">
            {budget.default_monthly_usd !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Default Monthly Budget</span>
                <Badge variant="outline">${String(budget.default_monthly_usd)}</Badge>
              </div>
            )}
            {budget.enforcement_enabled !== undefined && (
              <div className="flex items-center justify-between p-3 rounded-lg border">
                <span className="text-sm text-muted-foreground">Enforcement</span>
                <Badge variant={budget.enforcement_enabled ? 'default' : 'secondary'}>
                  {budget.enforcement_enabled ? 'Enabled' : 'Disabled'}
                </Badge>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
