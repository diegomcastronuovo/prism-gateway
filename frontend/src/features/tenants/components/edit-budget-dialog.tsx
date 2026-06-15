'use client'

import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import * as z from 'zod'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useUpdateTenantConfig, type TenantConfig } from '../api/use-tenants'

const timezones = [
  // North America
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Mexico_City',
  // LATAM
  'America/Bogota',
  'America/Lima',
  'America/Santiago',
  'America/Buenos_Aires',
  'America/Sao_Paulo',
  'America/Montevideo',
  'America/Asuncion',
  'America/La_Paz',
  'America/Guayaquil',
  'America/Caracas',
  'America/Guatemala',
  'America/Costa_Rica',
  'America/Panama',
  'America/El_Salvador',
  'America/Havana',
  'America/Santo_Domingo',
  'America/Puerto_Rico',
  // Europe
  'Europe/London',
  'Europe/Paris',
  // Asia
  'Asia/Tokyo',
  // UTC
  'UTC',
]

const enforcementModes = ['report_only', 'block', 'degrade'] as const

type EnforcementMode = (typeof enforcementModes)[number]

const budgetSchema = z
  .object({
    monthly_usd: z.coerce.number().min(0, 'Budget must be positive').optional(),
    timezone: z.string().optional(),
    enabled: z.boolean(),
    mode: z.enum(enforcementModes),
    warn_pct_ui: z.coerce.number().min(0, 'Warn % must be between 0 and 100').max(100, 'Warn % must be between 0 and 100'),
    hard_pct_ui: z.coerce.number().min(0, 'Hard % must be between 0 and 100').max(100, 'Hard % must be between 0 and 100'),
    block_status: z.coerce.number().int().min(100).max(599),
    degrade_route_group: z.string().optional(),
    include_cost_in_error: z.boolean(),
    tag_budgets_enabled: z.boolean(),
    tag_budget_keys: z.string().optional(),
    tag_budget_monthly_by_tag: z.string().optional(),
  })
  .superRefine((value, ctx) => {
    if (value.warn_pct_ui >= value.hard_pct_ui) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['warn_pct_ui'],
        message: 'Warn % must be lower than Hard %',
      })
    }
    if (value.mode === 'degrade' && !value.degrade_route_group) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['degrade_route_group'],
        message: 'Degrade route group is required for degrade mode',
      })
    }
  })

type BudgetFormData = z.infer<typeof budgetSchema>

interface EditBudgetDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantConfig: TenantConfig | null
}

function parseTagBudgetsMonthly(raw: string | undefined): Record<string, number> | undefined {
  if (!raw || !raw.trim()) return undefined
  try {
    const json = JSON.parse(raw) as Record<string, unknown>
    const parsed: Record<string, number> = {}
    for (const [k, v] of Object.entries(json)) {
      const n = Number(v)
      if (Number.isFinite(n)) parsed[k] = n
    }
    return parsed
  } catch {
    return undefined
  }
}

