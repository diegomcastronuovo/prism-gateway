'use client'

import { useEffect } from 'react'
import { SectionCard } from '@/components/shared/section-card'
import { Button } from '@/components/ui/button'
import { useSystemHealth, type HealthStatus, type TableHealthStatus } from '../api/use-system-health'
import { CheckCircle, XCircle, MinusCircle, Loader2, RefreshCw } from 'lucide-react'

function StatusIcon({ status }: { status: string }) {
  if (status === 'loading') return <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
  if (status === 'ok') return <CheckCircle className="h-4 w-4 text-green-500" />
  if (status === 'disabled') return <MinusCircle className="h-4 w-4 text-muted-foreground" />
  return <XCircle className="h-4 w-4 text-destructive" />
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    ok: 'bg-green-500/10 text-green-600 dark:text-green-400',
    error: 'bg-destructive/10 text-destructive',
    disabled: 'bg-muted text-muted-foreground',
    loading: 'bg-muted text-muted-foreground',
  }
  return (
    <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${colors[status] ?? colors.error}`}>
      {status}
    </span>
  )
}

function CheckRow({
  label,
  item,
}: {
  label: string
  item: HealthStatus | TableHealthStatus
}) {
  const tableItem = item as TableHealthStatus
  return (
    <div className="flex items-start gap-3 py-2 border-b last:border-0">
      <StatusIcon status={item.status} />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">{label}</span>
          <StatusBadge status={item.status} />
        </div>
        {item.error && (
          <p className="text-xs text-destructive mt-0.5 truncate">{item.error}</p>
        )}
        {tableItem.expected !== undefined && (
          <p className="text-xs text-muted-foreground mt-0.5">
            {tableItem.found} / {tableItem.expected} tables found
            {tableItem.missing && tableItem.missing.length > 0 && (
              <span className="text-destructive"> — missing: {tableItem.missing.join(', ')}</span>
            )}
          </p>
        )}
      </div>
    </div>
  )
}

export function SystemHealthCard() {
  const { data, isLoading, error, run } = useSystemHealth()

  useEffect(() => {
    run()
  }, [run])

  const loadingItem: HealthStatus = { status: 'loading' }

  return (
    <SectionCard
      title="System Health"
      className="border-t-4 border-t-green-500"
      action={
        <Button variant="outline" size="sm" onClick={run} disabled={isLoading}>
          <RefreshCw className={`h-3.5 w-3.5 mr-1.5 ${isLoading ? 'animate-spin' : ''}`} />
          Health Check
        </Button>
      }
    >
      {error && !data && (
        <div className="text-sm text-destructive py-2">{error}</div>
      )}
      <div className="divide-y-0">
        <CheckRow label="Gateway" item={isLoading && !data ? loadingItem : (data?.gateway ?? loadingItem)} />
        <CheckRow label="PostgreSQL" item={isLoading && !data ? loadingItem : (data?.postgres ?? loadingItem)} />
        <CheckRow label="Tables" item={isLoading && !data ? loadingItem : (data?.tables ?? loadingItem)} />
        <CheckRow label="Redis" item={isLoading && !data ? loadingItem : (data?.redis ?? loadingItem)} />
        <CheckRow label="Keycloak" item={isLoading && !data ? loadingItem : (data?.keycloak ?? loadingItem)} />
      </div>
    </SectionCard>
  )
}
