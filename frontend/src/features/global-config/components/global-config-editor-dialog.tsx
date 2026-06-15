'use client'

import { useState, useEffect } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { AlertCircle } from 'lucide-react'
import { useUpdateGlobalConfig, type GlobalConfig } from '../api/use-global-config'
import { useQueryClient } from '@tanstack/react-query'

interface GlobalConfigEditorDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentConfig: Record<string, unknown>
  currentVersion: number
}

export function GlobalConfigEditorDialog({
  open,
  onOpenChange,
  currentConfig,
  currentVersion,
}: GlobalConfigEditorDialogProps) {
  const [jsonValue, setJsonValue] = useState('')
  const [parseError, setParseError] = useState<string | null>(null)
  const updateMutation = useUpdateGlobalConfig()
  const queryClient = useQueryClient()

  useEffect(() => {
    if (open) {
      setJsonValue(JSON.stringify(currentConfig, null, 2))
      setParseError(null)
    }
  }, [open, currentConfig])

  const handleSave = () => {
    try {
      const parsed = JSON.parse(jsonValue)
      setParseError(null)
      
      const cachedData = queryClient.getQueryData<GlobalConfig>(['globalConfig'])
      const latestVersion = cachedData?.version ?? currentVersion
      if (process.env.NODE_ENV !== 'production') {
        console.log('[GlobalConfig] Editor: sending version:', latestVersion)
        console.log('[GlobalConfig] Editor: current query version:', cachedData?.version ?? 'not in cache')
      }
      
      updateMutation.mutate(
        { config: parsed, version: latestVersion },
        {
          onSuccess: () => {
            onOpenChange(false)
          },
        }
      )
    } catch (error) {
      setParseError(error instanceof Error ? error.message : 'Invalid JSON')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>Edit Global Configuration</DialogTitle>
          <DialogDescription>
            Edit the global configuration JSON. Changes will be saved with version control.
            Current version: {currentVersion}
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 overflow-hidden">
          <Textarea
            value={jsonValue}
            onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setJsonValue(e.target.value)}
            className="font-mono text-xs h-full min-h-[400px] resize-none"
            placeholder="Enter JSON configuration..."
          />
        </div>

        {parseError && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{parseError}</AlertDescription>
          </Alert>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleSave}
            disabled={updateMutation.isPending || !jsonValue.trim()}
          >
            {updateMutation.isPending ? 'Saving...' : 'Save Configuration'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
