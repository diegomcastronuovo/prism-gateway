'use client'

import { useState } from 'react'
import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Edit, Trash2, ChevronDown, ChevronUp, Layers, CheckCircle2, AlertCircle, XCircle, DollarSign, Zap, BarChart3, RefreshCw, Bug } from 'lucide-react'
import type { Model, ModelBenchmark } from '../api/use-models'

interface ModelDetailPanelProps {
  model: Model | undefined
  benchmark: ModelBenchmark | undefined
  isLoading: boolean
  onEdit: () => void
  onDelete: () => void
  windowHours: number
  onWindowHoursChange: (hours: number) => void
  onRefreshBenchmarks: () => void
}

function getStatusDisplay(benchmark: ModelBenchmark | undefined) {
  if (!benchmark || benchmark.samples === 0) {
    return { label: 'No data', variant: 'secondary' as const, icon: <XCircle className="h-4 w-4" /> }
  }
  if (benchmark.success_rate === 0) {
    return { label: 'Failing', variant: 'destructive' as const, icon: <AlertCircle className="h-4 w-4" /> }
  }
  if (benchmark.success_rate < 0.95) {
    return { label: 'Degraded', variant: 'outline' as const, icon: <AlertCircle className="h-4 w-4" /> }
  }
  return { label: 'Healthy', variant: 'default' as const, icon: <CheckCircle2 className="h-4 w-4" /> }
}

