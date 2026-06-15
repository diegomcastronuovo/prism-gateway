import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'

interface SystemInfoPanelProps {
  version: {
    service: string
    backend_version: string
    git_commit: string
    build_time: string
    release_notes: string
  }
}

export function SystemInfoPanel({ version }: SystemInfoPanelProps) {
  if (!version) {
    return (
      <SectionCard
        title="System Information"
        description="Backend version and build details"
        className="border-t-4 border-t-cyan-400"
      >
        <div className="text-center py-8 text-muted-foreground">
          No system information available
        </div>
      </SectionCard>
    )
  }

  return (
    <SectionCard
      title="System Information"
      description="Backend version and build details"
      className="border-t-4 border-t-cyan-400"
    >
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">Service</span>
          <Badge variant="outline">{version.service || 'Unknown'}</Badge>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">Version</span>
          <span className="text-sm font-medium">{version.backend_version || 'Unknown'}</span>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">Git Commit</span>
          <code className="text-xs bg-muted px-2 py-1 rounded">{version.git_commit || 'Unknown'}</code>
        </div>
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">Build Time</span>
          <span className="text-sm">{version.build_time || 'Unknown'}</span>
        </div>
        {version.release_notes && (
          <div className="pt-2 border-t">
            <span className="text-sm text-muted-foreground">Release Notes</span>
            <p className="text-sm mt-1">{version.release_notes}</p>
          </div>
        )}
      </div>
    </SectionCard>
  )
}
