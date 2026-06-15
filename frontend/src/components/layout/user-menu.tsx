'use client'

import { LogOut, User } from 'lucide-react'
import { useAuth } from '@/hooks/use-auth'
import { Button } from '@/components/ui/button'
import { getDisplayRole } from '@/lib/auth/roles'

export function UserMenu() {
  const { user, logout } = useAuth()

  if (!user) return null
  const roleLabel = getDisplayRole(user.role)

  return (
    <div className="flex items-center gap-3">
      <div className="hidden md:block text-right">
        <p className="text-sm font-medium">{user.name}</p>
        {roleLabel ? (
          <p className="text-xs text-muted-foreground">{roleLabel}</p>
        ) : (
          <p className="text-xs text-muted-foreground">{user.email}</p>
        )}
      </div>
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="icon" title="Profile">
          <User className="h-5 w-5" />
        </Button>
        <Button variant="ghost" size="icon" onClick={logout} title="Logout">
          <LogOut className="h-5 w-5" />
        </Button>
      </div>
    </div>
  )
}
