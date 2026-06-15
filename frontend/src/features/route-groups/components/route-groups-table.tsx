'use client'

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { GitBranch } from 'lucide-react'
import type { RouteGroup } from '../api/use-route-groups'

interface RouteGroupsTableProps {
  routeGroups: RouteGroup[]
  selectedRouteGroupId: string | null
  onSelectRouteGroup: (id: string) => void
}

export function RouteGroupsTable({
  routeGroups,
  selectedRouteGroupId,
  onSelectRouteGroup,
}: RouteGroupsTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Route Group ID</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Models Count</TableHead>
          <TableHead>Preview</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {routeGroups.map((group) => {
          const models = group.models || []
          const previewModels = models.slice(0, 2)
          const remainingCount = Math.max(0, models.length - 2)

          return (
            <TableRow
              key={group.id}
              className={`cursor-pointer ${selectedRouteGroupId === group.id ? 'bg-muted' : ''}`}
              onClick={() => onSelectRouteGroup(group.id)}
            >
              <TableCell>
                <div className="flex items-center gap-2">
                  <GitBranch className="h-4 w-4 text-muted-foreground" />
                  <span className="font-medium">{group.id}</span>
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="outline">Active</Badge>
              </TableCell>
              <TableCell>
                <span className="text-sm text-muted-foreground">{models.length}</span>
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-1">
                  {previewModels.map((model) => (
                    <Badge key={model} variant="secondary" className="text-xs">
                      {model}
                    </Badge>
                  ))}
                  {remainingCount > 0 && (
                    <Badge variant="secondary" className="text-xs">
                      +{remainingCount} more
                    </Badge>
                  )}
                  {models.length === 0 && (
                    <span className="text-xs text-muted-foreground">No models</span>
                  )}
                </div>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
