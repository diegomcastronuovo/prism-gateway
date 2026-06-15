'use client'

import { Badge } from '@/components/ui/badge'
import type { TenantRateLimit } from '../api/use-tenants'

interface TenantRateLimitSectionProps {
  rateLimit: TenantRateLimit | null | undefined
}

export function TenantRateLimitSection({ rateLimit }: TenantRateLimitSectionProps) {
  if (!rateLimit) {
    return <p className="text-sm text-muted-foreground">No rate limit configured</p>
  }

  return (
    <div className="space-y-2">
      <div className="flex justify-between items-center gap-4">
        <span className="text-sm text-muted-foreground">Requests per Minute (RPM)</span>
        <Badge variant="outline" className="tabular-nums">
          {rateLimit.rpm}
        </Badge>
      </div>
      <div className="flex justify-between items-center gap-4">
        <span className="text-sm text-muted-foreground">Burst</span>
        <Badge variant="outline" className="tabular-nums">
          {rateLimit.burst}
        </Badge>
      </div>
      <div className="flex justify-between items-center gap-4">
        <span className="text-sm text-muted-foreground">Scope</span>
        <Badge variant="outline" className="font-mono text-xs">
          {rateLimit.scope}
        </Badge>
      </div>
    </div>
  )
}
