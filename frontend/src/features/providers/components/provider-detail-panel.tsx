'use client'

import { useState } from 'react'
import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { Edit, KeyRound, ChevronDown, ChevronUp, Boxes, AlertCircle, CheckCircle2, XCircle, Globe, Clock, RefreshCw } from 'lucide-react'
import type { ProviderWithVersion } from '../api/use-providers'
import { isAwsBedrockProvider, maskAwsAccessKeyId, providerDetailHeading, providerDisplayTitle } from '../lib/provider-display'

interface ProviderDetailPanelProps {
  provider: ProviderWithVersion | undefined
  isLoading: boolean
  onEdit: () => void
  onToggleEnabled: (enabled: boolean) => void
}

// Map backend status to UI badge
function getStatusBadge(provider: ProviderWithVersion) {
  switch (provider.status) {
    case 'ready':
      return { label: 'Ready', variant: 'default' as const, icon: <CheckCircle2 className="h-4 w-4" /> }
    case 'missing_credentials':
      return { label: 'Missing Credentials', variant: 'destructive' as const, icon: <AlertCircle className="h-4 w-4" /> }
    case 'disabled':
      return { label: 'Disabled', variant: 'secondary' as const, icon: <XCircle className="h-4 w-4" /> }
    default:
      return { label: provider.status || 'Unknown', variant: 'outline' as const, icon: <AlertCircle className="h-4 w-4" /> }
  }
}