export function EditBudgetDialog({ open, onOpenChange, tenantConfig }: EditBudgetDialogProps) {
  const updateConfig = useUpdateTenantConfig()
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
    setValue,
    watch,
  } = useForm<BudgetFormData>({
    resolver: zodResolver(budgetSchema),
    defaultValues: {
      timezone: 'UTC',
      enabled: false,
      mode: 'report_only',
      warn_pct_ui: 80,
      hard_pct_ui: 100,
      block_status: 429,
      include_cost_in_error: false,
      tag_budgets_enabled: false,
      degrade_route_group: '',
      tag_budget_keys: '',
      tag_budget_monthly_by_tag: '',
    },
  })

  const selectedTimezone = watch('timezone')
  const selectedMode = watch('mode')
  const tagBudgetsEnabled = watch('tag_budgets_enabled')

  const config = tenantConfig?.config as TenantConfig['config'] | undefined
  const routeGroups: string[] =
    config?.selection?.route_groups && Object.keys(config.selection.route_groups).length > 0
      ? Object.keys(config.selection.route_groups)
      : config?.route_groups ?? []

  useEffect(() => {
    if (!tenantConfig) return
    const be = config?.budget_enforcement
    const tagBudgets = be?.tag_budgets
    reset({
      monthly_usd: config?.budgets?.monthly_usd,
      timezone: config?.budgets?.timezone || 'UTC',
      enabled: be?.enabled === true,
      mode: (be?.mode as EnforcementMode | undefined) ?? 'report_only',
      warn_pct_ui: (be?.thresholds?.warn_pct ?? 0.8) * 100,
      hard_pct_ui: (be?.thresholds?.hard_pct ?? 1.0) * 100,
      block_status: be?.block_status ?? 429,
      degrade_route_group: be?.degrade_route_group ?? '',
      include_cost_in_error: be?.include_cost_in_error === true,
      tag_budgets_enabled: tagBudgets?.enabled === true,
      tag_budget_keys: (tagBudgets?.keys ?? []).join(','),
      tag_budget_monthly_by_tag: JSON.stringify(tagBudgets?.monthly_usd_by_tag ?? {}, null, 2),
    })
  }, [tenantConfig, reset])

  const onSubmit = async (data: BudgetFormData) => {
    if (!tenantConfig) return

    const tagKeys = (data.tag_budget_keys ?? '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    const monthlyByTag = parseTagBudgetsMonthly(data.tag_budget_monthly_by_tag)

    const patch: Record<string, unknown> = {
      budgets: {
        monthly_usd: data.monthly_usd,
        timezone: data.timezone,
      },
      budget_enforcement: {
        enabled: data.enabled,
        mode: data.mode,
        thresholds: {
          warn_pct: data.warn_pct_ui / 100,
          hard_pct: data.hard_pct_ui / 100,
        },
        include_cost_in_error: data.include_cost_in_error,
        ...(data.mode === 'block' ? { block_status: data.block_status } : {}),
        ...(data.mode === 'degrade' ? { degrade_route_group: data.degrade_route_group } : {}),
        tag_budgets: {
          enabled: data.tag_budgets_enabled,
          keys: tagKeys,
          ...(monthlyByTag ? { monthly_usd_by_tag: monthlyByTag } : {}),
        },
      },
    }

    try {
      await updateConfig.mutateAsync({
        tenantId: tenantConfig.tenant_id,
        version: tenantConfig.version,
        patch,
      })
      onOpenChange(false)
    } catch {
      // Error is handled by the mutation
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Edit Budget</DialogTitle>
          <DialogDescription>
            Update budget and budget enforcement settings for {tenantConfig?.tenant_id}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-5 py-4">
            <div className="space-y-2">
              <Label htmlFor="monthly_usd">Monthly Budget (USD)</Label>
              <Input id="monthly_usd" type="number" step="0.01" placeholder="500" {...register('monthly_usd')} />
              {errors.monthly_usd && <p className="text-sm text-destructive">{errors.monthly_usd.message}</p>}
            </div>

            <div className="space-y-2">
              <Label htmlFor="timezone">Timezone</Label>
              <Select value={selectedTimezone} onValueChange={(value: string) => setValue('timezone', value)}>
                <SelectTrigger>
                  <SelectValue placeholder="Select timezone" />
                </SelectTrigger>
                <SelectContent>
                  {timezones.map((tz) => (
                    <SelectItem key={tz} value={tz}>
                      {tz}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="border-t pt-4 space-y-4">
              <h4 className="font-medium">Budget Enforcement</h4>

              <div className="flex items-center justify-between rounded-lg border p-3">
                <Label htmlFor="be-enabled" className="cursor-pointer">Enabled</Label>
                <Switch id="be-enabled" checked={watch('enabled')} onCheckedChange={(v) => setValue('enabled', v)} />
              </div>

              <div className="space-y-2">
                <Label htmlFor="be-mode">Mode</Label>
                <Select value={selectedMode} onValueChange={(v) => setValue('mode', v as EnforcementMode)}>
                  <SelectTrigger id="be-mode">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="report_only">report_only</SelectItem>
                    <SelectItem value="block">block</SelectItem>
                    <SelectItem value="degrade">degrade</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="warn_pct_ui">Warn Threshold (%)</Label>
                  <Input id="warn_pct_ui" type="number" min={0} max={100} step="0.1" {...register('warn_pct_ui')} />
                  {errors.warn_pct_ui && <p className="text-sm text-destructive">{errors.warn_pct_ui.message}</p>}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="hard_pct_ui">Hard Threshold (%)</Label>
                  <Input id="hard_pct_ui" type="number" min={0} max={100} step="0.1" {...register('hard_pct_ui')} />
                  {errors.hard_pct_ui && <p className="text-sm text-destructive">{errors.hard_pct_ui.message}</p>}
                </div>
              </div>

              {selectedMode === 'block' && (
                <div className="space-y-2">
                  <Label htmlFor="block_status">Block Status Code</Label>
                  <Input id="block_status" type="number" min={100} max={599} step={1} {...register('block_status')} />
                  {errors.block_status && <p className="text-sm text-destructive">{errors.block_status.message}</p>}
                </div>
              )}

              {selectedMode === 'degrade' && (
                <div className="space-y-2">
                  <Label htmlFor="degrade_route_group">Degrade Route Group</Label>
                  <Select
                    value={watch('degrade_route_group') || '__empty__'}
                    onValueChange={(value) => setValue('degrade_route_group', value === '__empty__' ? '' : value)}
                  >
                    <SelectTrigger id="degrade_route_group">
                      <SelectValue placeholder="Select route group" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__empty__">Select route group</SelectItem>
                      {routeGroups.map((group: string) => (
                        <SelectItem key={group} value={group}>
                          {group}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {errors.degrade_route_group && <p className="text-sm text-destructive">{errors.degrade_route_group.message}</p>}
                </div>
              )}

              <div className="flex items-center justify-between rounded-lg border p-3">
                <Label htmlFor="include_cost_in_error" className="cursor-pointer">Include Cost in Error</Label>
                <Switch
                  id="include_cost_in_error"
                  checked={watch('include_cost_in_error')}
                  onCheckedChange={(v) => setValue('include_cost_in_error', v)}
                />
              </div>

              <div className="space-y-3 rounded-lg border p-3">
                <div className="flex items-center justify-between">
                  <Label htmlFor="tag_budgets_enabled" className="cursor-pointer">Tag Budgets (optional)</Label>
                  <Switch
                    id="tag_budgets_enabled"
                    checked={tagBudgetsEnabled}
                    onCheckedChange={(v) => setValue('tag_budgets_enabled', v)}
                  />
                </div>
                {tagBudgetsEnabled && (
                  <>
                    <div className="space-y-2">
                      <Label htmlFor="tag_budget_keys">Tag Keys (comma-separated)</Label>
                      <Input id="tag_budget_keys" placeholder="team,project" {...register('tag_budget_keys')} />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="tag_budget_monthly_by_tag">Monthly USD by Tag (JSON)</Label>
                      <textarea
                        id="tag_budget_monthly_by_tag"
                        className="min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                        placeholder='{"team:ai": 100, "project:x": 50}'
                        {...register('tag_budget_monthly_by_tag')}
                      />
                    </div>
                  </>
                )}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={updateConfig.isPending}>
              {updateConfig.isPending ? 'Saving...' : 'Save Changes'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
