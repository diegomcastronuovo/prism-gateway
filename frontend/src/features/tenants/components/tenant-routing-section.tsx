import { Badge } from '@/components/ui/badge'

const STRATEGY_LABELS: Record<string, string> = {
  smart:        'Smart',
  round_robin:  'Round Robin',
  latency:      'Latency',
  cost:         'Cost',
  header:       'Header',
  decision_ops: 'DecisionOps',
}

interface TrafficSplitConfig {
  [key: string]: Array<{
    model: string
    weight: number
  }>
}

interface TenantRoutingSectionProps {
  routingStrategy?: string
  defaultRouteGroup?: string | null
  allowedModels?: string[]
  routeGroups?: string[]
  trafficSplit?: TrafficSplitConfig
}

export function TenantRoutingSection({
  routingStrategy,
  defaultRouteGroup,
  allowedModels,
  routeGroups,
  trafficSplit,
}: TenantRoutingSectionProps) {
  return (
    <div className="space-y-4">
      {/* Routing Strategy */}
      <div>
        <h4 className="text-sm font-medium text-muted-foreground mb-2">Routing Strategy</h4>
        {routingStrategy ? (
          <Badge
            variant="outline"
            className={routingStrategy === 'decision_ops'
              ? 'border-violet-500 text-violet-700 dark:text-violet-400'
              : undefined}
          >
            {STRATEGY_LABELS[routingStrategy] ?? routingStrategy}
          </Badge>
        ) : (
          <p className="text-sm text-muted-foreground">Not configured</p>
        )}
      </div>

      {/* Default Route Group — hidden for decision_ops */}
      {routingStrategy !== 'decision_ops' && (
        <div>
          <h4 className="text-sm font-medium text-muted-foreground mb-2">Default Route Group</h4>
          {defaultRouteGroup ? (
            <Badge variant="outline">{defaultRouteGroup}</Badge>
          ) : (
            <p className="text-sm text-muted-foreground">None (all allowed models)</p>
          )}
        </div>
      )}

      {/* Allowed Models */}
      {allowedModels && allowedModels.length > 0 && (
        <div>
          <h4 className="text-sm font-medium text-muted-foreground mb-2">Allowed Models</h4>
          <div className="flex flex-wrap gap-2">
            {allowedModels.map((model) => (
              <Badge key={model} variant="secondary">
                {model}
              </Badge>
            ))}
          </div>
        </div>
      )}

      {/* Route Groups */}
      {routeGroups && routeGroups.length > 0 && (
        <div>
          <h4 className="text-sm font-medium text-muted-foreground mb-2">Route Groups</h4>
          <div className="flex flex-wrap gap-2">
            {routeGroups.map((group) => (
              <Badge key={group} variant="secondary">
                {group}
              </Badge>
            ))}
          </div>
        </div>
      )}

      {/* Traffic Split */}
      {trafficSplit && Object.keys(trafficSplit).length > 0 ? (
        <div>
          <h4 className="text-sm font-medium text-muted-foreground mb-2">Traffic Split</h4>
          <div className="space-y-3">
            {Object.entries(trafficSplit).map(([name, splits]) => (
              <div key={name} className="rounded-lg border p-3">
                <p className="text-sm font-medium mb-2">{name}</p>
                <div className="space-y-1">
                  {splits.map((split, idx) => (
                    <div key={idx} className="flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">{split.model}</span>
                      <Badge variant="outline">{split.weight}%</Badge>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div>
          <h4 className="text-sm font-medium text-muted-foreground mb-2">Traffic Split</h4>
          <p className="text-sm text-muted-foreground">No traffic split configured</p>
        </div>
      )}
    </div>
  )
}
