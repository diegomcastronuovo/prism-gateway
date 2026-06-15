'use client'

import { useState, useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { SectionCard } from '@/components/shared/section-card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Code2, Layers } from 'lucide-react'

export type CodingModel = {
  family: 'haiku' | 'sonnet' | 'opus'
  input_price: number
  output_price: number
}

interface CodingModelEditPanelProps {
  model: CodingModel | null
}

export function CodingModelEditPanel({ model }: CodingModelEditPanelProps) {
  const queryClient = useQueryClient()
  const [inputPrice, setInputPrice] = useState('')
  const [outputPrice, setOutputPrice] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saveSuccess, setSaveSuccess] = useState(false)

  useEffect(() => {
    if (model) {
      setInputPrice(String(model.input_price))
      setOutputPrice(String(model.output_price))
      setSaveError(null)
      setSaveSuccess(false)
    }
  }, [model])

  if (!model) {
    return (
      <SectionCard title="Coding Model Details">
        <div className="text-center py-12 text-muted-foreground">
          <Layers className="h-12 w-12 mx-auto mb-4 opacity-50" />
          <p>Select a coding model to edit pricing</p>
        </div>
      </SectionCard>
    )
  }

  const handleSave = async () => {
    const inputVal = parseFloat(inputPrice)
    const outputVal = parseFloat(outputPrice)

    if (isNaN(inputVal) || inputVal < 0 || isNaN(outputVal) || outputVal < 0) {
      setSaveError('Prices must be valid numbers >= 0')
      return
    }

    setSaving(true)
    setSaveError(null)
    setSaveSuccess(false)

    try {
      const res = await fetch('/api/providers/claude-code/coding_models', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify([
          {
            family: model.family,
            input_price: inputVal,
            output_price: outputVal,
          },
        ]),
      })

      if (!res.ok) {
        const data = await res.json().catch(() => ({}))
        const msg = data?.error?.message ?? data?.error ?? `Error ${res.status}`
        setSaveError(String(msg))
        return
      }

      setSaveSuccess(true)
      queryClient.invalidateQueries({ queryKey: ['claude-code-coding-models'] })
    } catch {
      setSaveError('Failed to save pricing')
    } finally {
      setSaving(false)
    }
  }

  const isDirty =
    String(model.input_price) !== inputPrice ||
    String(model.output_price) !== outputPrice

  return (
    <SectionCard title="Coding Model Details">
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center gap-2 flex-wrap">
          <Code2 className="h-5 w-5 text-primary" />
          <h3 className="text-lg font-semibold capitalize">{model.family}</h3>
          <Badge variant="secondary">Claude Code</Badge>
        </div>

        <div className="border-t" />

        {/* Pricing fields */}
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="input_price">Input Price (per 1M tokens)</Label>
            <Input
              id="input_price"
              type="number"
              min="0"
              step="0.01"
              value={inputPrice}
              onChange={(e) => {
                setInputPrice(e.target.value)
                setSaveSuccess(false)
              }}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="output_price">Output Price (per 1M tokens)</Label>
            <Input
              id="output_price"
              type="number"
              min="0"
              step="0.01"
              value={outputPrice}
              onChange={(e) => {
                setOutputPrice(e.target.value)
                setSaveSuccess(false)
              }}
            />
          </div>
        </div>

        {saveError && (
          <p className="text-sm text-destructive">{saveError}</p>
        )}
        {saveSuccess && (
          <p className="text-sm text-green-600 dark:text-green-400">Pricing updated successfully</p>
        )}

        <Button
          onClick={handleSave}
          disabled={saving || !isDirty}
          className="w-full"
        >
          {saving ? 'Saving…' : 'Save Pricing'}
        </Button>
      </div>
    </SectionCard>
  )
}
