import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Activity } from 'lucide-react'
import { formatCurrency } from '@/lib/utils/format'
import { type TenantUsage } from '../api/use-tenants'

interface TenantUsageSectionProps {
  usage: TenantUsage | undefined
  isLoading: boolean
  error: Error | null
}

export function TenantUsageSection({ usage, isLoading, error }: TenantUsageSectionProps) {
  const formatNumber = (num: number) => {
    return new Intl.NumberFormat().format(num)
  }

  // use shared formatCurrency from utils for consistent currency precision

  return (
    <div className="space-y-4">
      <h3 className="text-lg font-semibold">Usage</h3>

      {isLoading ? (
        <div className="grid grid-cols-3 gap-4">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      ) : error ? (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">Failed to load usage data</p>
          <p className="text-xs text-muted-foreground mt-1">{error.message}</p>
        </div>
      ) : !usage ? (
        <div className="text-center py-8 text-muted-foreground">
          <Activity className="h-8 w-8 mx-auto mb-2 opacity-50" />
          <p className="text-sm">No usage data available</p>
        </div>
      ) : (
        <div>
          <div className="flex items-center gap-2 mb-3">
            <span className="text-sm text-muted-foreground">Current Month:</span>
            <Badge variant="outline">{usage.month}</Badge>
          </div>
          <div className="grid grid-cols-3 gap-4">
            <div className="rounded-lg border p-4">
              <p className="text-sm text-muted-foreground mb-1">Requests</p>
              <p className="text-2xl font-bold">{formatNumber(usage.requests)}</p>
            </div>
            <div className="rounded-lg border p-4">
              <p className="text-sm text-muted-foreground mb-1">Tokens</p>
              <p className="text-2xl font-bold">{formatNumber(usage.tokens)}</p>
            </div>
            <div className="rounded-lg border p-4">
              <p className="text-sm text-muted-foreground mb-1">Cost</p>
              <p className="text-2xl font-bold">{formatCurrency(usage.cost_usd)}</p>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
