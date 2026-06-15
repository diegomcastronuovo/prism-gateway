import { Badge } from '@/components/ui/badge'

interface ConfigStatusSectionProps {
  config: Record<string, unknown>
  version: number
}

export function ConfigStatusSection({ config, version }: ConfigStatusSectionProps) {
  const dynamicConfig = config.dynamic_config as Record<string, unknown> | undefined

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
      <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
        <span className="text-xs text-muted-foreground">Active Version</span>
        <Badge variant="outline" className="w-fit">v{version}</Badge>
      </div>
      
      <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
        <span className="text-xs text-muted-foreground">Dynamic Config</span>
        <Badge variant={dynamicConfig?.enabled ? 'default' : 'secondary'} className="w-fit">
          {dynamicConfig?.enabled ? 'Enabled' : 'Disabled'}
        </Badge>
      </div>

      {dynamicConfig?.source !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">Config Source</span>
          <span className="font-medium text-sm">{String(dynamicConfig.source)}</span>
        </div>
      )}

      {dynamicConfig?.refresh_interval_ms !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">Refresh Interval</span>
          <span className="font-medium text-sm">{Number(dynamicConfig.refresh_interval_ms)} ms</span>
        </div>
      )}

      {dynamicConfig?.cache_ttl_ms !== undefined && (
        <div className="flex flex-col gap-2 p-4 rounded-lg border bg-card">
          <span className="text-xs text-muted-foreground">Cache TTL</span>
          <span className="font-medium text-sm">{Number(dynamicConfig.cache_ttl_ms)} ms</span>
        </div>
      )}
    </div>
  )
}
