'use client'

import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Edit } from 'lucide-react'
import { RoutingInfraEditor } from '../editors/routing-infra-editor'

interface RoutingInfraSectionProps {
  config: Record<string, unknown>
  onUpdate?: (updated: Record<string, unknown>) => void
}

export function RoutingInfraSection({ config, onUpdate }: RoutingInfraSectionProps) {
  const [isEditing, setIsEditing] = useState(false)
  const rateLimit = config.rate_limit as Record<string, unknown> | undefined
  const circuitBreaker = config.circuit_breaker as Record<string, unknown> | undefined

  const handleSave = (updated: Record<string, unknown>) => {
    if (onUpdate) {
      onUpdate(updated)
    }
    setIsEditing(false)
  }

  const handleCancel = () => {
    setIsEditing(false)
  }

  if (isEditing) {
    return (
      <RoutingInfraEditor
        config={{
          smart_routing: config.smart_routing as Record<string, unknown>,
          rate_limit: config.rate_limit as Record<string, unknown>,
          circuit_breaker: config.circuit_breaker as Record<string, unknown>,
        }}
        onSave={handleSave}
        onCancel={handleCancel}
      />
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setIsEditing(true)}>
          <Edit className="mr-2 h-4 w-4" />
          Edit Routing Infrastructure
        </Button>
      </div>
      <div className="space-y-6">

      {/* Global Rate Limit Backend */}
      {rateLimit && (
        <div className="flex flex-col gap-3 p-4 rounded-lg border bg-card">
          <h4 className="font-semibold">Global Rate Limit Backend</h4>
          {(() => {
            const backend = rateLimit.Backend ?? rateLimit.backend
            const redis = (rateLimit.Redis ?? rateLimit.redis) as Record<string, unknown> | undefined
            const addr = redis?.Addr ?? redis?.addr
            const db = redis?.DB ?? redis?.db
            const keyPrefix = redis?.KeyPrefix ?? redis?.key_prefix
            const failOpen = redis?.FailOpen ?? redis?.fail_open
            const dialTimeout = redis?.DialTimeoutMs ?? redis?.dial_timeout_ms
            const opTimeout = redis?.OpTimeoutMs ?? redis?.op_timeout_ms
            return (
              <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
                {backend !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Backend</span>
                    <Badge variant="outline" className="w-fit">{String(backend)}</Badge>
                  </div>
                )}
                {addr !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Redis Address</span>
                    <span className="font-mono text-sm">{String(addr)}</span>
                  </div>
                )}
                {db !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">DB</span>
                    <span className="font-medium text-sm">{Number(db)}</span>
                  </div>
                )}
                {keyPrefix !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Key Prefix</span>
                    <span className="font-mono text-sm">{String(keyPrefix)}</span>
                  </div>
                )}
                {failOpen !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Fail Open</span>
                    <Badge variant={failOpen ? 'default' : 'secondary'} className="w-fit">
                      {failOpen ? 'Yes' : 'No'}
                    </Badge>
                  </div>
                )}
                {dialTimeout !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Dial Timeout</span>
                    <span className="font-medium text-sm">{Number(dialTimeout)} ms</span>
                  </div>
                )}
                {opTimeout !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Op Timeout</span>
                    <span className="font-medium text-sm">{Number(opTimeout)} ms</span>
                  </div>
                )}
              </div>
            )
          })()}
        </div>
      )}

      {/* Circuit Breaker Defaults */}
      {circuitBreaker && (
        <div className="flex flex-col gap-3 p-4 rounded-lg border bg-card">
          <h4 className="font-semibold">Circuit Breaker Defaults</h4>
          {(() => {
            const backend = circuitBreaker.Backend ?? circuitBreaker.backend
            const defaults = (circuitBreaker.Defaults ?? circuitBreaker.defaults) as Record<string, unknown> | undefined
            const enabled = defaults?.Enabled ?? defaults?.enabled
            const redis = (circuitBreaker.Redis ?? circuitBreaker.redis) as Record<string, unknown> | undefined
            const addr = redis?.Addr ?? redis?.addr
            const db = redis?.DB ?? redis?.db
            const keyPrefix = redis?.KeyPrefix ?? redis?.key_prefix
            const failOpen = redis?.FailOpen ?? redis?.fail_open
            const dialTimeout = redis?.DialTimeoutMs ?? redis?.dial_timeout_ms
            const opTimeout = redis?.OpTimeoutMs ?? redis?.op_timeout_ms
            return (
              <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
                {backend !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Backend</span>
                    <Badge variant="outline" className="w-fit">{String(backend)}</Badge>
                  </div>
                )}
                {enabled !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Enabled</span>
                    <Badge variant={enabled ? 'default' : 'secondary'} className="w-fit">
                      {enabled ? 'Yes' : 'No'}
                    </Badge>
                  </div>
                )}
                {addr !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Redis Address</span>
                    <span className="font-mono text-sm">{String(addr)}</span>
                  </div>
                )}
                {db !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">DB</span>
                    <span className="font-medium text-sm">{Number(db)}</span>
                  </div>
                )}
                {keyPrefix !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Key Prefix</span>
                    <span className="font-mono text-sm">{String(keyPrefix)}</span>
                  </div>
                )}
                {failOpen !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Fail Open</span>
                    <Badge variant={failOpen ? 'default' : 'secondary'} className="w-fit">
                      {failOpen ? 'Yes' : 'No'}
                    </Badge>
                  </div>
                )}
                {dialTimeout !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Dial Timeout</span>
                    <span className="font-medium text-sm">{Number(dialTimeout)} ms</span>
                  </div>
                )}
                {opTimeout !== undefined && (
                  <div className="flex flex-col gap-1">
                    <span className="text-xs text-muted-foreground">Op Timeout</span>
                    <span className="font-medium text-sm">{Number(opTimeout)} ms</span>
                  </div>
                )}
              </div>
            )
          })()}
        </div>
      )}
      </div>
    </div>
  )
}
