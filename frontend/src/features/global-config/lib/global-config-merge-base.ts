import type { QueryClient } from '@tanstack/react-query'
import type { GlobalConfig } from '@/features/global-config/api/use-global-config'

/**
 * PATCH /admin/config/global merges at the top level; `models` as an array replaces the whole array.
 * Always build the next payload from the latest global config in React Query so we never send a stale
 * subset (e.g. only one model) after other updates.
 *
 * @see bugs/bug_fe_models_editor_overwrites_models_array.md
 */
export function getLatestGlobalConfigRecord(
  queryClient: QueryClient,
  fallback: Record<string, unknown>
): Record<string, unknown> {
  const cached = queryClient.getQueryData<GlobalConfig>(['globalConfig'])
  const cfg = cached?.config
  if (cfg && typeof cfg === 'object' && !Array.isArray(cfg)) {
    return { ...(cfg as Record<string, unknown>) }
  }
  return { ...fallback }
}
