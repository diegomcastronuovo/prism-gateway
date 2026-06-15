import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'
import { getFirstValue } from '@/features/dashboard/lib/config-utils'

interface FeatureFlagsPanelProps {
  config?: Record<string, unknown>
}

export function FeatureFlagsPanel({ config }: FeatureFlagsPanelProps) {
  const featureList = [
    {
      key: 'budget_enforcement',
      label: 'Budget Enforcement',
      value: getFirstValue(config, ['budget.enforcement_enabled', 'budget_enforcement']),
    },
    {
      key: 'dynamic_routes',
      label: 'Dynamic Routes',
      value: getFirstValue(config, ['routing.dynamic_routes', 'routing.dynamic_routes_enabled', 'dynamic_routes']),
    },
    {
      key: 'semantic_cache',
      label: 'Semantic Cache',
      value: getFirstValue(config, ['semantic.cache_enabled', 'semantic_cache']),
    },
    {
      key: 'semantic_routing',
      label: 'Semantic Routing',
      value: getFirstValue(config, ['semantic.enabled', 'semantic_routing']),
    },
    {
      key: 'tool_routing',
      label: 'Tool Routing',
      value: getFirstValue(config, ['tool_routing.enabled', 'tool_routing_enabled']),
    },
  ]

  return (
    <SectionCard title="Feature Flags" description="Enabled platform features" className="border-t-4 border-t-cyan-400">
      <div className="grid grid-cols-2 gap-4">
        {featureList.map((feature) => {
          const isEnabled = feature.value === undefined || feature.value === null ? null : Boolean(feature.value)
          return (
            <div key={feature.key} className="flex items-center justify-between p-3 rounded-lg border">
              <span className="text-sm font-medium">{feature.label}</span>
              {isEnabled === null ? (
                <span className="text-sm text-muted-foreground">Not available</span>
              ) : (
                <Badge variant={isEnabled ? 'default' : 'secondary'}>
                  {isEnabled ? 'Enabled' : 'Disabled'}
                </Badge>
              )}
            </div>
          )
        })}
      </div>
    </SectionCard>
  )
}
