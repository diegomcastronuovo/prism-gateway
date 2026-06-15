'use client'

import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard } from '@/components/shared/section-card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { ArrowLeft, Layers, Activity, TrendingUp, Percent } from 'lucide-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

type SemanticCorrelationRouteGroup = {
  route_group: string
  cache_hits: number
  total_requests: number
  hit_rate: number
}

type SemanticCorrelationResponse = {
  object: string
  by_route_group: SemanticCorrelationRouteGroup[]
}

async function fetchSemanticCorrelation(): Promise<SemanticCorrelationResponse> {
  const resp = await fetch('/api/observability/semantic-correlation', {
    credentials: 'include',
    cache: 'no-store',
  })
  if (!resp.ok) {
    throw new Error(`Failed to fetch semantic correlation: ${resp.status}`)
  }
  
  const data = await resp.json()
  return {
    object: data.object || 'semantic_correlation',
    by_route_group: data.by_route_group || [],
  }
}

export function SemanticCorrelationView({ onBack }: { onBack: () => void }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['semantic-correlation'],
    queryFn: fetchSemanticCorrelation,
  })

  const summary = useMemo(() => {
    if (!data || data.by_route_group.length === 0) {
      return {
        totalRouteGroups: 0,
        totalRequests: 0,
        totalCacheHits: 0,
        avgHitRate: 0,
      }
    }

    const totalRequests = data.by_route_group.reduce((sum, g) => sum + g.total_requests, 0)
    const totalCacheHits = data.by_route_group.reduce((sum, g) => sum + g.cache_hits, 0)
    const avgHitRate = totalRequests > 0 ? totalCacheHits / totalRequests : 0

    return {
      totalRouteGroups: data.by_route_group.length,
      totalRequests,
      totalCacheHits,
      avgHitRate,
    }
  }, [data])

  return (
    <div>
      <PageHeader
        title="Semantic Correlation"
        description="Cache hits vs routing traffic by route group"
        action={
          <Button variant="outline" onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Observability
          </Button>
        }
      />

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-6">
        {isLoading ? (
          <>
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
          </>
        ) : error ? (
          <div className="col-span-4">
            <SectionCard title="Error">
              <div className="text-sm text-destructive">
                Failed to load semantic correlation analytics
              </div>
            </SectionCard>
          </div>
        ) : (
          <>
            <StatCard
              title="Total Route Groups"
              value={summary.totalRouteGroups.toLocaleString()}
              icon={Layers}
              description="Distinct groups"
            />
            <StatCard
              title="Total Requests"
              value={summary.totalRequests.toLocaleString()}
              icon={Activity}
              description="All traffic"
            />
            <StatCard
              title="Total Cache Hits"
              value={summary.totalCacheHits.toLocaleString()}
              icon={TrendingUp}
              description="Cache reuses"
            />
            <StatCard
              title="Avg Hit Rate"
              value={`${(summary.avgHitRate * 100).toFixed(1)}%`}
              icon={Percent}
              description="Overall efficiency"
            />
          </>
        )}
      </div>

      {/* Correlation by Route Group */}
      <SectionCard title="Correlation by Route Group">
        {isLoading ? (
          <Skeleton className="h-64" />
        ) : error ? (
          <div className="text-sm text-muted-foreground">Failed to load route group data</div>
        ) : !data || data.by_route_group.length === 0 ? (
          <div className="text-sm text-muted-foreground">No route group correlation data available.</div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Route Group</TableHead>
                <TableHead className="text-right">Total Requests</TableHead>
                <TableHead className="text-right">Cache Hits</TableHead>
                <TableHead className="text-right">Hit Rate</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.by_route_group.map((group, idx) => (
                <TableRow key={idx}>
                  <TableCell className="font-medium">
                    {group.route_group || <span className="text-muted-foreground text-xs">unknown</span>}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{group.total_requests.toLocaleString()}</TableCell>
                  <TableCell className="text-right tabular-nums">{group.cache_hits.toLocaleString()}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-2">
                      <span className="tabular-nums">{(group.hit_rate * 100).toFixed(1)}%</span>
                      <div className="w-16 h-2 rounded-full bg-muted">
                        <div
                          className="h-2 rounded-full bg-green-600"
                          style={{ width: `${Math.min(group.hit_rate * 100, 100)}%` }}
                        />
                      </div>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </SectionCard>
    </div>
  )
}
