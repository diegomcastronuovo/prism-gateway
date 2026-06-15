import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { CostAnalytics } from '../types'

interface CostAnalyticsTableProps {
  analytics: CostAnalytics[]
}

function formatCurrency(value: number): string {
  return `$${value.toFixed(2)}`
}

export function CostAnalyticsTable({ analytics }: CostAnalyticsTableProps) {
  return (
    <div className="border rounded-md">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Model</TableHead>
            <TableHead>Requests</TableHead>
            <TableHead>Avg Latency (ms)</TableHead>
            <TableHead>Total Cost</TableHead>
            <TableHead>Success Rate</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {analytics.length === 0 ? (
            <TableRow>
              <TableCell colSpan={5} className="text-center text-muted-foreground">
                No cost analytics available
              </TableCell>
            </TableRow>
          ) : (
            analytics.map((item) => (
              <TableRow key={item.model}>
                <TableCell className="font-medium">{item.model}</TableCell>
                <TableCell className="tabular-nums">{item.requests.toLocaleString()}</TableCell>
                <TableCell className="tabular-nums">{item.avg_latency}</TableCell>
                <TableCell className="tabular-nums">{formatCurrency(item.total_cost)}</TableCell>
                <TableCell className="tabular-nums">{item.success_rate.toFixed(1)}%</TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  )
}
