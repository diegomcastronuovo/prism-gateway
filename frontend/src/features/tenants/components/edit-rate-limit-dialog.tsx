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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  useUpdateTenantConfig,
  type TenantConfig,
  type TenantRateLimit,
} from '../api/use-tenants'

const SCOPE_VALUES = ['tenant', 'api_key', 'jwt_sub'] as const

const rateLimitSchema = z.object({
  rpm: z.coerce.number({ invalid_type_error: 'RPM must be a number' }).min(1, 'RPM must be at least 1'),
  burst: z.coerce.number({ invalid_type_error: 'Burst must be a number' }).min(1, 'Burst must be at least 1'),
  scope: z.enum(SCOPE_VALUES),
})

type RateLimitFormData = z.infer<typeof rateLimitSchema>

function parseRateLimit(raw: unknown): TenantRateLimit | null {
  if (!raw || typeof raw !== 'object') return null
  const o = raw as Record<string, unknown>
  const rpm = Number(o.rpm)
  const burst = Number(o.burst)
  const scope = o.scope
  if (!Number.isFinite(rpm) || !Number.isFinite(burst)) return null
  if (scope !== 'tenant' && scope !== 'api_key' && scope !== 'jwt_sub') return null
  return { rpm, burst, scope }
}

interface EditRateLimitDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantConfig: TenantConfig | null
}

export function EditRateLimitDialog({ open, onOpenChange, tenantConfig }: EditRateLimitDialogProps) {
  const updateConfig = useUpdateTenantConfig()
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
    setValue,
    watch,
  } = useForm<RateLimitFormData>({
    resolver: zodResolver(rateLimitSchema),
    defaultValues: {
      rpm: 60,
      burst: 5,
      scope: 'tenant',
    },
  })

  const scopeValue = watch('scope')

  useEffect(() => {
    if (!tenantConfig) return
    const rl = parseRateLimit(tenantConfig.config.rate_limit)
    if (rl) {
      reset({
        rpm: rl.rpm,
        burst: rl.burst,
        scope: rl.scope,
      })
    } else {
      reset({
        rpm: 60,
        burst: 5,
        scope: 'tenant',
      })
    }
  }, [tenantConfig, reset])

  const onSubmit = async (data: RateLimitFormData) => {
    if (!tenantConfig) return

    const patch: Record<string, unknown> = {
      rate_limit: {
        rpm: data.rpm,
        burst: data.burst,
        scope: data.scope,
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
      // Toast handled by mutation
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit Rate Limit</DialogTitle>
          <DialogDescription>
            Update per-tenant request rate limits for {tenantConfig?.tenant_id}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-4 py-2">
            <p className="text-sm text-muted-foreground">
              Controls how request-per-minute limits are applied for this tenant.
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="rate-rpm">Requests per Minute (RPM)</Label>
                <Input id="rate-rpm" type="number" min={1} step={1} placeholder="60" {...register('rpm')} />
                {errors.rpm && <p className="text-sm text-destructive">{errors.rpm.message}</p>}
              </div>
              <div className="space-y-2">
                <Label htmlFor="rate-burst">Burst</Label>
                <Input id="rate-burst" type="number" min={1} step={1} placeholder="5" {...register('burst')} />
                {errors.burst && <p className="text-sm text-destructive">{errors.burst.message}</p>}
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="rate-scope">Scope</Label>
              <Select
                value={scopeValue}
                onValueChange={(v) => setValue('scope', v as RateLimitFormData['scope'])}
              >
                <SelectTrigger id="rate-scope">
                  <SelectValue placeholder="Select scope" />
                </SelectTrigger>
                <SelectContent>
                  {SCOPE_VALUES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.scope && <p className="text-sm text-destructive">{errors.scope.message}</p>}
              <p className="text-xs text-muted-foreground leading-relaxed">
                tenant: one shared limit for the whole tenant
                <br />
                api_key: one limit per API key within the tenant
                <br />
                jwt_sub: one limit per JWT user within the tenant
              </p>
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
