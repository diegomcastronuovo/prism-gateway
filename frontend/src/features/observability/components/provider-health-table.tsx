import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { ProviderHealth } from '../types'

interface ProviderHealthTableProps {
  providers: ProviderHealth[]
}

export function ProviderHealthTable({ providers }: ProviderHealthTableProps) {
  return (
    <div className="border rounded-md">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Provider</TableHead>
            <TableHead>Success Rate</TableHead>
            <TableHead>Avg Latency (ms)</TableHead>
            <TableHead>Total Requests</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {providers.length === 0 ? (
            <TableRow>
              <TableCell colSpan={4} className="text-center text-muted-foreground">
                No provider data available
              </TableCell>
            </TableRow>
          ) : (
            providers.map((provider) => (
              <TableRow key={provider.provider}>
                <TableCell className="font-medium">{provider.provider}</TableCell>
                <TableCell className="tabular-nums">{provider.success_rate.toFixed(1)}%</TableCell>
                <TableCell className="tabular-nums">{provider.avg_latency}</TableCell>
                <TableCell className="tabular-nums">{provider.total_requests.toLocaleString()}</TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  )
}
