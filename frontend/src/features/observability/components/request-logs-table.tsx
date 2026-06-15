import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import type { RequestLogEntry } from '../types'

export type RequestLogSortField = 'timestamp' | 'tenant_id' | 'model' | 'provider' | 'latency_ms'
export type RequestLogSortDirection = 'asc' | 'desc'

interface RequestLogsTableProps {
  logs: RequestLogEntry[]
  sortField: RequestLogSortField
  sortDirection: RequestLogSortDirection
  onSortChange: (field: RequestLogSortField) => void
}

function formatTimestamp(timestamp: string): string {
  const date = new Date(timestamp)
  return date.toLocaleString()
}

function getStatusClassName(status: string): string {
  if (status === 'success' || status === 'ok') {
    return 'bg-green-100 text-green-800 border-green-200 hover:bg-green-100'
  }
  if (status === 'error') {
    return 'bg-red-100 text-red-800 border-red-200 hover:bg-red-100'
  }
  return ''
}

function SortHeader({
  label,
  field,
  sortField,
  sortDirection,
  onSortChange,
}: {
  label: string
  field: RequestLogSortField
  sortField: RequestLogSortField
  sortDirection: RequestLogSortDirection
  onSortChange: (field: RequestLogSortField) => void
}) {
  const active = sortField === field
  const indicator = active ? (sortDirection === 'asc' ? '↑' : '↓') : ''
  return (
    <button
      type="button"
      className="inline-flex items-center gap-1 font-medium hover:text-foreground"
      onClick={() => onSortChange(field)}
    >
      {label}
      <span className="text-xs text-muted-foreground">{indicator}</span>
    </button>
  )
}

export function RequestLogsTable({ logs, sortField, sortDirection, onSortChange }: RequestLogsTableProps) {
  return (
    <div className="border rounded-md">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>
              <SortHeader
                label="Timestamp"
                field="timestamp"
                sortField={sortField}
                sortDirection={sortDirection}
                onSortChange={onSortChange}
              />
            </TableHead>
            <TableHead>
              <SortHeader
                label="Tenant"
                field="tenant_id"
                sortField={sortField}
                sortDirection={sortDirection}
                onSortChange={onSortChange}
              />
            </TableHead>
            <TableHead>
              <SortHeader
                label="Model"
                field="model"
                sortField={sortField}
                sortDirection={sortDirection}
                onSortChange={onSortChange}
              />
            </TableHead>
            <TableHead>
              <SortHeader
                label="Provider"
                field="provider"
                sortField={sortField}
                sortDirection={sortDirection}
                onSortChange={onSortChange}
              />
            </TableHead>
            <TableHead>
              <SortHeader
                label="Latency (ms)"
                field="latency_ms"
                sortField={sortField}
                sortDirection={sortDirection}
                onSortChange={onSortChange}
              />
            </TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Fallback</TableHead>
            <TableHead>Cache</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {logs.length === 0 ? (
            <TableRow>
              <TableCell colSpan={8} className="text-center text-muted-foreground">
                No request logs available
              </TableCell>
            </TableRow>
          ) : (
            logs.map((log, index) => (
              <TableRow key={index}>
                <TableCell className="font-mono text-xs">
                  {formatTimestamp(log.timestamp)}
                </TableCell>
                <TableCell className="font-medium">{log.tenant_id}</TableCell>
                <TableCell>{log.model}</TableCell>
                <TableCell>{log.provider}</TableCell>
                <TableCell className="tabular-nums">{log.latency_ms}</TableCell>
                <TableCell>
                  <Badge className={getStatusClassName(log.status)} variant="outline">
                    {log.status}
                  </Badge>
                </TableCell>
                <TableCell>
                  {log.fallback_used ? (
                    <Badge variant="secondary">Yes</Badge>
                  ) : (
                    <span className="text-muted-foreground text-sm">-</span>
                  )}
                </TableCell>
                <TableCell>
                  {log.cache_status ? (
                    <Badge variant="outline">{log.cache_status.toUpperCase()}</Badge>
                  ) : (
                    <span className="text-muted-foreground text-sm">-</span>
                  )}
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  )
}
