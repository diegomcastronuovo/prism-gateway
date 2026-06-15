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
import { useUpdateTenantConfig, type TenantConfig } from '../api/use-tenants'

const outputLimitSchema = z.object({
  max_output_tokens: z.coerce
    .number({ invalid_type_error: 'Please enter 0 or a positive integer.' })
    .int('Please enter 0 or a positive integer.')
    .min(0, 'Please enter 0 or a positive integer.'),
})

type OutputLimitFormData = z.infer<typeof outputLimitSchema>

interface EditOutputLimitDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantConfig: TenantConfig | null
}

export function EditOutputLimitDialog({ open, onOpenChange, tenantConfig }: EditOutputLimitDialogProps) {
  const updateConfig = useUpdateTenantConfig()
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
  } = useForm<OutputLimitFormData>({
    resolver: zodResolver(outputLimitSchema),
    defaultValues: { max_output_tokens: 0 },
  })

  useEffect(() => {
    if (!tenantConfig) return
    const raw = (tenantConfig.config as Record<string, unknown>).max_output_tokens
    reset({ max_output_tokens: raw != null ? Number(raw) : 0 })
  }, [tenantConfig, reset])

  const onSubmit = async (data: OutputLimitFormData) => {
    if (!tenantConfig) return
    try {
      await updateConfig.mutateAsync({
        tenantId: tenantConfig.tenant_id,
        version: tenantConfig.version,
        patch: { max_output_tokens: data.max_output_tokens },
      })
      onOpenChange(false)
    } catch {
      // Toast handled by mutation
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Edit Output Limit</DialogTitle>
          <DialogDescription>
            Set the maximum LLM output tokens for {tenantConfig?.tenant_id}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="max-output-tokens">LLM maximum output tokens:</Label>
              <Input
                id="max-output-tokens"
                type="number"
                min={0}
                step={1}
                placeholder="0"
                {...register('max_output_tokens')}
              />
              {errors.max_output_tokens ? (
                <p className="text-sm text-destructive">{errors.max_output_tokens.message}</p>
              ) : (
                <p className="text-xs text-muted-foreground">
                  0 means unlimited. Applies only to LLM requests for this tenant.
                </p>
              )}
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
