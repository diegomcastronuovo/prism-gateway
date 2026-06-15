'use client'

import { AuthGuard } from '@/lib/auth/guards'
import { AppShell } from '@/components/layout/app-shell'

export default function AppLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthGuard>
      <AppShell>{children}</AppShell>
    </AuthGuard>
  )
}
