export interface RequestLogDetail {
  id: string
  request_id: string
  timestamp: string
  tenant_id: string
  model: string
  provider: string
  strategy: string
  latency_ms: number
  status: string
  fallback_used: boolean
  attempt?: number
  cache_status?: string
  raw_request?: Record<string, unknown>
  // SPEC_66: Additional diagnostic fields
  decision_reason?: string
  error_type?: string
  metadata?: Record<string, unknown>
  pii_webhook_request_decision?: string
  pii_webhook_response_decision?: string
  routing_snapshot?: Record<string, unknown>
  decision_snapshot?: Record<string, unknown>
  // SPEC_143: workflow/conversation context
  workflow_id?: string
  conversation_id?: string
}

export interface RequestExplorerPagination {
  limit: number
  offset: number
  returned: number
  total: number
}

export interface RequestExplorerData {
  requests: RequestLogDetail[]
  total: number
  pagination?: RequestExplorerPagination
}

export interface RequestRoutingSnapshot {
  request_id: string
  tenant_id: string
  routing_snapshot: {
    provider?: string
    timestamp?: string
    selected_model?: string
    candidate_models?: string[]
    routing_strategy?: string
    fallback_attempts?: number
    estimated_costs_usd?: Record<string, number>
    cost_optimizer_applied?: boolean
  }
}

export interface ReplayRoutingSnapshot {
  provider?: string
  timestamp?: string
  selected_model?: string
  candidate_models?: string[]
  routing_strategy?: string
  fallback_attempts?: number
  estimated_costs_usd?: Record<string, number>
  cost_optimizer_applied?: boolean
}

export interface RequestReplayResult {
  mode?: string
  request_id: string
  tenant_id?: string
  selected_model?: string
  provider?: string
  routing_snapshot?: ReplayRoutingSnapshot
  // SPEC_68: Replay diagnostic fields
  decision_reason?: string
  decision_snapshot?: Record<string, unknown>
  routing_snapshot_full?: Record<string, unknown>
}

export interface RequestExplorerFilters {
  tenant_id?: string
  model?: string
  provider?: string
  status?: string
  fallback_used?: boolean
  time_range?: string
  limit?: number
  offset?: number
  // SPEC_143: workflow/conversation drill-down
  workflow_id?: string
  conversation_id?: string
}

export type RequestExplorerSortField = 'timestamp' | 'tenant_id' | 'model' | 'provider' | 'latency_ms'
export type RequestExplorerSortDirection = 'asc' | 'desc'
