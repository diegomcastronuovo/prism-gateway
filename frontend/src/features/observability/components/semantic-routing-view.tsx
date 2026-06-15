'use client'

import { useQuery } from '@tanstack/react-query'
import { PageHeader } from '@/components/layout/page-header'
import { StatCard } from '@/components/shared/stat-card'
import { SectionCard } from '@/components/shared/section-card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { ArrowLeft, Activity, CheckCircle, Percent } from 'lucide-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

type SemanticRoutingRoute = {
  route_group: string
  matches: number
}

type SemanticRoutingAnchor = {
  anchor: string
  matches: number
}

type SemanticRoutingStats = {
  object: string
  coverage: {
    total_requests: number
    matched_requests: number
    coverage_rate: number
  }
  top_routes: SemanticRoutingRoute[]
  top_anchors: SemanticRoutingAnchor[]
}

async function fetchSemanticRoutingStats(): Promise<SemanticRoutingStats> {
  const resp = await fetch('/api/observability/semantic-routing', {
    credentials: 'include',
    cache: 'no-store',
  })
  if (!resp.ok) {
    throw new Error(`Failed to fetch semantic routing stats: ${resp.status}`)
  }
  
  const data = await resp.json()
  return {
    object: data.object || 'semantic_routing_stats',
    coverage: data.coverage || {
      total_requests: 0,
      matched_requests: 0,
      coverage_rate: 0,
    },
    top_routes: data.top_routes || [],
    top_anchors: data.top_anchors || [],
  }
}

export function SemanticRoutingView({ onBack }: { onBack: () => void }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['semantic-routing-stats'],
    queryFn: fetchSemanticRoutingStats,
  })

  const hasNoMatches = data && data.coverage.matched_requests === 0 && data.coverage.total_requests > 0

  return (
    <div>
      <PageHeader
        title="Semantic Routing"
        description="Coverage, anchors, and route groups"
        action={
          <Button variant="outline" onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Observability
          </Button>
        }
      />

      {/* Coverage Summary Cards */}
      <div className="grid gap-4 md:grid-cols-3 mb-6">
        {isLoading ? (
          <>
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
          </>
        ) : error ? (
          <div className="col-span-3">
            <SectionCard title="Error">
              <div className="text-sm text-destructive">
                Failed to load semantic routing analytics
              </div>
            </SectionCard>
          </div>
        ) : (
          <>
            <StatCard
              title="Total Requests"
              value={data?.coverage.total_requests.toLocaleString() ?? '0'}
              icon={Activity}
              description="All requests"
            />
            <StatCard
              title="Matched Requests"
              value={data?.coverage.matched_requests.toLocaleString() ?? '0'}
              icon={CheckCircle}
              description="Routed semantically"
            />
            <StatCard
              title="Coverage Rate"
              value={`${((data?.coverage.coverage_rate ?? 0) * 100).toFixed(1)}%`}
              icon={Percent}
              description="Match percentage"
            />
          </>
        )}
      </div>

      {/* No Matches Message */}
      {hasNoMatches && (
        <div className="mb-6">
          <SectionCard title="Status">
            <div className="text-sm text-muted-foreground">
              Semantic routing is not currently matching requests.
            </div>
          </SectionCard>
        </div>
      )}

      {/* Top Route Groups */}
      <div className="grid gap-6 md:grid-cols-2 mb-6">
        <SectionCard title="Top Route Groups">
          {isLoading ? (
            <Skeleton className="h-48" />
          ) : error ? (
            <div className="text-sm text-muted-foreground">Failed to load route groups</div>
          ) : data?.top_routes.length === 0 ? (
            <div className="text-sm text-muted-foreground">No route groups found.</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Route Group</TableHead>
                  <TableHead className="text-right">Matches</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.top_routes.map((route, idx) => (
                  <TableRow key={idx}>
                    <TableCell className="font-medium">
                      {route.route_group || <span className="text-muted-foreground text-xs">unknown</span>}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">{route.matches}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </SectionCard>

        {/* Top Anchors */}
        <SectionCard title="Top Anchors">
          {isLoading ? (
            <Skeleton className="h-48" />
          ) : error ? (
            <div className="text-sm text-muted-foreground">Failed to load anchors</div>
          ) : data?.top_anchors.length === 0 ? (
            <div className="text-sm text-muted-foreground">No anchors found.</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Anchor</TableHead>
                  <TableHead className="text-right">Matches</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.top_anchors.map((anchor, idx) => (
                  <TableRow key={idx}>
                    <TableCell className="font-medium">{anchor.anchor}</TableCell>
                    <TableCell className="text-right tabular-nums">{anchor.matches}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </SectionCard>
      </div>
    </div>
  )
}