// Map api_key_source to display label
function getCredentialSourceLabel(source: string) {
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

function bedrockMaskedJsonPreview(provider: ProviderWithVersion) {
  return {
    type: 'aws_bedrock',
    aws_access_key_id: provider.aws_access_key_id ? maskAwsAccessKeyId(provider.aws_access_key_id) : '—',
    aws_secret_access_key: '***',
    aws_region: provider.aws_region || '',
  }
}

export function ProviderDetailPanel({
  provider,
  isLoading,
  onEdit,
  onToggleEnabled,
}: ProviderDetailPanelProps) {
  const [showRawJson, setShowRawJson] = useState(false)

  if (isLoading) {
    return (
      <SectionCard title="Provider Details" className="border-t-4 border-t-purple-500">
        <div className="space-y-4">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      </SectionCard>
    )
  }

  if (!provider) {
    return (
      <SectionCard title="Provider Details" className="border-t-4 border-t-purple-500">
        <div className="text-center py-12 text-muted-foreground">
          <Boxes className="h-12 w-12 mx-auto mb-4 opacity-50" />
          <p>Select a provider to view details</p>
        </div>
      </SectionCard>
    )
  }

  const status = getStatusBadge(provider)
  const bedrock = isAwsBedrockProvider(provider)
  const heading = providerDetailHeading(provider.id)
  const titleLabel = providerDisplayTitle(provider.id)

  return (
    <SectionCard title={heading} className="border-t-4 border-t-purple-500">
      <div className="space-y-6">
        {/* Summary Section */}
        <div className="space-y-4">
          <div className="flex items-start justify-between">
            <div className="space-y-2">
              <div className="flex items-center gap-2 flex-wrap">
                <h3 className="text-lg font-semibold">{titleLabel}</h3>
                {bedrock ? (
                  <Badge variant="secondary">AWS Bedrock</Badge>
                ) : null}
                <Badge variant={status.variant} className="flex items-center gap-1">
                  {status.icon}
                  {status.label}
                </Badge>
              </div>
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <span>Type: {provider.type}</span>
                <span>•</span>
                <span>Config Version: {provider.version}</span>
              </div>
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="outline" onClick={onEdit} className="min-w-[140px]">
                <span className="inline-flex items-center gap-2">
                  <Edit className="h-4 w-4" />
                  <span>Edit</span>
                </span>
              </Button>
            </div>
          </div>

          {/* Enable/Disable Toggle */}
          <div className="flex items-center justify-between rounded-lg border p-3">
            <div className="space-y-0.5">
              <p className="font-medium">Provider Status</p>
              <p className="text-sm text-muted-foreground">
                {provider.enabled ? 'Provider is enabled and can be used for routing' : 'Provider is disabled'}
              </p>
            </div>
            <Switch checked={provider.enabled} onCheckedChange={onToggleEnabled} />
          </div>
        </div>

        <div className="border-t" />

        {/* Connection Section */}
        <div className="space-y-3">
          <h4 className="font-medium flex items-center gap-2">
            <Globe className="h-4 w-4" />
            Connection
          </h4>
          <div className="grid gap-2 text-sm">
            {bedrock ? (
              <>
                <div className="flex justify-between gap-4">
                  <span className="text-muted-foreground shrink-0">Type</span>
                  <span className="font-mono text-right">aws_bedrock</span>
                </div>
                <div className="flex justify-between gap-4">
                  <span className="text-muted-foreground shrink-0">Region</span>
                  <span className="font-mono text-right break-all">{provider.aws_region || '—'}</span>
                </div>
              </>
            ) : (
              <>
                {provider.base_url && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Base URL</span>
                    <span className="font-mono">{provider.base_url}</span>
                  </div>
                )}
                {provider.api_version && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">API Version</span>
                    <span>{provider.api_version}</span>
                  </div>
                )}
                {provider.organization && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Organization</span>
                    <span>{provider.organization}</span>
                  </div>
                )}
                {provider.timeout_ms && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Timeout</span>
                    <span>{provider.timeout_ms}ms</span>
                  </div>
                )}
                {provider.max_retries !== undefined && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Max Retries</span>
                    <span>{provider.max_retries}</span>
                  </div>
                )}
                {!provider.base_url && !provider.api_version && !provider.organization && (
                  <p className="text-muted-foreground">Using default connection settings</p>
                )}
              </>
            )}
          </div>
        </div>

        <div className="border-t" />

        {/* Credentials Section */}
        <div className="space-y-3">
          <h4 className="font-medium flex items-center gap-2">
            <KeyRound className="h-4 w-4" />
            Credentials
          </h4>
          {bedrock ? (
            <div className="rounded-lg border p-3 space-y-3">
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm text-muted-foreground">AWS Access Key ID</span>
                <span className="text-sm font-mono">{maskAwsAccessKeyId(provider.aws_access_key_id)}</span>
              </div>
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm text-muted-foreground">AWS Secret Access Key</span>
                <span className="text-sm font-mono">{provider.aws_secret_configured ? '***' : '—'}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">Source</span>
                <span className="text-sm">{getCredentialSourceLabel(provider.api_key_source)}</span>
              </div>
            </div>
          ) : (
            <div className="rounded-lg border p-3 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">API Key</span>
                <Badge variant={provider.has_api_key ? 'default' : 'destructive'}>
                  {provider.has_api_key ? 'Configured' : 'Missing'}
                </Badge>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">Source</span>
                <span className="text-sm">{getCredentialSourceLabel(provider.api_key_source)}</span>
              </div>
              {provider.last_credential_update && (
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Last Updated</span>
                  <span className="text-sm flex items-center gap-1">
                    <Clock className="h-3 w-3" />
                    {new Date(provider.last_credential_update).toLocaleString()}
                  </span>
                </div>
              )}
            </div>
          )}
        </div>

        <div className="border-t" />

        {/* Capabilities Section */}
        <div className="space-y-3">
          <h4 className="font-medium flex items-center gap-2">
            <RefreshCw className="h-4 w-4" />
            Capabilities
          </h4>
          <div className="flex flex-wrap gap-2">
            <Badge variant="outline">Chat</Badge>
            {!bedrock ? <Badge variant="outline">Streaming</Badge> : null}
            {provider.id === 'openai' && <Badge variant="outline">Tool Calling</Badge>}
            {provider.id === 'anthropic' && <Badge variant="outline">Tool Calling</Badge>}
            {provider.id === 'gemini' && <Badge variant="outline">Embeddings</Badge>}
          </div>
        </div>

        <div className="border-t" />

        {/* Raw JSON */}
        <div>
          <Button variant="ghost" size="sm" onClick={() => setShowRawJson(!showRawJson)} className="mb-2">
            {showRawJson ? (
              <>
                <ChevronUp className="h-4 w-4 mr-2" />
                Hide Raw JSON
              </>
            ) : (
              <>
                <ChevronDown className="h-4 w-4 mr-2" />
                Show Raw JSON
              </>
            )}
          </Button>
          {showRawJson && (
            <pre className="bg-muted p-4 rounded-lg overflow-auto text-xs max-h-96">
              {bedrock
                ? JSON.stringify(bedrockMaskedJsonPreview(provider), null, 2)
                : JSON.stringify(provider, null, 2)}
            </pre>
          )}
        </div>
      </div>
    </SectionCard>
  )
}