function formatLatency(ms: number): string {
  if (ms < 1000) return `${ms.toFixed(0)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function formatCost(usd: number): string {
  if (usd < 0.001) return `<$0.001`
  return `$${usd.toFixed(4)}`
}

function normalizeType(type: string | undefined): string {
  if (type === 'embedding') return 'Embedding'
  if (type === 'ml') return 'ML'
  return 'LLM'
}

function formatInfraCost(value: number | undefined): string {
  const amount = Number(value ?? 0)
  if (!Number.isFinite(amount)) return '$0'
  if (Number.isInteger(amount)) return `$${amount}`
  return `$${amount.toFixed(2)}`
}

/** Display model markup; missing or invalid → 0%. */
function formatMarkupPercent(model: Model): string {
  const raw = model.markup_percentage
  const v = typeof raw === 'number' && Number.isFinite(raw) ? raw : 0
  return `${v}%`
}

export function ModelDetailPanel({
  model,
  benchmark,
  isLoading,
  onEdit,
  onDelete,
  windowHours,
  onWindowHoursChange,
  onRefreshBenchmarks,
}: ModelDetailPanelProps) {
  const [showRawJson, setShowRawJson] = useState(false)
  const [inputHours, setInputHours] = useState(windowHours.toString())

  if (isLoading) {
    return (
      <SectionCard title="Model Details">
        <div className="space-y-4">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      </SectionCard>
    )
  }

  if (!model) {
    return (
      <SectionCard title="Model Details">
        <div className="text-center py-12 text-muted-foreground">
          <Layers className="h-12 w-12 mx-auto mb-4 opacity-50" />
          <p>Select a model to view details</p>
        </div>
      </SectionCard>
    )
  }

  const status = getStatusDisplay(benchmark)
  const isFailing = benchmark?.success_rate === 0

  return (
    <SectionCard title={`${model.id} Details`}>
      <div className="space-y-6">
        {/* Summary Section */}
        <div className="space-y-3">
          {/* Model name + status badge */}
          <div>
            <h3 className="text-base font-semibold break-all leading-snug">{model.id}</h3>
            <div className="flex items-center gap-2 mt-1.5 flex-wrap">
              <Badge variant={status.variant} className="flex items-center gap-1 shrink-0">
                {status.icon}
                {status.label}
              </Badge>
              {(() => {
                const raw = (model as Record<string, unknown>)['Enabled'] ?? (model as Record<string, unknown>)['enabled']
                if (raw === undefined || raw === null) return null
                const isEnabled = Boolean(raw)
                return (
                  <Badge variant={isEnabled ? 'default' : 'secondary'} className="shrink-0">
                    {isEnabled ? 'Enabled' : 'Disabled'}
                  </Badge>
                )
              })()}
              {model.mock?.enabled && (
                <Badge variant="outline" className="flex items-center gap-1 shrink-0">
                  <Bug className="h-3 w-3" />
                  Mock
                </Badge>
              )}
            </div>
            <p className="text-xs text-muted-foreground mt-1.5">
              Provider: <span className="font-medium text-foreground">{model.provider}</span>
              <span className="mx-1.5">·</span>
              Type: <span className="font-medium text-foreground">{normalizeType(model.type)}</span>
            </p>
          </div>

          {/* Action buttons — equal width, same size */}
          <div className="grid grid-cols-2 gap-2">
            <Button variant="outline" size="sm" onClick={onEdit} className="w-full">
              <span className="inline-flex items-center gap-1.5">
                <Edit className="h-3.5 w-3.5" />
                <span>Edit</span>
              </span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={onDelete}
              className="w-full border-destructive/40 text-destructive hover:bg-destructive hover:text-destructive-foreground hover:border-destructive"
            >
              <span className="inline-flex items-center gap-1.5">
                <Trash2 className="h-3.5 w-3.5" />
                <span>Delete</span>
              </span>
            </Button>
          </div>
        </div>

        <div className="border-t" />

        {/* Routing Metadata */}
        <div className="space-y-3">
          <h4 className="font-medium flex items-center gap-2">
            <Zap className="h-4 w-4" />
            Routing
          </h4>
          <div className="grid gap-2 text-sm">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Provider</span>
              <span className="font-medium">{model.provider}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Route Groups</span>
              <div className="flex flex-wrap gap-1 justify-end">
                {model.route_groups && model.route_groups.length > 0 ? (
                  model.route_groups.map((group) => (
                    <Badge key={group} variant="secondary" className="text-xs">
                      {group}
                    </Badge>
                  ))
                ) : (
                  <span className="text-muted-foreground">None</span>
                )}
              </div>
            </div>
            {model.type === 'ml' && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">Execution endpoint</span>
                <span className="font-medium">
                  {model.execution?.endpoint || '—'}
                </span>
              </div>
            )}
            {model.base_url && model.type !== 'ml' && (
              <div className="flex justify-between">
                <span className="text-muted-foreground">Base URL (override)</span>
                <span className="font-medium font-mono text-xs">{model.base_url}</span>
              </div>
            )}
          </div>
        </div>

        {/* Mock Configuration */}
        {model.mock?.enabled && (
          <>
            <div className="border-t" />
            <div className="space-y-3">
              <h4 className="font-medium flex items-center gap-2">
                <Bug className="h-4 w-4" />
                Mock Configuration
              </h4>
              <div className="grid gap-2 text-sm">
                {model.mock.delay_min_ms !== undefined && model.mock.delay_max_ms !== undefined && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Delay</span>
                    <span>{model.mock.delay_min_ms} - {model.mock.delay_max_ms} ms</span>
                  </div>
                )}
                {model.mock.error_rate !== undefined && model.mock.error_rate > 0 && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Error Rate</span>
                    <span>{(model.mock.error_rate * 100).toFixed(0)}%</span>
                  </div>
                )}
                {model.mock.error_status !== undefined && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Error Status</span>
                    <span>{model.mock.error_status}</span>
                  </div>
                )}
                {model.mock.error_message && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Error Message</span>
                    <span className="truncate max-w-[200px]">{model.mock.error_message}</span>
                  </div>
                )}
                {model.mock.fixed_response && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Fixed Response</span>
                    <span className="truncate max-w-[200px]">{model.mock.fixed_response.substring(0, 20)}...</span>
                  </div>
                )}
              </div>
            </div>
          </>
        )}

        {model.type === 'ml' && (
          <>
            <div className="border-t" />
            <div className="space-y-3">
              <h4 className="font-medium flex items-center gap-2">
                <Zap className="h-4 w-4" />
                Observable Fields
              </h4>
              {Array.isArray(model.observable?.fields) && model.observable.fields.length > 0 ? (
                <div className="space-y-2">
                  {model.observable.fields.map((field, idx) => (
                    <div key={`${field.path}-${field.type}-${field.role}-${idx}`} className="grid grid-cols-3 gap-2 text-sm">
                      <span className="font-medium">{field.path}</span>
                      <span className="text-muted-foreground">{field.type}</span>
                      <span className="text-muted-foreground">{field.role}</span>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">No observable fields configured</p>
              )}
            </div>
          </>
        )}

        {/* Pricing */}
        {model.pricing && (model.pricing.prompt_per_1m !== undefined || model.pricing.completion_per_1m !== undefined) && (
          <>
            <div className="border-t" />
            <div className="space-y-3">
              <h4 className="font-medium flex items-center gap-2">
                <DollarSign className="h-4 w-4" />
                Pricing (per 1M tokens)
              </h4>
              <div className="grid gap-2 text-sm">
                {model.pricing.prompt_per_1m !== undefined && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Prompt</span>
                    <span>${model.pricing.prompt_per_1m.toFixed(2)}</span>
                  </div>
                )}
                {model.pricing.completion_per_1m !== undefined && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Completion</span>
                    <span>${model.pricing.completion_per_1m.toFixed(2)}</span>
                  </div>
                )}
              </div>
            </div>
          </>
        )}

        <>
          <div className="border-t" />
          <div className="space-y-3">
            <h4 className="font-medium flex items-center gap-2">
              <DollarSign className="h-4 w-4" />
              Model Markup %
            </h4>
            <div className="grid gap-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Model Markup %</span>
                <span className="font-medium">{formatMarkupPercent(model)}</span>
              </div>
              <p className="text-xs text-muted-foreground">
                Percentage added on top of model cost to calculate final price.
              </p>
            </div>
          </div>
        </>

        <>
          <div className="border-t" />
          <div className="space-y-3">
            <h4 className="font-medium flex items-center gap-2">
              <DollarSign className="h-4 w-4" />
              Infrastructure monthly cost (USD)
            </h4>
            <div className="grid gap-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Monthly cost</span>
                <span>{formatInfraCost(model.infrastructure_monthly_usd)}</span>
              </div>
            </div>
          </div>
        </>

        {/* Performance Section — only rendered when benchmark data exists */}
        {benchmark && benchmark.samples > 0 && (
          <>
            <div className="border-t" />
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <h4 className="font-medium flex items-center gap-2">
                  <BarChart3 className="h-4 w-4" />
                  Performance
                </h4>
                <div className="flex items-center gap-2">
                  <div className="flex items-center gap-1">
                    <Input
                      type="number"
                      min={1}
                      max={720}
                      value={inputHours}
                      onChange={(e) => setInputHours(e.target.value)}
                      onBlur={() => {
                        const hours = parseInt(inputHours, 10)
                        if (!isNaN(hours) && hours > 0) {
                          onWindowHoursChange(hours)
                        } else {
                          setInputHours(windowHours.toString())
                        }
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          const hours = parseInt(inputHours, 10)
                          if (!isNaN(hours) && hours > 0) {
                            onWindowHoursChange(hours)
                          }
                        }
                      }}
                      className="w-16 h-8 text-sm px-2"
                    />
                    <span className="text-sm text-muted-foreground">h</span>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={onRefreshBenchmarks}
                    className="h-8 px-2"
                  >
                    <RefreshCw className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className={`rounded-lg border p-3 ${isFailing ? 'bg-destructive/10 border-destructive/30' : ''}`}>
                  <p className="text-xs text-muted-foreground">Success Rate</p>
                  <p className={`text-lg font-semibold ${isFailing ? 'text-destructive' : ''}`}>
                    {(benchmark.success_rate * 100).toFixed(1)}%
                  </p>
                  <p className="text-xs text-muted-foreground">{benchmark.samples} samples</p>
                </div>
                <div className={`rounded-lg border p-3 ${isFailing ? 'bg-muted' : ''}`}>
                  <p className="text-xs text-muted-foreground">Avg Latency</p>
                  <p className={`text-lg font-semibold ${isFailing ? 'text-muted-foreground' : ''}`}>
                    {isFailing ? 'N/A' : formatLatency(benchmark.avg_latency_ms)}
                  </p>
                  {!isFailing && benchmark.p95_latency_ms > 0 && (
                    <p className="text-xs text-muted-foreground">
                      P95: {formatLatency(benchmark.p95_latency_ms)}
                    </p>
                  )}
                </div>
                <div className="rounded-lg border p-3">
                  <p className="text-xs text-muted-foreground">Avg Cost</p>
                  <p className="text-lg font-semibold">{formatCost(benchmark.avg_cost_usd)}</p>
                </div>
                <div className="rounded-lg border p-3">
                  <p className="text-xs text-muted-foreground">Provider</p>
                  <p className="text-lg font-semibold">{benchmark.provider}</p>
                </div>
              </div>
            </div>
          </>
        )}

        {/* Raw JSON */}
        <div className="border-t" />
        <div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setShowRawJson(!showRawJson)}
            className="mb-2"
          >
            {showRawJson ? (
              <>
                <ChevronUp className="h-4 w-4 mr-2" />
                Hide Raw JSON
              </>
            ) : (
              <>
                <ChevronDown className="h-4 w-4 mr-2" />
                Show Raw JSON
              </>
            )}
          </Button>
          {showRawJson && (
            <pre className="bg-muted p-4 rounded-lg overflow-auto text-xs max-h-96">
              {JSON.stringify(model, null, 2)}
            </pre>
          )}
        </div>
      </div>
    </SectionCard>
  )
}
