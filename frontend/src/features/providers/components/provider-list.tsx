'use client'

import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { AlertCircle, CheckCircle2, XCircle } from 'lucide-react'
import type { Provider } from '../api/use-providers'
import { cn } from '@/lib/utils/cn'
import { providerDisplayTitle } from '../lib/provider-display'
import { ProviderIcon } from './provider-icon'

interface ProviderListProps {
  providers: Provider[] | undefined
  isLoading: boolean
  selectedProviderId: string | null
  onSelectProvider: (providerId: string) => void
}

// Map backend status to UI display
function getStatusBadge(provider: Provider): {
  label: string
  variant: 'default' | 'secondary' | 'destructive' | 'outline'
  icon: React.ReactNode
} {
  switch (provider.status) {
    case 'ready':
      return { label: 'Ready', variant: 'default', icon: <CheckCircle2 className="h-3 w-3" /> }
    case 'missing_credentials':
      return { label: 'Missing Credentials', variant: 'destructive', icon: <AlertCircle className="h-3 w-3" /> }
    case 'disabled':
      return { label: 'Disabled', variant: 'secondary', icon: <XCircle className="h-3 w-3" /> }
    default:
      return { label: provider.status || 'Unknown', variant: 'outline', icon: <AlertCircle className="h-3 w-3" /> }
  }
}

// Map api_key_source to display label
function getSourceLabel(source: string): string {
  switch (source) {
    case 'env':
      return 'Environment'
    case 'stored':
      return 'Stored'
    case 'missing':
      return 'Missing'
    default:
      return source
  }
}

export function ProviderList({ providers, isLoading, selectedProviderId, onSelectProvider }: ProviderListProps) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {[...Array(5)].map((_, i) => (
          <Skeleton key={i} className="h-16 w-full" />
        ))}
      </div>
    )
  }

  if (!providers || providers.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        No providers configured
      </div>
    )
  }

  return (
    <div className="space-y-2">
      {providers.map((provider) => {
        const status = getStatusBadge(provider)
        const isSelected = selectedProviderId === provider.id
        
        return (
          <button
            key={provider.id}
            onClick={() => onSelectProvider(provider.id)}
            className={cn(
              'w-full text-left p-4 rounded-lg border transition-colors',
              isSelected
                ? 'bg-primary text-primary-foreground border-primary'
                : 'bg-card hover:bg-accent hover:text-accent-foreground'
            )}
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <ProviderIcon providerId={provider.id} size="lg" />
                <div>
                  <p className="font-medium">{providerDisplayTitle(provider.id)}</p>
                  <p className="text-xs text-muted-foreground">
                    {provider.type} • {getSourceLabel(provider.api_key_source)}
                  </p>
                </div>
              </div>
              <Badge variant={status.variant} className="flex items-center gap-1">
                {status.icon}
                {status.label}
              </Badge>
            </div>
          </button>
        )
      })}
    </div>
  )
}
