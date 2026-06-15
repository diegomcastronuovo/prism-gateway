import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@/hooks/use-auth'

/** TanStack Query v5: queryFn must not resolve to `undefined` or the query errors with "data is undefined". */
export type GlobalAccessProbeOk = { ok: true }

/**
 * Lightweight probe: same middleware as snapshot/replay (no tenant_id in path → 403 for local_admin).
 * Global admins get 404 if the request id has no snapshot — still "allowed" to use the page.
 */
export async function fetchReplayGlobalAccess(): Promise<GlobalAccessProbeOk> {
  const res = await fetch('/api/routing/requests/__replay_access__/routing', {
    credentials: 'include',
    cache: 'no-store',
  })
  if (res.status === 401 || res.status === 403) {
    const body = await res.json().catch(() => ({} as { error?: unknown }))
    const raw = body?.error
    const msg =
      typeof raw === 'string'
        ? raw
        : raw && typeof raw === 'object' && 'message' in raw
          ? String((raw as { message?: string }).message)
          : 'Access denied'
    const err = new Error(msg) as Error & { status?: number }
    err.status = res.status
    throw err
  }
  if (!res.ok && res.status !== 404) {
    const body = await res.json().catch(() => ({} as { error?: unknown }))
    const raw = body?.error
    const msg =
      typeof raw === 'string'
        ? raw
        : raw && typeof raw === 'object' && 'message' in raw
          ? String((raw as { message?: string }).message)
          : `Request failed (${res.status})`
    const err = new Error(msg) as Error & { status?: number }
    err.status = res.status
    throw err
  }
  return { ok: true }
}

/**
 * Access probe: keyed by session token + expiry so a new token after refresh refetches;
 * disabled while refresh is in flight to avoid stale success vs new cookie.
 */
export function useReplayGlobalAccess() {
  const { user, session, isRefreshingSession } = useAuth()
  const sessionKey = session?.accessToken
    ? `${session.accessToken.slice(0, 24)}:${session.expiresAt}`
    : 'none'

  const isMock = Boolean(session?.isMock)
  const enabled = Boolean(user && session && !isRefreshingSession && !isMock)

  return useQuery({
    queryKey: ['replayGlobalAccess', sessionKey],
    queryFn: fetchReplayGlobalAccess,
    enabled,
    // Mock sessions bypass the real backend probe — always report access granted.
    placeholderData: isMock ? { ok: true as const } : undefined,
    gcTime: 0,
    staleTime: 0,
    retry: (failureCount, error) => {
      const s = (error as Error & { status?: number }).status
      if (s === 401 || s === 403) return false
      return failureCount < 2
    },
  })
}
