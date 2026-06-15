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
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useUpdateTenantBudget, type TenantBudget } from '../api/use-budgets'

const TIMEZONES = [
  // North America
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Mexico_City',
  // LATAM
  'America/Bogota',
  'America/Lima',
  'America/Santiago',
  'America/Buenos_Aires',
  'America/Sao_Paulo',
  'America/Montevideo',
  'America/Asuncion',
  'America/La_Paz',
  'America/Guayaquil',
  'America/Caracas',
  'America/Guatemala',
  'America/Costa_Rica',
  'America/Panama',
  'America/El_Salvador',
  'America/Havana',
  'America/Santo_Domingo',
  'America/Puerto_Rico',
  // Europe
  'Europe/London',
  'Europe/Paris',
  // Asia
  'Asia/Tokyo',
  // UTC
  'UTC',
]

interface EditBudgetDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  budget: TenantBudget | null
}

export function EditBudgetDialog({ open, onOpenChange, budget }: EditBudgetDialogProps) {
  const updateBudget = useUpdateTenantBudget()
  const [monthlyUsd, setMonthlyUsd] = useState('')
  const [timezone, setTimezone] = useState('UTC')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (budget) {
      setMonthlyUsd(budget.monthly_usd?.toString() ?? '')
      setTimezone(budget.timezone ?? 'UTC')
      setError(null)
    }
  }, [budget])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!budget) return

    const monthlyValue = monthlyUsd ? parseFloat(monthlyUsd) : undefined
    if (monthlyValue !== undefined && (isNaN(monthlyValue) || monthlyValue <= 0)) {
      setError('Monthly limit must be greater than 0')
      return
    }

    try {
      await updateBudget.mutateAsync({
        tenantId: budget.tenant_id,
        version: budget.version,
        request: {
          monthly_usd: monthlyValue,
          timezone,
        },
      })
      onOpenChange(false)
    } catch {
      // Error handled by mutation toast
    }
  }

  if (!budget) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Edit Budget</DialogTitle>
          <DialogDescription>
            Update monthly limit and timezone for <strong>{budget.tenant_id}</strong>
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="edit-monthly">Monthly Limit (USD)</Label>
            <Input
              id="edit-monthly"
              type="number"
              step="0.01"
              min="0"
              value={monthlyUsd}
              onChange={(e) => setMonthlyUsd(e.target.value)}
              placeholder="500.00"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="edit-timezone">Timezone</Label>
            <Select value={timezone} onValueChange={setTimezone}>
              <SelectTrigger id="edit-timezone">
                <SelectValue placeholder="Select timezone" />
              </SelectTrigger>
              <SelectContent>
                {TIMEZONES.map((tz) => (
                  <SelectItem key={tz} value={tz}>
                    {tz}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={updateBudget.isPending}>
              {updateBudget.isPending ? 'Saving...' : 'Save'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
