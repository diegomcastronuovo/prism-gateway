'use client'

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

interface BenchmarkDetailDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function BenchmarkDetailDialog({
  open,
  onOpenChange,
}: BenchmarkDetailDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Coming Soon</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          Detailed benchmark drilldown is not available in the current
          aggregate-only flow.
        </p>
        <div className="flex justify-end">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
