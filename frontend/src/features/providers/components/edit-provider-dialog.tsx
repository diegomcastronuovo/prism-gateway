'use client'

import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
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
import type { ProviderWithVersion } from '../api/use-providers'
import { useUpdateProvider } from '../api/use-providers'
import { getVersionForProviderMutation } from '../api/provider-mutation-version'
import { isAwsBedrockProvider, providerDisplayTitle } from '../lib/provider-display'

const providerSchema = z.object({
  base_url: z.string().url().optional().or(z.literal('')),
  api_version: z.string().optional(),
  organization: z.string().optional(),
  project: z.string().optional(),
  timeout_ms: z.number().min(1000).max(120000).optional().or(z.nan()),
  max_retries: z.number().min(0).max(10).optional().or(z.nan()),
})

const bedrockSchema = z.object({
  aws_access_key_id: z.string().min(1, 'AWS Access Key ID is required'),
  aws_secret_access_key: z.string().min(1, 'AWS Secret Access Key is required'),
  aws_region: z.string().min(1, 'AWS Region is required'),
})

type ProviderFormData = z.infer<typeof providerSchema>
type BedrockFormData = z.infer<typeof bedrockSchema>

interface EditProviderDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  provider: ProviderWithVersion | null
}

export function EditProviderDialog({ open, onOpenChange, provider }: EditProviderDialogProps) {
  const queryClient = useQueryClient()
  const updateProvider = useUpdateProvider()
  const isBedrock = provider ? isAwsBedrockProvider(provider) : false

  const defaultForm = useForm<ProviderFormData>({
    resolver: zodResolver(providerSchema),
    defaultValues: {
      base_url: '',
      api_version: '',
      organization: '',
      project: '',
      timeout_ms: undefined,
      max_retries: undefined,
    },
  })

  const bedrockForm = useForm<BedrockFormData>({
    resolver: zodResolver(bedrockSchema),
    defaultValues: {
      aws_access_key_id: '',
      aws_secret_access_key: '',
      aws_region: '',
    },
  })

  // Load current values
  useEffect(() => {
    if (open && provider) {
      if (isAwsBedrockProvider(provider)) {
        bedrockForm.reset({
          aws_access_key_id: provider.aws_access_key_id || '',
          aws_secret_access_key: '',
          aws_region: provider.aws_region || '',
        })
      } else {
        defaultForm.reset({
          base_url: provider.base_url || '',
          api_version: provider.api_version || '',
          organization: provider.organization || '',
          project: provider.project || '',
          timeout_ms: provider.timeout_ms,
          max_retries: provider.max_retries,
        })
      }
    }
  }, [open, provider, defaultForm, bedrockForm])

  const onSubmitDefault = async (data: ProviderFormData) => {
    if (!provider) return

    const cleanData: Partial<ProviderFormData> = {}
    if (data.base_url) cleanData.base_url = data.base_url
    if (data.api_version) cleanData.api_version = data.api_version
    if (data.organization) cleanData.organization = data.organization
    if (data.project) cleanData.project = data.project
    if (data.timeout_ms && !isNaN(data.timeout_ms)) cleanData.timeout_ms = data.timeout_ms
    if (data.max_retries !== undefined && !isNaN(data.max_retries)) cleanData.max_retries = data.max_retries

    try {
      const version = getVersionForProviderMutation(queryClient, provider.version)
      await updateProvider.mutateAsync({
        providerId: provider.id,
        config: cleanData,
        version,
      })
      onOpenChange(false)
    } catch {
      // Error is handled by the mutation
    }
  }

  const onSubmitBedrock = async (data: BedrockFormData) => {
    if (!provider) return

    try {
      const version = getVersionForProviderMutation(queryClient, provider.version)
      await updateProvider.mutateAsync({
        providerId: provider.id,
        config: {
          type: 'aws_bedrock',
          aws_access_key_id: data.aws_access_key_id.trim(),
          aws_secret_access_key: data.aws_secret_access_key.trim(),
          aws_region: data.aws_region.trim(),
        },
        version,
      })
      onOpenChange(false)
    } catch {
      // Error is handled by the mutation
    }
  }

  const titleBase = provider ? providerDisplayTitle(provider.id) : 'Provider'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Edit {titleBase}</DialogTitle>
          <DialogDescription>
            {isBedrock ? 'Update AWS Bedrock connection and credentials' : 'Update provider connection settings'}
          </DialogDescription>
        </DialogHeader>
        {isBedrock ? (
          <form onSubmit={bedrockForm.handleSubmit(onSubmitBedrock)} className="space-y-4">
            <div className="space-y-2">
              <Label>Type</Label>
              <Input value="aws_bedrock" readOnly className="bg-muted" />
            </div>
            <div className="space-y-2">
              <Label htmlFor="bedrock_aws_access_key_id">AWS Access Key ID</Label>
              <Input
                id="bedrock_aws_access_key_id"
                autoComplete="off"
                {...bedrockForm.register('aws_access_key_id')}
              />
              {bedrockForm.formState.errors.aws_access_key_id && (
                <p className="text-sm text-destructive">{bedrockForm.formState.errors.aws_access_key_id.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="bedrock_aws_secret_access_key">AWS Secret Access Key</Label>
              <Input
                id="bedrock_aws_secret_access_key"
                type="password"
                autoComplete="off"
                {...bedrockForm.register('aws_secret_access_key')}
              />
              {bedrockForm.formState.errors.aws_secret_access_key && (
                <p className="text-sm text-destructive">{bedrockForm.formState.errors.aws_secret_access_key.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="bedrock_aws_region">AWS Region</Label>
              <Input
                id="bedrock_aws_region"
                placeholder="e.g. us-east-1"
                autoComplete="off"
                {...bedrockForm.register('aws_region')}
              />
              {bedrockForm.formState.errors.aws_region && (
                <p className="text-sm text-destructive">{bedrockForm.formState.errors.aws_region.message}</p>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={updateProvider.isPending}>
                {updateProvider.isPending ? 'Saving...' : 'Save Changes'}
              </Button>
            </DialogFooter>
          </form>
        ) : (
          <form onSubmit={defaultForm.handleSubmit(onSubmitDefault)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="base_url">Base URL</Label>
              <Input id="base_url" placeholder="https://api.example.com/v1" {...defaultForm.register('base_url')} />
              {defaultForm.formState.errors.base_url && (
                <p className="text-sm text-destructive">{defaultForm.formState.errors.base_url.message}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="api_version">API Version</Label>
              <Input id="api_version" placeholder="e.g., 2024-01-01" {...defaultForm.register('api_version')} />
            </div>

            <div className="space-y-2">
              <Label htmlFor="organization">Organization</Label>
              <Input id="organization" placeholder="Organization ID or name" {...defaultForm.register('organization')} />
            </div>

            <div className="space-y-2">
              <Label htmlFor="project">Project</Label>
              <Input id="project" placeholder="Project ID" {...defaultForm.register('project')} />
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="timeout_ms">Timeout (ms)</Label>
                <Input
                  id="timeout_ms"
                  type="number"
                  min={1000}
                  max={120000}
                  step={1000}
                  placeholder="30000"
                  {...defaultForm.register('timeout_ms', { valueAsNumber: true })}
                />
                {defaultForm.formState.errors.timeout_ms && (
                  <p className="text-sm text-destructive">{defaultForm.formState.errors.timeout_ms.message}</p>
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="max_retries">Max Retries</Label>
                <Input
                  id="max_retries"
                  type="number"
                  min={0}
                  max={10}
                  placeholder="3"
                  {...defaultForm.register('max_retries', { valueAsNumber: true })}
                />
                {defaultForm.formState.errors.max_retries && (
                  <p className="text-sm text-destructive">{defaultForm.formState.errors.max_retries.message}</p>
                )}
              </div>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={updateProvider.isPending}>
                {updateProvider.isPending ? 'Saving...' : 'Save Changes'}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
