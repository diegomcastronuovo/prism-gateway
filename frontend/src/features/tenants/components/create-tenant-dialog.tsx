'use client'

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
import { useCreateTenant } from '../api/use-tenants'
import type { TenantEnvironment } from '../api/use-tenants'

const tenantSchema = z.object({
  tenant_id: z
    .string()
    .min(1, 'Tenant ID is required')
    .regex(/^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/, 'Must start with a letter and contain only letters, numbers, hyphens, and underscores (max 64 characters)'),
  environment: z.enum(['DEV', 'STAGING', 'PROD']),
})

type TenantFormData = z.infer<typeof tenantSchema>

interface CreateTenantDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: (tenantId: string) => void
}

export function CreateTenantDialog({ open, onOpenChange, onSuccess }: CreateTenantDialogProps) {
  const createTenant = useCreateTenant()
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
    setValue,
    watch,
  } = useForm<TenantFormData>({
    resolver: zodResolver(tenantSchema),
    defaultValues: {
      environment: 'DEV',
    },
  })

  const selectedEnvironment = watch('environment')

  const onSubmit = async (data: TenantFormData) => {
    try {
      const result = await createTenant.mutateAsync({
        tenant_id: data.tenant_id,
        environment: data.environment as TenantEnvironment,
      })
      reset()
      onOpenChange(false)
      onSuccess?.(result.tenant_id)
    } catch {
      // Error is handled by the mutation
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create Tenant</DialogTitle>
          <DialogDescription>
            Create a new tenant to manage access and routing configuration.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="tenant_id">Tenant ID</Label>
              <Input
                id="tenant_id"
                placeholder="my-tenant"
                {...register('tenant_id')}
              />
              {errors.tenant_id && (
                <p className="text-sm text-destructive">{errors.tenant_id.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="tenant_environment">Environment</Label>
              <Select
                value={selectedEnvironment}
                onValueChange={(value) =>
                  setValue('environment', value as TenantEnvironment, { shouldValidate: true, shouldDirty: true })
                }
              >
                <SelectTrigger id="tenant_environment">
                  <SelectValue placeholder="Select environment" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="DEV">DEV</SelectItem>
                  <SelectItem value="STAGING">STAGING</SelectItem>
                  <SelectItem value="PROD">PROD</SelectItem>
                </SelectContent>
              </Select>
              {errors.environment && (
                <p className="text-sm text-destructive">{errors.environment.message}</p>
              )}
              <input type="hidden" {...register('environment')} />
            </div>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                reset()
                onOpenChange(false)
              }}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={createTenant.isPending}>
              {createTenant.isPending ? 'Creating...' : 'Create Tenant'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
