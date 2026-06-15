'use client'

import { useEffect, useState } from 'react'
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
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  useCreateToolCatalogEntry,
  useUpdateToolCatalogEntry,
  type ToolCatalogEntry,
  type CreateToolCatalogEntryInput,
} from '../api/use-tool-catalog'
import { useProviders } from '@/features/providers/api/use-providers'
import { catalogProviderIdsForModels } from '@/features/providers/lib/catalog-provider-ids'

interface ToolCatalogDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  entry?: ToolCatalogEntry
}

export function ToolCatalogDialog({ open, onOpenChange, entry }: ToolCatalogDialogProps) {
  const isEdit = !!entry

  const createEntry = useCreateToolCatalogEntry()
  const updateEntry = useUpdateToolCatalogEntry()
  const { data: runtimeProviders } = useProviders()
  const providerList = catalogProviderIdsForModels(runtimeProviders)

  const [id, setId] = useState('')
  const [provider, setProvider] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [toolType, setToolType] = useState('')
  const [unit, setUnit] = useState('call')
  const [pricePerUnit, setPricePerUnit] = useState('0')
  const [isActive, setIsActive] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (entry) {
      setId(entry.id)
      setProvider(entry.provider)
      setDisplayName(entry.display_name)
      setToolType(entry.tool_type)
      setUnit(entry.unit || 'call')
      setPricePerUnit(String(entry.price_per_unit))
      setIsActive(entry.is_active)
    } else {
      setId('')
      setProvider('')
      setDisplayName('')
      setToolType('')
      setUnit('call')
      setPricePerUnit('0')
      setIsActive(true)
    }
    setError('')
  }, [entry, open])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')

    if (!id.trim()) {
      setError('Tool ID is required')
      return
    }
    if (!provider.trim()) {
      setError('Provider is required')
      return
    }

    const priceVal = parseFloat(pricePerUnit)
    if (!Number.isFinite(priceVal) || priceVal < 0) {
      setError('Price per unit must be 0 or greater')
      return
    }

    try {
      if (isEdit && entry) {
        await updateEntry.mutateAsync({
          provider: entry.provider,
          id: entry.id,
          data: {
            display_name: displayName.trim(),
            tool_type: toolType.trim(),
            unit,
            price_per_unit: priceVal,
            is_active: isActive,
          },
        })
      } else {
        const payload: CreateToolCatalogEntryInput = {
          id: id.trim(),
          provider: provider.trim(),
          display_name: displayName.trim(),
          tool_type: toolType.trim(),
          unit,
          price_per_unit: priceVal,
          is_active: isActive,
        }
        await createEntry.mutateAsync(payload)
      }
      onOpenChange(false)
    } catch {
      // Error handled by mutation
    }
  }

  const isPending = createEntry.isPending || updateEntry.isPending

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? 'Edit Tool Catalog Entry' : 'Add Tool Catalog Entry'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Update pricing and metadata for this tool catalog entry.'
              : 'Add a new tool to the pricing catalog.'}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="text-sm text-destructive bg-destructive/10 p-2 rounded">
              {error}
            </div>
          )}

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="tool-catalog-id">Tool ID *</Label>
              <Input
                id="tool-catalog-id"
                placeholder="e.g., web_search_standard"
                value={id}
                onChange={(e) => setId(e.target.value)}
                disabled={isEdit}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="tool-catalog-provider">Provider *</Label>
              <Select value={provider} onValueChange={setProvider} disabled={isEdit}>
                <SelectTrigger id="tool-catalog-provider">
                  <SelectValue placeholder="Select provider" />
                </SelectTrigger>
                <SelectContent>
                  {providerList.map((p) => (
                    <SelectItem key={p} value={p}>{p}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="tool-catalog-display-name">Display Name</Label>
            <Input
              id="tool-catalog-display-name"
              placeholder="e.g., Web Search"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="tool-catalog-tool-type">Tool Type</Label>
            <Input
              id="tool-catalog-tool-type"
              placeholder="e.g., web_search, file_search, container"
              value={toolType}
              onChange={(e) => setToolType(e.target.value)}
            />
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="tool-catalog-unit">Unit</Label>
              <Select value={unit} onValueChange={setUnit}>
                <SelectTrigger id="tool-catalog-unit">
                  <SelectValue placeholder="Select unit" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="call">call</SelectItem>
                  <SelectItem value="session">session</SelectItem>
                  <SelectItem value="gb_day">gb_day</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="tool-catalog-price">Price per Unit (USD)</Label>
              <Input
                id="tool-catalog-price"
                type="number"
                min="0"
                step="0.000001"
                placeholder="0.01"
                value={pricePerUnit}
                onChange={(e) => setPricePerUnit(e.target.value)}
              />
            </div>
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="tool-catalog-is-active">Active</Label>
            <Switch id="tool-catalog-is-active" checked={isActive} onCheckedChange={setIsActive} />
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isPending}>
              {isPending ? (isEdit ? 'Saving...' : 'Creating...') : (isEdit ? 'Save Changes' : 'Add Entry')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
