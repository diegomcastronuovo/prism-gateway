'use client'

import { useState } from 'react'
import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Edit, Trash2, ChevronDown, ChevronUp, GitBranch, Layers } from 'lucide-react'
import type { RouteGroup } from '../api/use-route-groups'

interface RouteGroupDetailPanelProps {
  routeGroup: RouteGroup | undefined
  isLoading: boolean
  onEdit: () => void
  onDelete: () => void
}

export function RouteGroupDetailPanel({
  routeGroup,
  isLoading,
  onEdit,
  onDelete,
}: RouteGroupDetailPanelProps) {
  const [showRawJson, setShowRawJson] = useState(false)

  if (isLoading) {
    return (
      <SectionCard title="Route Group Details" className="border-t-4 border-t-purple-500">
        <div className="space-y-4">
          <Skeleton className="h-8 w-3/4" />
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-32 w-full" />
        </div>
      </SectionCard>
    )
  }

  if (!routeGroup) {
    return (
      <SectionCard title="Route Group Details" className="border-t-4 border-t-purple-500">
        <div className="text-center py-12 text-muted-foreground">
          <GitBranch className="h-12 w-12 mx-auto mb-4 opacity-50" />
          <p>Select a route group to view details</p>
        </div>
      </SectionCard>
    )
  }

  const models = routeGroup.models || []

  return (
    <SectionCard
      title={routeGroup.id}
      description={`${models.length} model${models.length === 1 ? '' : 's'} in group`}
      className="border-t-4 border-t-purple-500"
    >
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <h3 className="text-lg font-semibold">{routeGroup.id}</h3>
              <Badge variant="outline">Active</Badge>
            </div>
          </div>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" onClick={onEdit} className="min-w-[140px]">
              <span className="inline-flex items-center gap-2">
                <Edit className="h-4 w-4" />
                <span>Edit</span>
              </span>
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={onDelete}
              className="min-w-[140px] text-destructive hover:bg-destructive/10"
            >
              <span className="inline-flex items-center gap-2">
                <Trash2 className="h-4 w-4" />
                <span>Delete</span>
              </span>
            </Button>
          </div>
        </div>

        <div className="border-t" />

        {/* Models Section */}
        <div className="space-y-3">
          <h4 className="font-medium flex items-center gap-2">
            <Layers className="h-4 w-4" />
            Member Models
          </h4>
          {models.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {models.map((model) => (
                <Badge key={model} variant="secondary">
                  {model}
                </Badge>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">No models in this group</p>
          )}
        </div>

        {/* Raw JSON */}
        <div className="border-t" />
        <div className="space-y-2">
          <button
            onClick={() => setShowRawJson(!showRawJson)}
            className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            {showRawJson ? (
              <ChevronUp className="h-4 w-4" />
            ) : (
              <ChevronDown className="h-4 w-4" />
            )}
            Raw JSON
          </button>
          {showRawJson && (
            <pre className="bg-muted p-3 rounded-md text-xs overflow-x-auto">
              {JSON.stringify(routeGroup, null, 2)}
            </pre>
          )}
        </div>
      </div>
    </SectionCard>
  )
}
