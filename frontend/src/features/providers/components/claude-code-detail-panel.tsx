'use client'

import { useState } from 'react'
import { SectionCard } from '@/components/shared/section-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ChevronDown, ChevronUp, Code2, ShieldOff } from 'lucide-react'

export type ClaudeCodeProviderData = {
  id: string
  enabled: boolean
  has_api_key: boolean
  api_key_source: string
  base_url: string
  type: string
  status: string
  licensed: boolean
  feature: string
}

export type ClaudeCodeState =
  | { status: 'loading' }
  | { status: 'licensed'; data: ClaudeCodeProviderData }
  | { status: 'not_licensed' }
  | { status: 'unavailable'; error?: string }

interface ClaudeCodeDetailPanelProps {
  state: ClaudeCodeState
}

export function ClaudeCodeDetailPanel({ state }: ClaudeCodeDetailPanelProps) {
  const [showRawJson, setShowRawJson] = useState(false)

  if (state.status === 'loading') {
    return (
      <SectionCard title="Provider Details" className="border-t-4 border-t-purple-500">
        <div className="space-y-4">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      </SectionCard>
    )
  }

  if (state.status === 'not_licensed') {
    return (
      <SectionCard title="Claude Code" className="border-t-4 border-t-purple-500">
        <div className="space-y-4">
          <div className="flex items-center gap-2 flex-wrap">
            <h3 className="text-lg font-semibold">Claude Code</h3>
            <Badge variant="destructive" className="flex items-center gap-1">
              <ShieldOff className="h-3 w-3" />
              Not Licensed
            </Badge>
          </div>
          <div className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-950/20 dark:text-amber-400">
            Claude Code feature not licensed
          </div>
        </div>
      </SectionCard>
    )
  }

  if (state.status === 'unavailable') {
    return (
      <SectionCard title="Claude Code" className="border-t-4 border-t-purple-500">
        <div className="space-y-4">
          <div className="flex items-center gap-2 flex-wrap">
            <h3 className="text-lg font-semibold">Claude Code</h3>
            <Badge variant="secondary">Unavailable</Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            {state.error ?? 'Could not load Claude Code provider status.'}
          </p>
        </div>
      </SectionCard>
    )
  }

  const { data } = state

  return (
    <SectionCard title="Claude Code Details" className="border-t-4 border-t-purple-500">
      <div className="space-y-6">
        {/* Header */}
        <div className="space-y-2">
          <div className="flex items-center gap-2 flex-wrap">
            <h3 className="text-lg font-semibold">Claude Code</h3>
            <Badge variant="secondary">{data.type}</Badge>
            <Badge variant="default" className="flex items-center gap-1">
              <Code2 className="h-3 w-3" />
              {data.status === 'ready' ? 'Ready' : data.status}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">Licensed coding provider</p>
        </div>

        <div className="border-t" />

        {/* Fields */}
        <div className="grid gap-2 text-sm">
          <div className="flex justify-between">
            <span className="text-muted-foreground">Enabled</span>
            <Badge variant={data.enabled ? 'default' : 'secondary'}>
              {data.enabled ? 'Yes' : 'No'}
            </Badge>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Licensed</span>
            <Badge variant={data.licensed ? 'default' : 'destructive'}>
              {data.licensed ? 'Yes' : 'No'}
            </Badge>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Has API Key</span>
            <span>{data.has_api_key ? 'Yes' : 'No'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">API Key Source</span>
            <span>{data.api_key_source || '—'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Base URL</span>
            <span className="font-mono text-xs">{data.base_url || '—'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Feature</span>
            <span>{data.feature}</span>
          </div>
        </div>

        <div className="border-t" />

        {/* Raw JSON */}
        <div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setShowRawJson(!showRawJson)}
            className="mb-2"
          >
            {showRawJson ? (
              <>
                <ChevronUp className="h-4 w-4 mr-2" />
                Hide Raw JSON
              </>
            ) : (
              <>
                <ChevronDown className="h-4 w-4 mr-2" />
                Show Raw JSON
              </>
            )}
          </Button>
          {showRawJson && (
            <pre className="bg-muted p-4 rounded-lg overflow-auto text-xs max-h-96">
              {JSON.stringify(data, null, 2)}
            </pre>
          )}
        </div>
      </div>
    </SectionCard>
  )
}
