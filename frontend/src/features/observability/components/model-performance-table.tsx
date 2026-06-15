import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { ModelPerformance } from '../types/real-backend'

interface ModelPerformanceTableProps {
  models: ModelPerformance[] | ModelPerformance
}

export function ModelPerformanceTable({ models }: ModelPerformanceTableProps) {
  // Ensure models is always an array
  const modelsArray = Array.isArray(models) ? models : (models ? [models] : [])

  const formatNumber = (value: unknown, digits = 0) => {
    const n = Number(value)
    return Number.isFinite(n) ? n.toFixed(digits) : '0'
  }
  
  return (
    <div className="border rounded-md">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Model</TableHead>
            <TableHead>Provider</TableHead>
            <TableHead>Avg Latency (ms)</TableHead>
            <TableHead>P95 Latency (ms)</TableHead>
            <TableHead>Success Rate</TableHead>
            <TableHead>Avg Cost (USD)</TableHead>
            <TableHead>Samples</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {modelsArray.length === 0 ? (
            <TableRow>
              <TableCell colSpan={7} className="text-center text-muted-foreground">
                No model performance data available
              </TableCell>
            </TableRow>
          ) : (
            modelsArray.map((model, idx) => (
              <TableRow key={`${model.model}-${model.provider}-${idx}`}>
                <TableCell className="font-medium">{model.model}</TableCell>
                <TableCell>{model.provider}</TableCell>
                <TableCell className="tabular-nums">{formatNumber(model.avg_latency_ms, 0)}</TableCell>
                <TableCell className="tabular-nums">{formatNumber(model.p95_latency_ms, 0)}</TableCell>
                <TableCell className="tabular-nums">{formatNumber(Number(model.success_rate) * 100, 1)}%</TableCell>
                <TableCell className="tabular-nums">${formatNumber(model.avg_cost_usd, 4)}</TableCell>
                <TableCell className="tabular-nums">{Math.floor(Number(model.samples) || 0).toLocaleString()}</TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  )
}
