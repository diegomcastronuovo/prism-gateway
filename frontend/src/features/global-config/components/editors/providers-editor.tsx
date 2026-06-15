'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'

interface Provider {
  Type: string
  BaseURL: string
  APIKeyEnv: string
  Enabled: boolean | null
}

interface ProvidersEditorProps {
  providers: Record<string, Provider>
  onSave: (updatedProviders: Record<string, Provider>) => void
  onCancel: () => void
}

export function ProvidersEditor({ providers, onSave, onCancel }: ProvidersEditorProps) {
  const [editedProviders, setEditedProviders] = useState<Record<string, Provider>>(
    JSON.parse(JSON.stringify(providers))
  )

  const handleProviderChange = (
    key: string,
    field: keyof Provider,
    value: string | boolean | null
  ) => {
    setEditedProviders((prev) => ({
      ...prev,
      [key]: {
        ...prev[key],
        [field]: value,
      },
    }))
  }

  const handleEnabledChange = (key: string, value: string) => {
    let enabledValue: boolean | null = null
    if (value === 'true') enabledValue = true
    if (value === 'false') enabledValue = false
    handleProviderChange(key, 'Enabled', enabledValue)
  }

  const handleSave = () => {
    onSave(editedProviders)
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {Object.entries(editedProviders).map(([key, provider]) => (
          <div key={key} className="flex flex-col gap-3 p-4 rounded-lg border bg-card">
            <div className="flex items-center justify-between mb-2">
              <span className="font-semibold">{key}</span>
              <Badge variant="outline">{provider.Type}</Badge>
            </div>

            <div className="space-y-3">
              <div className="space-y-1">
                <Label htmlFor={`${key}-type`} className="text-xs">
                  Type
                </Label>
                <Input
                  id={`${key}-type`}
                  value={provider.Type}
                  onChange={(e) => handleProviderChange(key, 'Type', e.target.value)}
                  className="h-8 text-sm"
                />
              </div>

              <div className="space-y-1">
                <Label htmlFor={`${key}-baseurl`} className="text-xs">
                  Base URL
                </Label>
                <Input
                  id={`${key}-baseurl`}
                  value={provider.BaseURL}
                  onChange={(e) => handleProviderChange(key, 'BaseURL', e.target.value)}
                  className="h-8 text-sm font-mono"
                  placeholder="https://api.example.com/v1"
                />
              </div>

              <div className="space-y-1">
                <Label htmlFor={`${key}-apikey`} className="text-xs">
                  API Key Env Var
                </Label>
                <Input
                  id={`${key}-apikey`}
                  value={provider.APIKeyEnv}
                  onChange={(e) => handleProviderChange(key, 'APIKeyEnv', e.target.value)}
                  className="h-8 text-sm font-mono"
                  placeholder="PROVIDER_API_KEY"
                />
              </div>

              <div className="space-y-1">
                <Label htmlFor={`${key}-enabled`} className="text-xs">
                  Enabled
                </Label>
                <select
                  id={`${key}-enabled`}
                  value={
                    provider.Enabled === null ? 'null' : provider.Enabled ? 'true' : 'false'
                  }
                  onChange={(e) => handleEnabledChange(key, e.target.value)}
                  className="flex h-8 w-full rounded-md border border-input bg-background px-3 py-1 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="null">Yes (default)</option>
                  <option value="true">Yes</option>
                  <option value="false">No</option>
                </select>
              </div>
            </div>
          </div>
        ))}
      </div>

      <div className="flex gap-2 pt-4 border-t">
        <Button onClick={handleSave}>Save Changes</Button>
        <Button variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </div>
  )
}
