'use client'

import { useState } from 'react'
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
import { Checkbox } from '@/components/ui/checkbox'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { useCreateTenantApiKey } from '../api/use-tenants'

type ExpirationOption = 'never' | '30days' | '90days' | 'custom'

interface CreateApiKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tenantId: string
  onSuccess: (apiKey: string) => void
}

export function CreateApiKeyDialog({
  open,
  onOpenChange,
  tenantId,
  onSuccess,
}: CreateApiKeyDialogProps) {
  const [name, setName] = useState('')
  const [scopes, setScopes] = useState<string[]>(['inference'])
  const [expiration, setExpiration] = useState<ExpirationOption>('never')
  const [customDate, setCustomDate] = useState('')
  const createMutation = useCreateTenantApiKey()

  const getExpiresAt = (): string | null => {
    if (expiration === 'never') return null
    if (expiration === 'custom') return customDate ? new Date(customDate).toISOString() : null
    
    const now = new Date()
    if (expiration === '30days') {
      now.setDate(now.getDate() + 30)
    } else if (expiration === '90days') {
      now.setDate(now.getDate() + 90)
    }
    return now.toISOString()
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    
    if (!name.trim()) return

    createMutation.mutate(
      {
        tenantId,
        name: name.trim(),
        scopes,
        expires_at: getExpiresAt(),
      },
      {
        onSuccess: (data) => {
          console.log('Create API key response:', data)
          const response = data as { key?: string; api_key?: string }
          const apiKey = response.key || response.api_key || ''
          console.log('API key value:', apiKey)
          onSuccess(apiKey)
          onOpenChange(false)
          setName('')
          setScopes(['inference'])
          setExpiration('never')
          setCustomDate('')
        },
      }
    )
  }

  const toggleScope = (scope: string) => {
    setScopes((prev) =>
      prev.includes(scope)
        ? prev.filter((s) => s !== scope)
        : [...prev, scope]
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Create API Key</DialogTitle>
            <DialogDescription>
              Create a new API key for this tenant
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                placeholder="production-app"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>

            <div className="space-y-2">
              <Label>Scopes</Label>
              <div className="space-y-2">
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="inference"
                    checked={scopes.includes('inference')}
                    onCheckedChange={() => toggleScope('inference')}
                  />
                  <label
                    htmlFor="inference"
                    className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                  >
                    inference
                  </label>
                </div>
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="admin_read"
                    checked={scopes.includes('admin_read')}
                    onCheckedChange={() => toggleScope('admin_read')}
                  />
                  <label
                    htmlFor="admin_read"
                    className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                  >
                    admin_read
                  </label>
                </div>
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="admin_write"
                    checked={scopes.includes('admin_write')}
                    onCheckedChange={() => toggleScope('admin_write')}
                  />
                  <label
                    htmlFor="admin_write"
                    className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                  >
                    admin_write
                  </label>
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <Label>Expiration</Label>
              <RadioGroup
                value={expiration}
                onValueChange={(value: string) => setExpiration(value as ExpirationOption)}
                className="space-y-2"
              >
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="never" id="never" />
                  <Label htmlFor="never" className="text-sm font-normal cursor-pointer">
                    Never
                  </Label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="30days" id="30days" />
                  <Label htmlFor="30days" className="text-sm font-normal cursor-pointer">
                    30 days
                  </Label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="90days" id="90days" />
                  <Label htmlFor="90days" className="text-sm font-normal cursor-pointer">
                    90 days
                  </Label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="custom" id="custom" />
                  <Label htmlFor="custom" className="text-sm font-normal cursor-pointer">
                    Custom date
                  </Label>
                </div>
              </RadioGroup>
              
              {expiration === 'custom' && (
                <div className="pt-2">
                  <Input
                    type="datetime-local"
                    value={customDate}
                    onChange={(e) => setCustomDate(e.target.value)}
                    min={new Date().toISOString().slice(0, 16)}
                  />
                </div>
              )}
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={createMutation.isPending || !name.trim()}>
              {createMutation.isPending ? 'Creating...' : 'Create API Key'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
