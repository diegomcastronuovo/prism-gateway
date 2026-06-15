/**
 * Provider IDs that must always appear in model create/edit pickers, even when
 * GET /admin/providers does not list them yet (e.g. before first global-config save).
 */
const ALWAYS_OFFERED_PROVIDER_IDS = ['bedrock'] as const

/** Merges runtime catalog provider ids with always-offered ids, sorted. */
export function catalogProviderIdsForModels(runtimeProviders: { id: string }[] | undefined): string[] {
  const ids = new Set<string>(runtimeProviders?.map((p) => p.id) ?? [])
  for (const id of ALWAYS_OFFERED_PROVIDER_IDS) {
    ids.add(id)
  }
  return Array.from(ids).sort((a, b) => a.localeCompare(b))
}
