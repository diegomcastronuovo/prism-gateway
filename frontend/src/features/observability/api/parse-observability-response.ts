/** Attach HTTP status to thrown Error for React Query retry / UI (401/403). */
export async function parseObservabilityErrorResponse(
  res: Response,
  fallbackMessage: string
): Promise<never> {
  const errorData = await res.json().catch(() => ({} as { error?: unknown }))
  const raw = errorData?.error
  const msg =
    typeof raw === 'string'
      ? raw
      : raw && typeof raw === 'object' && 'message' in raw
        ? String((raw as { message?: string }).message)
        : fallbackMessage
  const e = new Error(msg) as Error & { status?: number }
  e.status = res.status
  throw e
}
