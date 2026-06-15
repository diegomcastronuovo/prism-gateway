'use client'

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { WHITE_LABEL, BRAND_NAME } from '@/lib/config/branding'

interface AboutDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AboutDialog({ open, onOpenChange }: AboutDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl border-t-4 border-t-indigo-500">
        <DialogHeader>
          <DialogTitle className="text-center text-2xl font-bold">
            {WHITE_LABEL ? (BRAND_NAME || 'Admin Panel') : 'PrismGateway'}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-6 text-center py-4">
          <div className="border-t border-b py-6">
            <p className="text-base leading-relaxed">
              An intelligent control plane for AI infrastructure
            </p>
            <p className="text-base leading-relaxed">
              AI Routing, Governance &amp; Cost Control
            </p>
          </div>

          <div className="space-y-4 text-sm leading-relaxed text-muted-foreground">
            <p>
              It sits between applications and AI providers, providing smart routing,
              tool orchestration, semantic intent detection, cost optimization, and
              full observability for large language model traffic.
            </p>

            <p>
              Instead of hard-coding model choices, PrismGateway dynamically selects
              the best provider or model based on latency, cost, reliability, and
              context — while enforcing budgets, rate limits, and PII policies.
            </p>

            <p>
              With built-in benchmarking, budget analytics, and routing explainability,
              PrismGateway turns AI usage from a black box into a fully observable and
              optimizable platform.
            </p>
          </div>

          <div className="border-t pt-6">
            <p className="text-xs text-muted-foreground">
              {WHITE_LABEL
                ? (BRAND_NAME ? `© 2026 ${BRAND_NAME} - All Rights Reserved.` : '')
                : 'PrismGateway — Open Source AI Gateway'}
            </p>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
