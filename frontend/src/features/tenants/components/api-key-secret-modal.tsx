'use client'

import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Copy, Check, AlertTriangle } from 'lucide-react'

interface ApiKeySecretModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  apiKey: string
  isRotation?: boolean
}

export function ApiKeySecretModal({
  open,
  onOpenChange,
  apiKey,
  isRotation = false,
}: ApiKeySecretModalProps) {
  const [copied, setCopied] = useState(false)
  
  console.log('ApiKeySecretModal rendered with apiKey:', apiKey)
  console.log('apiKey type:', typeof apiKey)
  console.log('apiKey length:', apiKey?.length)

  const handleCopy = async () => {
    if (!apiKey) return
    await navigator.clipboard.writeText(apiKey)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isRotation ? 'API Key Rotated' : 'API Key Created'}
          </DialogTitle>
          <DialogDescription>
            Copy this key now. It will not be shown again.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <Alert variant="destructive">
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>
              This is the only time you will see the full API key. Make sure to copy it now.
            </AlertDescription>
          </Alert>

          {/* API Key Display */}
          <div className="space-y-2">
            <Label className="text-sm font-medium">Your API Key</Label>
            <div className="relative">
              <div className="bg-slate-100 dark:bg-slate-800 p-4 rounded-lg font-mono text-sm break-all border border-slate-300 dark:border-slate-600 text-slate-900 dark:text-slate-100 min-h-[60px] flex items-center">
                {apiKey || 'Error: No API key received'}
              </div>
            </div>
          </div>

          {/* Action Buttons */}
          <div className="flex gap-3 pt-2">
            <Button
              onClick={handleCopy}
              disabled={!apiKey}
              className="flex-1"
              variant={copied ? 'default' : 'outline'}
            >
              {copied ? (
                <>
                  <Check className="h-4 w-4 mr-2" />
                  Copied!
                </>
              ) : (
                <>
                  <Copy className="h-4 w-4 mr-2" />
                  Copy API Key
                </>
              )}
            </Button>
            <Button 
              onClick={() => onOpenChange(false)}
              variant="default"
              className="flex-1"
            >
              I&apos;ve Saved the Key
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
