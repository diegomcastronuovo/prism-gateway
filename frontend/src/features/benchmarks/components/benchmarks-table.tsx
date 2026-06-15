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
import type { BenchmarkModelAggregate } from '../api/use-benchmarks'

interface BenchmarksTableProps {
  rows: BenchmarkModelAggregate[]
}

function getHealthStatus(successRate: number) {
  if (successRate >= 0.99) return 'healthy'
  if (successRate >= 0.8) return 'degraded'
  return 'failing'
}

function getStatusVariant(status: string): 'default' | 'secondary' | 'destructive' {
  if (status === 'healthy') return 'default'
  if (status === 'degraded') return 'secondary'
  return 'destructive'
}

function formatLatency(value: number) {
  return `${Math.round(value).toLocaleString()} ms`
}

function formatSuccessRate(value: number) {
  return `${(value * 100).toFixed(1)}%`
}

function formatCost(value: number) {
  return `$${value.toFixed(6)}`
}

export function BenchmarksTable({ rows }: BenchmarksTableProps) {
  const sortedRows = [...rows].sort((a, b) => {
    if (b.success_rate !== a.success_rate) return b.success_rate - a.success_rate
    return a.avg_latency_ms - b.avg_latency_ms
  })

  return (
    <div className="border rounded-md">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Model</TableHead>
            <TableHead>Provider</TableHead>
            <TableHead className="text-right">Avg Latency</TableHead>
            <TableHead className="text-right">P95 Latency</TableHead>
            <TableHead className="text-right">Success Rate</TableHead>
            <TableHead className="text-right">Avg Cost</TableHead>
            <TableHead className="text-right">Samples</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sortedRows.map((row) => {
            const status = getHealthStatus(row.success_rate)

            return (
              <TableRow key={row.model}>
                <TableCell className="font-medium">{row.model}</TableCell>
                <TableCell>{row.provider}</TableCell>
                <TableCell className="text-right">{formatLatency(row.avg_latency_ms)}</TableCell>
                <TableCell className="text-right">{formatLatency(row.p95_latency_ms)}</TableCell>
                <TableCell className="text-right">{formatSuccessRate(row.success_rate)}</TableCell>
                <TableCell className="text-right">{formatCost(row.avg_cost_usd)}</TableCell>
                <TableCell className="text-right">{row.samples.toLocaleString()}</TableCell>
                <TableCell>
                  <Badge variant={getStatusVariant(status)}>{status}</Badge>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
