'use client'

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard } from '@/components/shared/section-card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { ArrowLeft, Database, TrendingUp, Hash, CheckCircle, XCircle } from 'lucide-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'

type SemanticCachePrompt = {
  request_text: string
  hit_count: number
  last_hit_at: string
  expires_at: string
  model: string
  route_group: string
}

type SemanticCacheModel = {
  model: string
  entries: number
  total_hits: number
}

type SemanticCacheRouteGroup = {
  route_group: string
  entries: number
  total_hits: number
}

type SemanticCacheStats = {
  object: string
  summary: {
    total_entries: number
    total_hits: number
    avg_hits_per_entry: number
    active_entries: number
    expired_entries: number
  }
  top_prompts: SemanticCachePrompt[]
  top_models: SemanticCacheModel[]
  top_route_groups: SemanticCacheRouteGroup[]
  expiration: {
    active: number
    expired: number
  }
}

async function fetchSemanticCacheStats(limit?: number): Promise<SemanticCacheStats> {
  const qs = new URLSearchParams()
  if (limit) qs.set('limit', String(limit))
  
  const queryString = qs.toString()
  const url = queryString 
    ? `/api/observability/semantic-cache?${queryString}`
    : '/api/observability/semantic-cache'
  
  const resp = await fetch(url, { credentials: 'include', cache: 'no-store' })
  if (!resp.ok) {
    throw new Error(`Failed to fetch semantic cache stats: ${resp.status}`)
  }
  
  const data = await resp.json()
  return {
    object: data.object || 'semantic_cache_stats',
    summary: data.summary || {
      total_entries: 0,
      total_hits: 0,
      avg_hits_per_entry: 0,
      active_entries: 0,
      expired_entries: 0,
    },
    top_prompts: data.top_prompts || [],
    top_models: data.top_models || [],
    top_route_groups: data.top_route_groups || [],
    expiration: data.expiration || { active: 0, expired: 0 },
  }
}

