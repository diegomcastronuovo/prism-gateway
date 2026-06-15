'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface RedisConfig {
  Addr?: string
  DB?: number
  KeyPrefix?: string
  FailOpen?: boolean
  DialTimeoutMs?: number
  OpTimeoutMs?: number
  Password?: string
}

interface CircuitBreakerDefaults {
  Enabled?: boolean
  WindowSeconds?: number
  MinRequests?: number
  FailureRateThreshold?: number
  OpenCooldownSeconds?: number
  HalfOpenMaxInflight?: number
  HalfOpenSuccessesToClose?: number
  BucketSizeSeconds?: number
}

interface CircuitBreaker {
  Backend?: string
  Redis?: RedisConfig
  Defaults?: CircuitBreakerDefaults
}

interface RateLimit {
  Backend?: string
  Redis?: RedisConfig
}



interface RoutingInfraConfig {
  rate_limit?: RateLimit
  circuit_breaker?: CircuitBreaker
}

interface RoutingInfraEditorProps {
  config: Record<string, unknown>
  onSave: (updated: Record<string, unknown>) => void
  onCancel: () => void
}

export function RoutingInfraEditor({ config, onSave, onCancel }: RoutingInfraEditorProps) {
  const [editedConfig, setEditedConfig] = useState<RoutingInfraConfig>(
    JSON.parse(JSON.stringify(config)) as RoutingInfraConfig
  )



  const handleRateLimitChange = (field: keyof RateLimit, value: string) => {
    setEditedConfig((prev) => ({
      ...prev,
      rate_limit: {
        ...prev.rate_limit,
        [field]: value,
      },
    }))
  }

  const handleRateLimitRedisChange = (field: keyof RedisConfig, value: string | number | boolean) => {
    setEditedConfig((prev) => ({
      ...prev,
      rate_limit: {
        ...prev.rate_limit,
        Redis: {
          ...prev.rate_limit?.Redis,
          [field]: value,
        },
      },
    }))
  }

  const handleCircuitBreakerChange = (field: keyof CircuitBreaker, value: string) => {
    setEditedConfig((prev) => ({
      ...prev,
      circuit_breaker: {
        ...prev.circuit_breaker,
        [field]: value,
      },
    }))
  }

  const handleCircuitBreakerRedisChange = (field: keyof RedisConfig, value: string | number | boolean) => {
    setEditedConfig((prev) => ({
      ...prev,
      circuit_breaker: {
        ...prev.circuit_breaker,
        Redis: {
          ...prev.circuit_breaker?.Redis,
          [field]: value,
        },
      },
    }))
  }

  const handleCircuitBreakerDefaultsChange = (field: keyof CircuitBreakerDefaults, value: string | number | boolean) => {
    setEditedConfig((prev) => ({
      ...prev,
      circuit_breaker: {
        ...prev.circuit_breaker,
        Defaults: {
          ...prev.circuit_breaker?.Defaults,
          [field]: value,
        },
      },
    }))
  }

  const handleSave = () => {
    onSave(editedConfig as unknown as Record<string, unknown>)
  }

  return (
    <div className="space-y-6">

      {/* Rate Limit Backend */}
      <div className="p-4 rounded-lg border bg-card space-y-4">
        <h3 className="font-semibold">Global Rate Limit Backend</h3>
        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
          <div className="space-y-1">
            <Label className="text-xs">Backend</Label>
            <Input
              value={editedConfig.rate_limit?.Backend || ''}
              onChange={(e) => handleRateLimitChange('Backend', e.target.value)}
              className="h-8 text-sm"
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Redis Address</Label>
            <Input
              value={editedConfig.rate_limit?.Redis?.Addr || ''}
              onChange={(e) => handleRateLimitRedisChange('Addr', e.target.value)}
              className="h-8 text-sm font-mono"
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Redis DB</Label>
            <Input
              type="number"
              value={editedConfig.rate_limit?.Redis?.DB ?? 0}
              onChange={(e) => handleRateLimitRedisChange('DB', parseInt(e.target.value) || 0)}
              className="h-8 text-sm"
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Key Prefix</Label>
            <Input
              value={editedConfig.rate_limit?.Redis?.KeyPrefix || ''}
              onChange={(e) => handleRateLimitRedisChange('KeyPrefix', e.target.value)}
              className="h-8 text-sm font-mono"
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Fail Open</Label>
            <select
              value={editedConfig.rate_limit?.Redis?.FailOpen ? 'true' : 'false'}
              onChange={(e) => handleRateLimitRedisChange('FailOpen', e.target.value === 'true')}
              className="flex h-8 w-full rounded-md border border-input bg-background px-3 py-1 text-sm"
            >
              <option value="false">No</option>
              <option value="true">Yes</option>
            </select>
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Dial Timeout (ms)</Label>
            <Input
              type="number"
              value={editedConfig.rate_limit?.Redis?.DialTimeoutMs ?? 0}
              onChange={(e) => handleRateLimitRedisChange('DialTimeoutMs', parseInt(e.target.value) || 0)}
              className="h-8 text-sm"
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Op Timeout (ms)</Label>
            <Input
              type="number"
              value={editedConfig.rate_limit?.Redis?.OpTimeoutMs ?? 0}
              onChange={(e) => handleRateLimitRedisChange('OpTimeoutMs', parseInt(e.target.value) || 0)}
              className="h-8 text-sm"
            />
          </div>
        </div>
      </div>

      {/* Circuit Breaker */}
      <div className="p-4 rounded-lg border bg-card space-y-4">
        <h3 className="font-semibold">Circuit Breaker Defaults</h3>
        <div className="space-y-4">
          <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
            <div className="space-y-1">
              <Label className="text-xs">Backend</Label>
              <Input
                value={editedConfig.circuit_breaker?.Backend || ''}
                onChange={(e) => handleCircuitBreakerChange('Backend', e.target.value)}
                className="h-8 text-sm"
              />
            </div>
          </div>

          <div>
            <h4 className="text-sm font-medium mb-2">Redis Configuration</h4>
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
              <div className="space-y-1">
                <Label className="text-xs">Redis Address</Label>
                <Input
                  value={editedConfig.circuit_breaker?.Redis?.Addr || ''}
                  onChange={(e) => handleCircuitBreakerRedisChange('Addr', e.target.value)}
                  className="h-8 text-sm font-mono"
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Redis DB</Label>
                <Input
                  type="number"
                  value={editedConfig.circuit_breaker?.Redis?.DB ?? 0}
                  onChange={(e) => handleCircuitBreakerRedisChange('DB', parseInt(e.target.value) || 0)}
                  className="h-8 text-sm"
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Key Prefix</Label>
                <Input
                  value={editedConfig.circuit_breaker?.Redis?.KeyPrefix || ''}
                  onChange={(e) => handleCircuitBreakerRedisChange('KeyPrefix', e.target.value)}
                  className="h-8 text-sm font-mono"
                />
              </div>
            </div>
          </div>

          <div>
            <h4 className="text-sm font-medium mb-2">Defaults</h4>
            <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
              <div className="space-y-1">
                <Label className="text-xs">Enabled</Label>
                <select
                  value={editedConfig.circuit_breaker?.Defaults?.Enabled ? 'true' : 'false'}
                  onChange={(e) => handleCircuitBreakerDefaultsChange('Enabled', e.target.value === 'true')}
                  className="flex h-8 w-full rounded-md border border-input bg-background px-3 py-1 text-sm"
                >
                  <option value="false">No</option>
                  <option value="true">Yes</option>
                </select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Window Seconds</Label>
                <Input
                  type="number"
                  value={editedConfig.circuit_breaker?.Defaults?.WindowSeconds ?? 0}
                  onChange={(e) => handleCircuitBreakerDefaultsChange('WindowSeconds', parseInt(e.target.value) || 0)}
                  className="h-8 text-sm"
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Min Requests</Label>
                <Input
                  type="number"
                  value={editedConfig.circuit_breaker?.Defaults?.MinRequests ?? 0}
                  onChange={(e) => handleCircuitBreakerDefaultsChange('MinRequests', parseInt(e.target.value) || 0)}
                  className="h-8 text-sm"
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Failure Rate Threshold</Label>
                <Input
                  type="number"
                  step="0.01"
                  value={editedConfig.circuit_breaker?.Defaults?.FailureRateThreshold ?? 0}
                  onChange={(e) => handleCircuitBreakerDefaultsChange('FailureRateThreshold', parseFloat(e.target.value) || 0)}
                  className="h-8 text-sm"
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Open Cooldown Seconds</Label>
                <Input
                  type="number"
                  value={editedConfig.circuit_breaker?.Defaults?.OpenCooldownSeconds ?? 0}
                  onChange={(e) => handleCircuitBreakerDefaultsChange('OpenCooldownSeconds', parseInt(e.target.value) || 0)}
                  className="h-8 text-sm"
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="flex gap-2 pt-4 border-t">
        <Button onClick={handleSave}>Save Changes</Button>
        <Button variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </div>
  )
}
