import { Badge } from '@/components/ui/badge'
import { type ExternalPiiConfig } from '../api/use-tenants'

interface TenantSecuritySectionProps {
  externalPii?: ExternalPiiConfig
}

export function TenantSecuritySection({ externalPii }: TenantSecuritySectionProps) {
  const enabled = externalPii?.enabled === true

  if (!enabled) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">PII Protection and external validation hooks</p>
        <div className="rounded-lg border p-4">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">External PII Webhook</span>
            <Badge variant="secondary">Disabled</Badge>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">PII Protection and external validation hooks</p>
      <div className="rounded-lg border p-4 space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">External PII Webhook</span>
          <Badge>Enabled</Badge>
        </div>

        <div className="grid gap-2 text-sm">
          <div className="flex items-center justify-between gap-4">
            <span className="text-muted-foreground">Request URL</span>
            <span className="font-mono text-xs break-all text-right">{externalPii?.request_url || '—'}</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="text-muted-foreground">Response URL</span>
            <span className="font-mono text-xs break-all text-right">{externalPii?.response_url || '—'}</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="text-muted-foreground">Timeout (ms)</span>
            <Badge variant="outline">{externalPii?.timeout_ms ?? 3000} ms</Badge>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="text-muted-foreground">Failure Policy</span>
            <Badge variant="outline">{externalPii?.failure_policy === 'deny' ? 'Deny' : 'Accept'}</Badge>
          </div>
        </div>
      </div>
    </div>
  )
}
