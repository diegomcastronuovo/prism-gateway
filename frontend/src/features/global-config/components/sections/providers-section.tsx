'use client'

import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Edit } from 'lucide-react'
import { ProvidersEditor } from '../editors/providers-editor'

const providerLogoMap: Record<string, string> = {
  openai: '/openai_logo.png',
  anthropic: '/anthropic_logo.png',
  gemini: '/gemini_logo.png',
  grok: '/grok_logo.png',
  local: '/local_logo.png',
  xai: '/xai_logo.png',
}

interface Provider {
  Type: string
  BaseURL: string
  APIKeyEnv: string
  Enabled: boolean | null
}

interface ProvidersSectionProps {
  config: Record<string, unknown>
  onUpdate?: (updatedProviders: Record<string, Provider>) => void
}

export function ProvidersSection({ config, onUpdate }: ProvidersSectionProps) {
  const [isEditing, setIsEditing] = useState(false)
  const providers = config.providers as Record<string, unknown> | undefined

  const handleSave = (updatedProviders: Record<string, Provider>) => {
    if (onUpdate) {
      onUpdate(updatedProviders)
    }
    setIsEditing(false)
  }

  const handleCancel = () => {
    setIsEditing(false)
  }

  if (!providers || Object.keys(providers).length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>No providers configured</p>
      </div>
    )
  }

  if (isEditing) {
    return (
      <ProvidersEditor
        providers={providers as Record<string, Provider>}
        onSave={handleSave}
        onCancel={handleCancel}
      />
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setIsEditing(true)}>
          <Edit className="mr-2 h-4 w-4" />
          Edit Providers
        </Button>
      </div>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {Object.entries(providers).map(([key, value]) => {
        const provider = value as Record<string, unknown>
        const normKey = String(key).toLowerCase()
        const logoSrc = providerLogoMap[normKey]
        return (
          <div key={key} className="relative flex flex-col gap-3 p-4 rounded-lg border bg-card">
            <div className="flex items-center justify-between">
              <span className="font-semibold">{key}</span>
              {provider.Type !== undefined && (
                <Badge variant="outline">{String(provider.Type)}</Badge>
              )}
            </div>
            
            {provider.BaseURL !== undefined && (
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Base URL</span>
                <span className="font-mono text-xs break-all">{String(provider.BaseURL)}</span>
              </div>
            )}

            {provider.APIKeyEnv !== undefined && (
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Credential Env Var</span>
                <Badge variant="secondary" className="w-fit font-mono text-xs">
                  ${String(provider.APIKeyEnv)}
                </Badge>
              </div>
            )}

            {provider.Enabled !== undefined && (
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">Enabled</span>
                <Badge
                  variant={provider.Enabled === false ? 'secondary' : 'default'}
                  className="w-fit"
                >
                  {provider.Enabled === null
                    ? 'Yes (default)'
                    : provider.Enabled
                    ? 'Yes'
                    : 'No'}
                </Badge>
              </div>
            )}

            {logoSrc && (
              <img
                src={logoSrc}
                alt={`${key} logo`}
                className="absolute bottom-2 right-2 h-[72px] w-[72px] object-contain opacity-90"
              />
            )}
          </div>
        )
      })}
      </div>
    </div>
  )
}
