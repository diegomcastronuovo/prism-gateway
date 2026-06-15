/**
 * Global config / admin API expect pricing keys as snake_case (`prompt_per_1m`, `completion_per_1m`).
 * @see bugs/bug_fe_models_pricing_not_persisting.md
 */

export type ModelPricingSnake = {
  prompt_per_1m: number
  completion_per_1m: number
}

/** Read pricing from mixed legacy shapes (PascalCase, camelCase, snake_case). */
export function readPricingFields(pricing: unknown): ModelPricingSnake {
  const p = pricing && typeof pricing === 'object' ? (pricing as Record<string, unknown>) : {}
  const prompt =
    num(p.prompt_per_1m) ??
    num(p.PromptPer1M) ??
    num((p as { promptPer1M?: unknown }).promptPer1M)
  const completion =
    num(p.completion_per_1m) ??
    num(p.CompletionPer1M) ??
    num((p as { completionPer1M?: unknown }).completionPer1M)
  return {
    prompt_per_1m: prompt ?? 0,
    completion_per_1m: completion ?? 0,
  }
}

function num(v: unknown): number | undefined {
  if (typeof v === 'number' && !Number.isNaN(v)) return v
  if (typeof v === 'string' && v.trim() !== '') {
    const n = parseFloat(v)
    return Number.isNaN(n) ? undefined : n
  }
  return undefined
}
