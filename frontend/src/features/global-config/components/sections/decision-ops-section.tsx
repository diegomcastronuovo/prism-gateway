'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Edit, Check, X } from 'lucide-react'

interface DecisionOpsSectionProps {
  config: Record<string, unknown>
  onUpdate?: (updated: Record<string, unknown>) => void
}

function formatTTL(seconds: number): string {
  const mins = Math.floor(seconds / 60)
  const hours = (seconds / 3600).toFixed(2)
  return `${seconds}s — ${mins} min — ${hours}h`
}

export function DecisionOpsSection({ config, onUpdate }: DecisionOpsSectionProps) {
  const currentTTL = (config.workflow_conversation_ttl_seconds as number | undefined) ?? 3600
  const [isEditing, setIsEditing] = useState(false)
  const [inputValue, setInputValue] = useState(String(currentTTL))
  const [inputError, setInputError] = useState<string | null>(null)

  const parsed = parseInt(inputValue, 10)
  const isValid = !isNaN(parsed) && parsed > 0

  const handleEdit = () => {
    setInputValue(String(currentTTL))
    setInputError(null)
    setIsEditing(true)
  }

  const handleCancel = () => {
    setIsEditing(false)
    setInputError(null)
  }

  const handleSave = () => {
    if (!isValid) {
      setInputError('Must be a positive integer (seconds)')
      return
    }
    if (onUpdate) {
      onUpdate({ workflow_conversation_ttl_seconds: parsed })
    }
    setIsEditing(false)
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 p-4 rounded-lg border bg-card">
        <div className="flex items-center justify-between">
          <h4 className="font-semibold">Conversation TTL</h4>
          {!isEditing && (
            <Button size="sm" variant="ghost" onClick={handleEdit}>
              <Edit className="h-4 w-4 mr-1" />
              Edit
            </Button>
          )}
        </div>

        {isEditing ? (
          <div className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="ttl-input">TTL (seconds)</Label>
              <Input
                id="ttl-input"
                type="number"
                min={1}
                value={inputValue}
                onChange={(e) => {
                  setInputValue(e.target.value)
                  setInputError(null)
                }}
                className="w-48"
              />
              {inputError && (
                <p className="text-xs text-destructive">{inputError}</p>
              )}
            </div>
            {isValid && (
              <p className="text-xs text-muted-foreground">
                <span className="font-medium text-foreground">Equivalent:</span>{' '}
                {formatTTL(parsed)}
              </p>
            )}
            <div className="flex gap-2">
              <Button size="sm" onClick={handleSave}>
                <Check className="h-4 w-4 mr-1" />
                Save
              </Button>
              <Button size="sm" variant="outline" onClick={handleCancel}>
                <X className="h-4 w-4 mr-1" />
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <div className="space-y-1">
            <p className="text-sm font-mono">{formatTTL(currentTTL)}</p>
            <p className="text-xs text-muted-foreground">
              Inactive workflow conversations are purged after this period. Changes take effect within seconds (cached globally).
            </p>
          </div>
        )}
      </div>
    </div>
  )
}
