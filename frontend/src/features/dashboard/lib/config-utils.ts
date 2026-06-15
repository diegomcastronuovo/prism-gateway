export function getNestedValue(
  obj: Record<string, unknown> | undefined,
  path: string
): unknown {
  if (!obj) return undefined
  const keys = path.split('.')
  let current: unknown = obj

  for (const key of keys) {
    if (current && typeof current === 'object' && key in current) {
      current = (current as Record<string, unknown>)[key]
    } else {
      return undefined
    }
  }

  return current
}

export function getFirstValue(
  obj: Record<string, unknown> | undefined,
  paths: string[]
): unknown {
  for (const path of paths) {
    const value = getNestedValue(obj, path)
    if (value !== undefined && value !== null) {
      return value
    }
  }
  return undefined
}

export function formatDurationMs(value: unknown): string | null | undefined {
  if (value === undefined) return undefined
  if (value === null) return null
  const ms = Number(value)
  if (!Number.isFinite(ms)) return undefined
  if (ms >= 1000) {
    const seconds = ms / 1000
    const secondsLabel = Number.isInteger(seconds) ? String(seconds) : seconds.toFixed(1)
    return `${ms} ms (${secondsLabel} s)`
  }
  return `${ms} ms`
}

export function formatBooleanLabel(value: unknown, yesLabel: string, noLabel: string): string | undefined {
  if (value === undefined || value === null) return undefined
  return value ? yesLabel : noLabel
}
