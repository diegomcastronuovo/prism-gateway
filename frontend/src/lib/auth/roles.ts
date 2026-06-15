export type CanonicalRole = 'admin' | 'audit' | 'local_admin' | 'finance' | 'user'

export function extractRolesFromClaims(claims: any | null | undefined): string[] | undefined {
  if (!claims || typeof claims !== 'object') return undefined
  // Preferred: direct roles array on claims
  if (Array.isArray((claims as any).roles)) return (claims as any).roles as string[]
  // Keycloak standard: realm_access.roles
  const realmRoles = (claims as any).realm_access?.roles
  if (Array.isArray(realmRoles)) return realmRoles as string[]
  // Optional: resource_access[client].roles (not used for global role label)
  return undefined
}

export function deriveCanonicalRole(roles?: string[] | null): CanonicalRole | null {
  if (!roles || roles.length === 0) return null
  const set = new Set(roles.map((r) => r.toLowerCase()))
  // Priority mapping per spec
  if (set.has('admin') || set.has('platform_admin')) return 'admin'
  if (set.has('audit')) return 'audit'
  if (set.has('local_admin')) return 'local_admin'
  if (set.has('finance') || set.has('financial')) return 'finance'
  if (set.has('user')) return 'user'
  return null
}

export function getDisplayRole(role?: CanonicalRole | null): string | null {
  switch (role) {
    case 'admin':
      return 'Admin'
    case 'audit':
      return 'Audit'
    case 'local_admin':
      return 'Local Admin'
    case 'finance':
      return 'Finance'
    case 'user':
      return 'User'
    default:
      return null
  }
}
