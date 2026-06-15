import type {
  AdminModelOption,
  SemanticAnchor,
  SemanticAnchorListResponse,
  SemanticCalibrationResponse,
  SemanticRoute,
  SemanticRouteListResponse,
  SemanticSuggestionResponse,
  SemanticTestResponse,
} from '../types'

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const payload = await res.json().catch(() => ({}))
    const message = (payload as { error?: string; message?: string }).error || (payload as { message?: string }).message
    throw new Error(message || 'Request failed')
  }
  return res.json()
}

export async function listSemanticAnchors(params: {
  limit?: number
  offset?: number
  includeAnchorText?: boolean
  tenantId?: string | null
} = {}): Promise<SemanticAnchorListResponse> {
  const search = new URLSearchParams()
  if (params.limit != null) search.set('limit', String(params.limit))
  if (params.offset != null) search.set('offset', String(params.offset))
  if (params.includeAnchorText) search.set('include_anchor_text', 'true')
  if (params.tenantId) search.set('tenant_id', params.tenantId)
  const res = await fetch(`/api/semantic/anchors?${search.toString()}`, { cache: 'no-store' })
  const payload = await handleResponse<unknown>(res)
  if (Array.isArray(payload)) {
    return { data: payload as SemanticAnchor[] }
  }
  if (payload && typeof payload === 'object') {
    const maybeAnchors = (payload as { anchors?: SemanticAnchor[]; data?: SemanticAnchor[] }).anchors || (payload as { data?: SemanticAnchor[] }).data
    if (Array.isArray(maybeAnchors)) {
      const tenantId = (payload as { tenant_id?: string }).tenant_id
      return { data: maybeAnchors, pagination: (payload as SemanticAnchorListResponse).pagination, tenant_id: tenantId }
    }
  }
  return payload as SemanticAnchorListResponse
}

export async function createSemanticAnchor(payload: {
  name: string
  text?: string
  route_group: string
  modality?: string
  preferred_models?: string[]
  tenantId?: string | null
}): Promise<SemanticAnchor> {
  const { tenantId, ...bodyPayload } = payload
  const search = new URLSearchParams()
  if (tenantId) search.set('tenant_id', tenantId)
  const res = await fetch(`/api/semantic/anchors${search.toString() ? `?${search.toString()}` : ''}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(bodyPayload),
  })
  return handleResponse(res)
}

export async function patchSemanticAnchor(
  tenantId: string,
  name: string,
  payload: Record<string, unknown>
): Promise<SemanticAnchor> {
  const q = new URLSearchParams({ tenant_id: tenantId }).toString()
  const res = await fetch(`/api/semantic/anchors/${encodeURIComponent(name)}?${q}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return handleResponse(res)
}

export async function deleteSemanticAnchor(tenantId: string, name: string): Promise<void> {
  const q = new URLSearchParams({ tenant_id: tenantId }).toString()
  const res = await fetch(`/api/semantic/anchors/${encodeURIComponent(name)}?${q}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    const payload = await res.json().catch(() => ({}))
    const message = (payload as { error?: string }).error
    throw new Error(message || 'Failed to delete anchor')
  }
}

export async function runSemanticTest(params: {
  text: string
  tenantId?: string | null
}): Promise<SemanticTestResponse> {
  const res = await fetch('/api/semantic/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text: params.text, tenant_id: params.tenantId }),
  })
  return handleResponse(res)
}

export async function listAdminModels(): Promise<AdminModelOption[]> {
  const res = await fetch('/api/admin/models', { cache: 'no-store' })
  const payload = await handleResponse<{ data?: AdminModelOption[] } | AdminModelOption[]>(res)
  if (Array.isArray(payload)) {
    return payload
  }
  if (payload && Array.isArray(payload.data)) {
    return payload.data
  }
  return []
}

export async function suggestSemanticAnchors(payload: {
  dataset: string[]
  max_clusters?: number
  tenantId?: string | null
}): Promise<SemanticSuggestionResponse> {
  const res = await fetch('/api/semantic/suggest', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      dataset: payload.dataset,
      max_clusters: payload.max_clusters,
      tenant_id: payload.tenantId,
    }),
  })
  return handleResponse(res)
}

export async function calibrateSemanticThreshold(payload: {
  dataset: { text: string; route: string }[]
  tenantId: string
}): Promise<SemanticCalibrationResponse> {
  const res = await fetch('/api/semantic/calibrate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      dataset: payload.dataset,
      tenant_id: payload.tenantId,
    }),
  })
  return handleResponse(res)
}

export async function listSemanticRoutes(tenantId: string): Promise<SemanticRouteListResponse> {
  const q = new URLSearchParams({ tenant_id: tenantId }).toString()
  const res = await fetch(`/api/semantic/routes?${q}`, { cache: 'no-store' })
  const payload = await handleResponse<unknown>(res)
  if (Array.isArray(payload)) {
    return { routes: payload as SemanticRoute[] }
  }
  if (payload && typeof payload === 'object') {
    const p = payload as { routes?: SemanticRoute[]; data?: SemanticRoute[] }
    if (Array.isArray(p.routes)) return { routes: p.routes }
    if (Array.isArray(p.data)) return { routes: p.data }
  }
  return { routes: [] }
}

export async function createSemanticRoute(
  tenantId: string,
  payload: {
    name: string
    description?: string
    action: string
    utterances?: string[]
  }
): Promise<SemanticRoute> {
  const q = new URLSearchParams({ tenant_id: tenantId }).toString()
  const res = await fetch(`/api/semantic/routes?${q}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return handleResponse(res)
}

export async function deleteSemanticRoute(tenantId: string, name: string): Promise<void> {
  const q = new URLSearchParams({ tenant_id: tenantId }).toString()
  const res = await fetch(`/api/semantic/routes/${encodeURIComponent(name)}?${q}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    const payload = await res.json().catch(() => ({}))
    throw new Error((payload as { error?: string }).error || 'Failed to delete route')
  }
}

export async function getSemanticThreshold(tenantId: string): Promise<{ tenant_id: string; threshold_default: number | null }> {
  const res = await fetch(`/api/semantic/threshold?tenant_id=${encodeURIComponent(tenantId)}`, { cache: 'no-store' })
  return handleResponse(res)
}

export async function updateSemanticThreshold(payload: {
  tenant_id: string
  threshold_default: number
}): Promise<{ tenant_id: string; threshold_default: number }> {
  const res = await fetch('/api/semantic/threshold', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return handleResponse(res)
}
