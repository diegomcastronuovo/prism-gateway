import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/shared/empty-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { History } from 'lucide-react'
import { type GlobalConfigChange } from '../api/use-global-config'

interface GlobalConfigChangeLogProps {
  changes: GlobalConfigChange[] | undefined
  isLoading: boolean
  error: Error | null
}

export function GlobalConfigChangeLog({
  changes,
  isLoading,
  error,
}: GlobalConfigChangeLogProps) {
  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString()
  }

  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
        <p className="text-sm text-destructive">Failed to load change log</p>
        <p className="text-xs text-muted-foreground mt-1">{error.message}</p>
      </div>
    )
  }

  if (!changes || changes.length === 0) {
    return (
      <EmptyState
        icon={History}
        title="No changes recorded"
        description="Global configuration changes will appear here"
      />
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Version</TableHead>
          <TableHead>Changed At</TableHead>
          <TableHead>Changed By</TableHead>
          <TableHead>Summary</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {changes.map((change) => (
          <TableRow key={change.version}>
            <TableCell>
              <Badge variant="outline">v{change.version}</Badge>
            </TableCell>
            <TableCell className="text-sm">{formatDate(change.changed_at)}</TableCell>
            <TableCell className="text-sm">
              {change.changed_by || <span className="text-muted-foreground">Unknown</span>}
            </TableCell>
            <TableCell className="text-sm">
              {change.summary || <span className="text-muted-foreground">No summary</span>}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
