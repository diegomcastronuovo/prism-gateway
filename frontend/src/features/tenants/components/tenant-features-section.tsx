import { Badge } from '@/components/ui/badge'

interface TenantFeaturesSectionProps {
  config: Record<string, unknown>
}

export function TenantFeaturesSection({ config }: TenantFeaturesSectionProps) {
  const getSemanticRoutingEnabled = (): boolean => {
    const routing = config.routing as Record<string, unknown> | undefined
    const smart = routing?.smart as Record<string, unknown> | undefined
    const stages = smart?.stages as Array<Record<string, unknown>> | undefined

    if (!Array.isArray(stages)) {
      return false
    }

    const semanticStage = stages.find((stage) => stage?.name === 'semantic_intent')
    if (!semanticStage) {
      return false
    }

    const rules = semanticStage.rules as unknown
    return Array.isArray(rules) && rules.length > 0
  }

  const isSemanticCacheEnabled = (config.semantic_cache as Record<string, unknown> | undefined)?.enabled === true
  const toolRoutingValue = config.tool_routing_enabled as boolean | null | undefined
  const isToolRoutingEnabled = toolRoutingValue === false ? false : true
  const isBudgetEnforcementEnabled = (config.budget_enforcement as Record<string, unknown> | undefined)?.enabled === true

  const features = [
    { name: 'Semantic Routing', isEnabled: getSemanticRoutingEnabled() },
    { name: 'Semantic Cache', isEnabled: isSemanticCacheEnabled },
    { name: 'Tool Routing', isEnabled: isToolRoutingEnabled },
    { name: 'Budget Enforcement', isEnabled: isBudgetEnforcementEnabled },
  ]

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3">
        {features.map((feature) => (
          <div
            key={feature.name}
            className="flex items-center justify-between p-3 rounded-lg border"
          >
            <span className="text-sm font-medium">{feature.name}</span>
            <Badge variant={feature.isEnabled ? 'default' : 'secondary'}>
              {feature.isEnabled ? 'Enabled' : 'Disabled'}
            </Badge>
          </div>
        ))}
      </div>
    </div>
  )
}
