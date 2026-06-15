'use client'

import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Layers } from 'lucide-react'
import type { Model, ModelBenchmark } from '../api/use-models'

interface ModelsTableProps {
  models: Model[] | undefined
  benchmarks: ModelBenchmark[] | undefined
  isLoading: boolean
  selectedModelId: string | null
  onSelectModel: (modelId: string) => void
  searchQuery: string
}


function normalizeType(type: string | undefined): string {
  if (type === 'embedding') return 'Embedding'
  if (type === 'ml') return 'ML'
  return 'LLM'
}

function formatInfraCost(value: number | undefined): string {
  const amount = Number(value ?? 0)
  if (!Number.isFinite(amount)) return '$0'
  if (Number.isInteger(amount)) return `$${amount}`
  return `$${amount.toFixed(2)}`
}

export function ModelsTable({
  models,
  benchmarks,
  isLoading,
  selectedModelId,
  onSelectModel,
  searchQuery,
}: ModelsTableProps) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {[...Array(5)].map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    )
  }

  const filteredModels = models?.filter(
    (model) =>
      model.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
      model.provider.toLowerCase().includes(searchQuery.toLowerCase())
  )

  if (!filteredModels || filteredModels.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        {searchQuery ? 'No models match your search' : 'No models configured'}
      </div>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Model ID</TableHead>
          <TableHead>Provider</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Route Groups</TableHead>
          <TableHead>Infra Cost / Month</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {filteredModels.map((model) => {
          const isSelected = selectedModelId === model.id
          const rawEnabled = (model as Record<string, unknown>)['Enabled'] ?? (model as Record<string, unknown>)['enabled']
          const isEnabled = rawEnabled === undefined || rawEnabled === null ? true : Boolean(rawEnabled)

          return (
            <TableRow
              key={model.id}
              className={`cursor-pointer ${
                isSelected ? 'bg-primary/10' : 'hover:bg-muted/50'
              }`}
              onClick={() => onSelectModel(model.id)}
            >
              <TableCell className="font-medium">
                <div className="flex items-center gap-2">
                  <Layers className="h-4 w-4 text-muted-foreground" />
                  {model.id}
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="outline">{model.provider}</Badge>
              </TableCell>
              <TableCell>
                <Badge variant="secondary">{normalizeType(model.type)}</Badge>
              </TableCell>
              <TableCell>
                {model.route_groups && model.route_groups.length > 0 ? (
                  <div className="flex flex-wrap gap-1">
                    {model.route_groups.slice(0, 2).map((group) => (
                      <Badge key={group} variant="secondary" className="text-xs">
                        {group}
                      </Badge>
                    ))}
                    {model.route_groups.length > 2 && (
                      <Badge variant="secondary" className="text-xs">
                        +{model.route_groups.length - 2}
                      </Badge>
                    )}
                  </div>
                ) : (
                  <span className="text-sm text-muted-foreground">None</span>
                )}
              </TableCell>
              <TableCell className="text-sm tabular-nums">
                {formatInfraCost(model.infrastructure_monthly_usd)}
              </TableCell>
              <TableCell>
                <Badge
                  variant="outline"
                  className={isEnabled
                    ? 'bg-green-100 text-green-800 border-green-200 dark:bg-green-900/30 dark:text-green-400 dark:border-green-800'
                    : 'bg-red-100 text-red-800 border-red-200 dark:bg-red-900/30 dark:text-red-400 dark:border-red-800'
                  }
                >
                  {isEnabled ? 'Enabled' : 'Disabled'}
                </Badge>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
