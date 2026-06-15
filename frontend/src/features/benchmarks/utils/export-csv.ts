import type { BenchmarkModelAggregate } from '../api/use-benchmarks'

function getHealthStatus(successRate: number): 'healthy' | 'degraded' | 'failing' {
  if (successRate >= 0.99) return 'healthy'
  if (successRate >= 0.8) return 'degraded'
  return 'failing'
}

function escapeCSVValue(value: string | number): string {
  const stringValue = String(value)
  // If value contains comma, quote, or newline, wrap in quotes and escape internal quotes
  if (stringValue.includes(',') || stringValue.includes('"') || stringValue.includes('\n')) {
    return `"${stringValue.replace(/"/g, '""')}"`
  }
  return stringValue
}

function formatDate(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function getWindowLabel(windowHours: number): string {
  if (windowHours === 1) return '1h'
  if (windowHours === 6) return '6h'
  if (windowHours === 24) return '24h'
  if (windowHours === 72) return '72h'
  if (windowHours === 168) return '168h'
  return `${windowHours}h`
}

export function exportBenchmarksToCSV(
  rows: BenchmarkModelAggregate[],
  windowHours: number
): void {
  if (rows.length === 0) {
    return
  }

  const generatedAt = new Date().toISOString()

  // CSV header (final order per spec)
  const headers = [
    'generated_at',
    'window_hours',
    'model',
    'provider',
    'avg_latency_ms',
    'p95_latency_ms',
    'success_rate',
    'avg_cost_usd',
    'samples',
    'status',
  ]

  // CSV rows
  const csvRows = rows.map((row) => {
    const status = getHealthStatus(row.success_rate)
    // Round values per spec
    const avgLatency = Number.isFinite(row.avg_latency_ms)
      ? Number(row.avg_latency_ms).toFixed(2)
      : ''
    const p95Latency = Number.isFinite(row.p95_latency_ms)
      ? Number(row.p95_latency_ms).toFixed(2)
      : ''
    const successRate = Number.isFinite(row.success_rate)
      ? Number(row.success_rate).toFixed(3)
      : ''
    const avgCost = Number.isFinite(row.avg_cost_usd)
      ? Number(row.avg_cost_usd).toFixed(6)
      : ''
    return [
      escapeCSVValue(generatedAt),
      escapeCSVValue(windowHours),
      escapeCSVValue(row.model),
      escapeCSVValue(row.provider),
      escapeCSVValue(avgLatency),
      escapeCSVValue(p95Latency),
      escapeCSVValue(successRate),
      escapeCSVValue(avgCost),
      escapeCSVValue(row.samples),
      escapeCSVValue(status),
    ].join(',')
  })

  // Combine header and rows
  const csvContent = [headers.join(','), ...csvRows].join('\n')

  // Create blob and trigger download
  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  
  const windowLabel = getWindowLabel(windowHours)
  const dateStr = formatDate(new Date())
  const filename = `benchmarks_${windowLabel}_${dateStr}.csv`
  
  link.setAttribute('href', url)
  link.setAttribute('download', filename)
  link.style.visibility = 'hidden'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}
