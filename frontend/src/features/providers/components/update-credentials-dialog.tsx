'use client'

import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Eye, EyeOff, KeyRound, Shield } from 'lucide-react'
import type { ProviderWithVersion } from '../api/use-providers'
import { useUpdateProviderCredentials } from '../api/use-providers'
import { getVersionForProviderMutation } from '../api/provider-mutation-version'
import { isAwsBedrockProvider, providerDisplayTitle } from '../lib/provider-display'

interface UpdateCredentialsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  provider: ProviderWithVersion | null
}

export function UpdateCredentialsDialog({ open, onOpenChange, provider }: UpdateCredentialsDialogProps) {
  const queryClient = useQueryClient()
  const updateCredentials = useUpdateProviderCredentials()
  const [apiKey, setApiKey] = useState('')
  const [apiSecret, setApiSecret] = useState('')
  const [organization, setOrganization] = useState('')
  const [showApiKey, setShowApiKey] = useState(false)
  const [showApiSecret, setShowApiSecret] = useState(false)

  const [bedrockAccessKey, setBedrockAccessKey] = useState('')
  const [bedrockSecretKey, setBedrockSecretKey] = useState('')
  const [bedrockRegion, setBedrockRegion] = useState('')
  const [showBedrockAccess, setShowBedrockAccess] = useState(false)
  const [showBedrockSecret, setShowBedrockSecret] = useState(false)

  const isBedrock = provider ? isAwsBedrockProvider(provider) : false

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!provider) return

    if (isBedrock) {
      const ak = bedrockAccessKey.trim()
      const sk = bedrockSecretKey.trim()
      const region = bedrockRegion.trim()
      if (!ak || !sk || !region) return

      try {
        const version = getVersionForProviderMutation(queryClient, provider.version)
        await updateCredentials.mutateAsync({
          providerId: provider.id,
          credentials: {
            aws_access_key_id: ak,
            aws_secret_access_key: sk,
            aws_region: region,
          },
          version,
        })
        setBedrockAccessKey('')
        setBedrockSecretKey('')
        setBedrockRegion('')
        onOpenChange(false)
      } catch {
        // Error is handled by the mutation
      }
      return
    }

    if (!apiKey.trim()) {
      return // Validation handled by required attribute
    }

    try {
      const version = getVersionForProviderMutation(queryClient, provider.version)
      await updateCredentials.mutateAsync({
        providerId: provider.id,
        credentials: {
          api_key: apiKey,
          api_secret: apiSecret || undefined,
          organization: organization || undefined,
        },
        version,
      })
      setApiKey('')
      setApiSecret('')
      setOrganization('')
      onOpenChange(false)
    } catch {
      // Error is handled by the mutation
    }
  }

  const hasExistingCredentials = provider?.has_api_key ?? false

  const bedrockCanSubmit =
    bedrockAccessKey.trim() !== '' && bedrockSecretKey.trim() !== '' && bedrockRegion.trim() !== ''

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5" />
            {hasExistingCredentials ? 'Update Credentials' : 'Add Credentials'}
          </DialogTitle>
          <DialogDescription>
            {hasExistingCredentials
              ? `Update credentials for ${provider ? providerDisplayTitle(provider.id) : 'provider'}`
              : `Add credentials for ${provider ? providerDisplayTitle(provider.id) : 'provider'}`}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <Alert>
            <Shield className="h-4 w-4" />
            <AlertDescription>
              Credentials are stored server-side only and never returned to the browser.
            </AlertDescription>
          </Alert>

          {isBedrock ? (
            <>
              <div className="space-y-2">
                <Label htmlFor="bedrock_cred_access">AWS Access Key ID</Label>
                <div className="relative">
                  <Input
                    id="bedrock_cred_access"
                    type={showBedrockAccess ? 'text' : 'password'}
                    value={bedrockAccessKey}
                    onChange={(e) => setBedrockAccessKey(e.target.value)}
                    placeholder="AKIA…"
                    required
                    autoComplete="off"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="absolute right-0 top-0 h-full px-3"
                    onClick={() => setShowBedrockAccess(!showBedrockAccess)}
                  >
                    {showBedrockAccess ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </Button>
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="bedrock_cred_secret">AWS Secret Access Key</Label>
                <div className="relative">
                  <Input
                    id="bedrock_cred_secret"
                    type={showBedrockSecret ? 'text' : 'password'}
                    value={bedrockSecretKey}
                    onChange={(e) => setBedrockSecretKey(e.target.value)}
                    placeholder="Secret access key"
                    required
                    autoComplete="off"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="absolute right-0 top-0 h-full px-3"
                    onClick={() => setShowBedrockSecret(!showBedrockSecret)}
                  >
                    {showBedrockSecret ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </Button>
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="bedrock_cred_region">AWS Region</Label>
                <Input
                  id="bedrock_cred_region"
                  value={bedrockRegion}
                  onChange={(e) => setBedrockRegion(e.target.value)}
                  placeholder="e.g. us-east-1"
                  required
                  autoComplete="off"
                />
              </div>
            </>
          ) : (
            <>
              <div className="space-y-2">
                <Label htmlFor="api_key">
                  API Key {hasExistingCredentials && '(required to update)'}
                </Label>
                <div className="relative">
                  <Input
                    id="api_key"
                    type={showApiKey ? 'text' : 'password'}
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    placeholder="Enter API key"
                    required
                    autoComplete="off"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="absolute right-0 top-0 h-full px-3"
                    onClick={() => setShowApiKey(!showApiKey)}
                  >
                    {showApiKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  {hasExistingCredentials
                    ? 'Enter new API key to replace existing credentials'
                    : 'Enter your API key from the provider dashboard'}
                </p>
              </div>

              {(provider?.id === 'gemini' || provider?.id === 'xai') && (
                <div className="space-y-2">
                  <Label htmlFor="api_secret">API Secret (optional)</Label>
                  <div className="relative">
                    <Input
                      id="api_secret"
                      type={showApiSecret ? 'text' : 'password'}
                      value={apiSecret}
                      onChange={(e) => setApiSecret(e.target.value)}
                      placeholder="Enter API secret"
                      autoComplete="off"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="absolute right-0 top-0 h-full px-3"
                      onClick={() => setShowApiSecret(!showApiSecret)}
                    >
                      {showApiSecret ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </Button>
                  </div>
                </div>
              )}

              {(provider?.id === 'openai' || provider?.id === 'anthropic') && (
                <div className="space-y-2">
                  <Label htmlFor="organization">Organization ID (optional)</Label>
                  <Input
                    id="organization"
                    type="text"
                    value={organization}
                    onChange={(e) => setOrganization(e.target.value)}
                    placeholder="org-..."
                  />
                  <p className="text-xs text-muted-foreground">
                    Optional organization ID for enterprise accounts
                  </p>
                </div>
              )}
            </>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={updateCredentials.isPending || (isBedrock ? !bedrockCanSubmit : !apiKey.trim())}
            >
              {updateCredentials.isPending
                ? 'Saving...'
                : hasExistingCredentials
                  ? 'Update Credentials'
                  : 'Add Credentials'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
