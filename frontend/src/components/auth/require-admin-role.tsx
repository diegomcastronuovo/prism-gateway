'use client'

import { useAuth } from '@/hooks/use-auth'
import type { CanonicalRole } from '@/lib/auth/roles'
import { ShieldAlert } from 'lucide-react'

interface RequireAdminRoleProps {
  children: React.ReactNode
  /**
   * Roles that ARE allowed to see this content.
   * Defaults to ['admin'] — only full admins.
   */
  allowedRoles?: CanonicalRole[]
}

/**
 * Renders children when the signed-in user has one of the allowed roles.
 * Otherwise renders an "Insufficient Permissions" screen inline.
 * The sidebar menu item is NOT hidden — only the page content is gated.
 */
export function RequireAdminRole({
  children,
  allowedRoles = ['admin'],
}: RequireAdminRoleProps) {
  const { user, isLoading } = useAuth()

  if (isLoading) return null

  const role = user?.role ?? null
  if (role && allowedRoles.includes(role as CanonicalRole)) {
    return <>{children}</>
  }

  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] gap-4 text-center px-4">
      <ShieldAlert className="h-12 w-12 text-muted-foreground" />
      <h2 className="text-xl font-semibold">Insufficient Permissions</h2>
      <p className="text-muted-foreground max-w-sm">
        Your role does not have access to this section. Contact your administrator
        if you think this is a mistake.
      </p>
    </div>
  )
}
