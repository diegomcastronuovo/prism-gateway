import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { BarChart3 } from 'lucide-react'

interface Benchmark {
  model: string
  provider: string
  avg_latency_ms: number
  p95_latency_ms: number
  success_rate: number
  avg_cost_usd: number
  samples: number
}

interface BenchmarksTableProps {
  benchmarks: Benchmark[]
}

export function BenchmarksTable({ benchmarks }: BenchmarksTableProps) {
  if (!benchmarks || benchmarks.length === 0) {
    return (
      <SectionCard
        title="Third-Party Model Benchmarks"
        description="Performance metrics (last 24 hours)"
        className="border-t-4 border-t-blue-500"
      >
        <EmptyState
          icon={BarChart3}
          title="No benchmark data"
          description="Benchmark data will appear here once models are tested"
        />
      </SectionCard>
    )
  }

  return (
    <SectionCard
      title="Third-Party Model Benchmarks"
      description="Performance metrics (last 24 hours)"
      className="border-t-4 border-t-blue-500"
    >
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
          </TableRow>
        </TableHeader>
        <TableBody>
          {benchmarks.map((benchmark, index) => (
            <TableRow key={`${benchmark.model}-${index}`}>
              <TableCell className="font-medium">{benchmark.model}</TableCell>
              <TableCell>{benchmark.provider}</TableCell>
              <TableCell className="text-right">{Math.round(benchmark.avg_latency_ms)}ms</TableCell>
              <TableCell className="text-right">{Math.round(benchmark.p95_latency_ms)}ms</TableCell>
              <TableCell className="text-right">{(benchmark.success_rate * 100).toFixed(1)}%</TableCell>
              <TableCell className="text-right">${benchmark.avg_cost_usd.toFixed(6)}</TableCell>
              <TableCell className="text-right">{benchmark.samples}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </SectionCard>
  )
}
