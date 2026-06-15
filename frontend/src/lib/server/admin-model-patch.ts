/**
 * PATCH /admin/models/:id merges the JSON body into the model map produced by
 * json.Marshal(config.ModelConfig). That uses Go's default field names:
 * Provider, Type, Pricing, Mock — not the catalog API's snake_case (provider, pricing, mock).
 *
 * Sending lowercase keys creates parallel entries and leaves Pricing/Mock unchanged.
 * @see bugs/bug_model_values.md
 */

export type CatalogLikeModelPatch = {
  id?: string
  provider?: string
  type?: string
  infrastructure_monthly_usd?: number
  execution?: {
    endpoint?: string
    protocol?: string
  }
  observable?: {
    fields?: Array<{
      path: string
      type: 'text' | 'json' | 'number'
      role: 'input' | 'output'
    }>
  }
  pricing?: Record<string, unknown>
  mock?: Record<string, unknown>
  route_groups?: string[]
  version?: number
  enabled?: boolean
  /** PascalCase variant — backend canonical form. Both are accepted. */
  Enabled?: boolean
  markup_percentage?: number
  /** Upstream native model id (e.g. Bedrock modelId); must match catalog id for provider bedrock. */
  provider_model_id?: string
  /** Catalog model name (same as id in API); Bedrock should match Model ID. */
  name?: string
  /** Per-model base URL override (e.g. local LLMs on different ports). */
  base_url?: string
  [key: string]: unknown
}

function isPresent(obj: Record<string, unknown>, snake: string, pascal: string): boolean {
  return Object.prototype.hasOwnProperty.call(obj, snake) || Object.prototype.hasOwnProperty.call(obj, pascal)
}

function numFrom(obj: Record<string, unknown>, snake: string, pascal: string): number | undefined {
  if (!isPresent(obj, snake, pascal)) return undefined
  const v = (obj[snake] ?? obj[pascal]) as unknown
  if (v === null || v === undefined || v === '') return undefined
  const n = typeof v === 'number' ? v : Number(v)
  return Number.isFinite(n) ? n : undefined
}

function intFrom(obj: Record<string, unknown>, snake: string, pascal: string): number | undefined {
  const n = numFrom(obj, snake, pascal)
  if (n === undefined) return undefined
  return Math.trunc(n)
}

/**
 * Builds the JSON body for PATCH /admin/models/:id so mergeMapsPublic updates the real fields.
 * Strips catalog-only / client-only keys (id, route_groups, version, …).
 */
export function buildAdminModelPatchPayload(input: CatalogLikeModelPatch): Record<string, unknown> {
  const out: Record<string, unknown> = {}

  // Enabled — accept both PascalCase (canonical) and lowercase (legacy)
  const enabledVal = input.Enabled ?? input.enabled
  if (enabledVal !== undefined) {
    out.Enabled = Boolean(enabledVal)
  }

  if (input.provider !== undefined) {
    out.Provider = input.provider
  }
  if (input.type !== undefined) {
    out.Type = input.type
  }
  if (input.infrastructure_monthly_usd !== undefined) {
    const value = Number(input.infrastructure_monthly_usd)
    if (Number.isFinite(value)) {
      out.infrastructure_monthly_usd = value
    }
  }

  if (input.markup_percentage !== undefined) {
    const value = Number(input.markup_percentage)
    if (Number.isFinite(value)) {
      out.markup_percentage = value
    }
  }

  if (input.provider_model_id !== undefined) {
    const v = typeof input.provider_model_id === 'string' ? input.provider_model_id.trim() : ''
    if (v) {
      out.provider_model_id = v
    }
  }

  if (input.name !== undefined) {
    const v = typeof input.name === 'string' ? input.name.trim() : ''
    if (v) {
      out.name = v
    }
  }

  if (input.base_url !== undefined) {
    const v = typeof input.base_url === 'string' ? input.base_url.trim() : ''
    // Send empty string explicitly so a PATCH can clear the value.
    out.base_url = v
  }

  if (input.execution && typeof input.execution === 'object') {
    const execution = input.execution as Record<string, unknown>
    const endpoint = execution.endpoint ?? execution.Endpoint
    const protocol = execution.protocol ?? execution.Protocol
    const payload: Record<string, unknown> = {}
    if (typeof endpoint === 'string' && endpoint.trim()) {
      payload.endpoint = endpoint.trim()
    }
    if (typeof protocol === 'string' && protocol.trim()) {
      payload.protocol = protocol.trim()
    }
    if (Object.keys(payload).length > 0) {
      out.execution = payload
    }
  }

  if (input.observable && typeof input.observable === 'object') {
    const observable = input.observable as Record<string, unknown>
    const fields = Array.isArray(observable.fields) ? observable.fields : []
    const mapped = fields
      .map((field) => ({
        path: String((field as Record<string, unknown>).path ?? '').trim(),
        type: String((field as Record<string, unknown>).type ?? ''),
        role: String((field as Record<string, unknown>).role ?? ''),
      }))
      .filter((field) => field.path.length > 0)
    out.observable = {
      fields: mapped,
    }
  }

  if (input.pricing && typeof input.pricing === 'object') {
    const p = input.pricing as Record<string, unknown>
    const pricing: Record<string, number> = {}
    const prompt = numFrom(p, 'prompt_per_1m', 'PromptPer1M')
    const completion = numFrom(p, 'completion_per_1m', 'CompletionPer1M')
    if (prompt !== undefined) pricing.PromptPer1M = prompt
    if (completion !== undefined) pricing.CompletionPer1M = completion
    if (Object.keys(pricing).length > 0) {
      out.Pricing = pricing
    }
  }

  if (input.mock && typeof input.mock === 'object') {
    const m = input.mock as Record<string, unknown>
    const mock: Record<string, unknown> = {}
    if (isPresent(m, 'enabled', 'Enabled')) {
      mock.Enabled = Boolean(m.enabled ?? m.Enabled)
    }
    if (isPresent(m, 'delay_min_ms', 'DelayMinMs')) {
      const v = intFrom(m, 'delay_min_ms', 'DelayMinMs')
      if (v !== undefined) mock.DelayMinMs = v
    }
    if (isPresent(m, 'delay_max_ms', 'DelayMaxMs')) {
      const v = intFrom(m, 'delay_max_ms', 'DelayMaxMs')
      if (v !== undefined) mock.DelayMaxMs = v
    }
    if (isPresent(m, 'error_rate', 'ErrorRate')) {
      const v = numFrom(m, 'error_rate', 'ErrorRate')
      if (v !== undefined) mock.ErrorRate = v
    }
    if (isPresent(m, 'error_status', 'ErrorStatus')) {
      const v = intFrom(m, 'error_status', 'ErrorStatus')
      if (v !== undefined) mock.ErrorStatus = v
    }
    if (isPresent(m, 'error_message', 'ErrorMessage')) {
      mock.ErrorMessage = String(m.error_message ?? m.ErrorMessage ?? '')
    }
    if (isPresent(m, 'fixed_response', 'FixedResponse')) {
      mock.FixedResponse = String(m.fixed_response ?? m.FixedResponse ?? '')
    }
    if (Object.keys(mock).length > 0) {
      out.Mock = mock
    }
  }

  return out
}