export function SemanticCacheView({ onBack }: { onBack: () => void }) {
  const [limit] = useState(10)
  
  const { data, isLoading, error } = useQuery({
    queryKey: ['semantic-cache-stats', limit],
    queryFn: () => fetchSemanticCacheStats(limit),
  })

  const formatDateTime = (isoString: string) => {
    if (!isoString) return '—'
    try {
      return new Date(isoString).toLocaleString()
    } catch {
      return '—'
    }
  }

  return (
    <div>
      <PageHeader
        title="Semantic Cache"
        description="Cache usage, top prompts, and expiration health"
        action={
          <Button variant="outline" onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Observability
          </Button>
        }
      />

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-5 mb-6">
        {isLoading ? (
          <>
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
          </>
        ) : error ? (
          <div className="col-span-5">
            <SectionCard title="Error">
              <div className="text-sm text-destructive">
                Failed to load semantic cache analytics
              </div>
            </SectionCard>
          </div>
        ) : (
          <>
            <StatCard
              title="Total Entries"
              value={data?.summary.total_entries.toLocaleString() ?? '0'}
              icon={Database}
              description="Cached prompts"
            />
            <StatCard
              title="Total Hits"
              value={data?.summary.total_hits.toLocaleString() ?? '0'}
              icon={TrendingUp}
              description="Cache reuses"
            />
            <StatCard
              title="Avg Hits / Entry"
              value={data?.summary.avg_hits_per_entry.toFixed(2) ?? '0.00'}
              icon={Hash}
              description="Average reuse"
            />
            <StatCard
              title="Active Entries"
              value={data?.summary.active_entries.toLocaleString() ?? '0'}
              icon={CheckCircle}
              description="Not expired"
            />
            <StatCard
              title="Expired Entries"
              value={data?.summary.expired_entries.toLocaleString() ?? '0'}
              icon={XCircle}
              description="Past TTL"
            />
          </>
        )}
      </div>

      {/* Top Cached Prompts */}
      <div className="grid gap-6 mb-6">
        <SectionCard title="Top Cached Prompts">
          {isLoading ? (
            <Skeleton className="h-64" />
          ) : error ? (
            <div className="text-sm text-muted-foreground">Failed to load prompts</div>
          ) : data?.top_prompts.length === 0 ? (
            <div className="text-sm text-muted-foreground">No cached prompts found.</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Prompt</TableHead>
                  <TableHead className="text-right">Hits</TableHead>
                  <TableHead>Model</TableHead>
                  <TableHead>Route Group</TableHead>
                  <TableHead>Last Hit</TableHead>
                  <TableHead>Expires</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.top_prompts.map((prompt, idx) => (
                  <TableRow key={idx}>
                    <TableCell className="max-w-md truncate" title={prompt.request_text}>
                      {prompt.request_text}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{prompt.hit_count}</TableCell>
                    <TableCell>{prompt.model || '—'}</TableCell>
                    <TableCell>
                      {prompt.route_group ? (
                        <Badge variant="secondary">{prompt.route_group}</Badge>
                      ) : (
                        <span className="text-muted-foreground text-xs">unknown</span>
                      )}
                    </TableCell>
                    <TableCell className="text-xs">{formatDateTime(prompt.last_hit_at)}</TableCell>
                    <TableCell className="text-xs">{formatDateTime(prompt.expires_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </SectionCard>
      </div>

      {/* Top Models */}
      <div className="grid gap-6 md:grid-cols-2 mb-6">
        <SectionCard title="Top Models">
          {isLoading ? (
            <Skeleton className="h-48" />
          ) : error ? (
            <div className="text-sm text-muted-foreground">Failed to load models</div>
          ) : data?.top_models.length === 0 ? (
            <div className="text-sm text-muted-foreground">No cached models found.</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Model</TableHead>
                  <TableHead className="text-right">Entries</TableHead>
                  <TableHead className="text-right">Total Hits</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.top_models.map((model, idx) => (
                  <TableRow key={idx}>
                    <TableCell className="font-medium">{model.model}</TableCell>
                    <TableCell className="text-right tabular-nums">{model.entries}</TableCell>
                    <TableCell className="text-right tabular-nums">{model.total_hits}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </SectionCard>

        {/* Top Route Groups */}
        <SectionCard title="Top Route Groups">
          {isLoading ? (
            <Skeleton className="h-48" />
          ) : error ? (
            <div className="text-sm text-muted-foreground">Failed to load route groups</div>
          ) : data?.top_route_groups.length === 0 ? (
            <div className="text-sm text-muted-foreground">No cached route groups found.</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Route Group</TableHead>
                  <TableHead className="text-right">Entries</TableHead>
                  <TableHead className="text-right">Total Hits</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.top_route_groups.map((group, idx) => (
                  <TableRow key={idx}>
                    <TableCell className="font-medium">
                      {group.route_group || <span className="text-muted-foreground text-xs">unknown</span>}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{group.entries}</TableCell>
                    <TableCell className="text-right tabular-nums">{group.total_hits}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </SectionCard>
      </div>

      {/* Expiration Health */}
      <div className="grid gap-6 mb-6">
        <SectionCard title="Expiration Health">
          {isLoading ? (
            <Skeleton className="h-32" />
          ) : error ? (
            <div className="text-sm text-muted-foreground">Failed to load expiration data</div>
          ) : (
            <div className="grid grid-cols-2 gap-4">
              <div className="p-4 border rounded-lg">
                <div className="text-sm text-muted-foreground mb-1">Active</div>
                <div className="text-2xl font-bold">{data?.expiration.active.toLocaleString() ?? '0'}</div>
              </div>
              <div className="p-4 border rounded-lg">
                <div className="text-sm text-muted-foreground mb-1">Expired</div>
                <div className="text-2xl font-bold">{data?.expiration.expired.toLocaleString() ?? '0'}</div>
              </div>
            </div>
          )}
        </SectionCard>
      </div>
    </div>
  )
}
