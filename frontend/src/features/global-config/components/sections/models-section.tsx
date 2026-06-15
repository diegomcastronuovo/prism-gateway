'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Edit } from 'lucide-react'
import { readPricingFields } from '@/features/global-config/lib/model-pricing'
import { useGlobalConfig } from '@/features/global-config/api/use-global-config'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ModelsEditor } from '../editors/models-editor'

interface ModelMock {
  Enabled: boolean
  DelayMinMs: number
  DelayMaxMs: number
  ErrorRate: number
  ErrorStatus: number
  ErrorMessage: string
  FixedResponse: string
}

interface Model {
  Name: string
  Provider: string
  Type: string
  Pricing: { prompt_per_1m: number; completion_per_1m: number }
  Mock: ModelMock
}

interface ModelsSectionProps {
  config: Record<string, unknown>
  onUpdate?: (updatedModels: Model[]) => void
}

function normalizeMockForEditor(raw: unknown): ModelMock {
  const m = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {}
  return {
    Enabled: Boolean(m.Enabled ?? m.enabled),
    DelayMinMs: Number(m.DelayMinMs ?? m.delay_min_ms ?? 0),
    DelayMaxMs: Number(m.DelayMaxMs ?? m.delay_max_ms ?? 0),
    ErrorRate: Number(m.ErrorRate ?? m.error_rate ?? 0),
    ErrorStatus: Number(m.ErrorStatus ?? m.error_status ?? 500),
    ErrorMessage: String(m.ErrorMessage ?? m.error_message ?? ''),
    FixedResponse: String(m.FixedResponse ?? m.fixed_response ?? ''),
  }
}

/** Map stored global config rows (mixed key styles) into the shape ModelsEditor expects */
function normalizeModelRowForEditor(raw: Record<string, unknown>): Model {
  const pricing = readPricingFields(raw.Pricing ?? raw.pricing)
  return {
    Name: String(raw.Name ?? raw.name ?? ''),
    Provider: String(raw.Provider ?? raw.provider ?? ''),
    Type: String(raw.Type ?? raw.type ?? '') || 'llm',
    Pricing: {
      prompt_per_1m: pricing.prompt_per_1m,
      completion_per_1m: pricing.completion_per_1m,
    },
    Mock: normalizeMockForEditor(raw.Mock ?? raw.mock),
  }
}

export function ModelsSection({ config, onUpdate }: ModelsSectionProps) {
  /** Always prefer latest React Query cache */
  const { data: globalQuery } = useGlobalConfig()
  const models = (
    (globalQuery?.config?.models ?? config.models) as Array<Record<string, unknown>> | undefined
  )

  const modelsForEditor = useMemo(
    () => (models ?? []).map((row) => normalizeModelRowForEditor(row)),
    [models]
  )

  // Keep onUpdate prop signature intact for compatibility even though editing is now in /models
  void onUpdate
  void modelsForEditor

  if (!models || models.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>No models configured</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button size="sm" asChild>
          <Link href="/models">
            <Edit className="mr-2 h-4 w-4" />
            Edit Models
          </Link>
        </Button>
      </div>
      <div className="border rounded-md">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Model Name</TableHead>
            <TableHead>Provider</TableHead>
            <TableHead>Type</TableHead>
            <TableHead className="text-right">Prompt / 1M</TableHead>
            <TableHead className="text-right">Completion / 1M</TableHead>
            <TableHead>Mock</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {models.map((model, idx) => {
            const pricing = readPricingFields(model.Pricing ?? model.pricing)
            const mock = model.Mock as Record<string, unknown> | undefined
            const mockEnabled = mock?.Enabled ?? (model as { mock?: { enabled?: boolean } }).mock?.enabled

            const modelName = String(model.Name ?? model.name ?? '—')
            const modelProvider = String(model.Provider ?? model.provider ?? '—')
            const modelType = String(model.Type ?? model.type ?? '') || 'llm'

            return (
              <TableRow key={idx}>
                <TableCell className="font-medium">{modelName}</TableCell>
                <TableCell>{modelProvider}</TableCell>
                <TableCell>
                  <Badge variant="outline">{modelType}</Badge>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  ${Number(pricing.prompt_per_1m).toFixed(2)}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  ${Number(pricing.completion_per_1m).toFixed(2)}
                </TableCell>
                <TableCell>
                  {mockEnabled ? (
                    <div className="flex flex-col gap-1">
                      <Badge variant="secondary" className="w-fit">Enabled</Badge>
                      {(mock?.DelayMinMs !== undefined ||
                        mock?.delay_min_ms !== undefined ||
                        mock?.DelayMaxMs !== undefined ||
                        mock?.delay_max_ms !== undefined) && (
                        <span className="text-xs text-muted-foreground">
                          Delay: {Number(mock?.DelayMinMs ?? mock?.delay_min_ms ?? 0)}-
                          {Number(mock?.DelayMaxMs ?? mock?.delay_max_ms ?? 0)} ms
                        </span>
                      )}
                      {(mock?.ErrorRate !== undefined || mock?.error_rate !== undefined) && (
                        <span className="text-xs text-muted-foreground">
                          Error: {Number(mock?.ErrorRate ?? mock?.error_rate) * 100}%
                        </span>
                      )}
                    </div>
                  ) : (
                    '—'
                  )}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
      </div>
    </div>
  )
}
