'use client'

import { RequireAdminRole } from '@/components/auth/require-admin-role'
import { PageHeader } from '@/components/layout/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { EmptyState } from '@/components/shared/empty-state'
import { Button } from '@/components/ui/button'
import { Plus, PlayCircle } from 'lucide-react'

function ReplayContent() {
  return (
    <div>
      <PageHeader
        title="Replay"
        description="Replay and analyze historical requests"
        action={
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            New Replay
          </Button>
        }
      />

      <SectionCard title="Replay Sessions">
        <EmptyState
          icon={PlayCircle}
          title="No replay sessions"
          description="Create replay sessions to test and analyze historical request patterns"
          action={
            <Button>
              <Plus className="mr-2 h-4 w-4" />
              Start Replay
            </Button>
          }
        />
      </SectionCard>
    </div>
  )
}

export default function ReplayPage() {
  return (
    <RequireAdminRole>
      <ReplayContent />
    </RequireAdminRole>
  )
}
