'use client'

import { Sidebar } from './sidebar'
import { Topbar } from './topbar'
import { useSidebarState } from '@/hooks/use-sidebar-state'
import { cn } from '@/lib/utils/cn'

interface AppShellProps {
  children: React.ReactNode
}

export function AppShell({ children }: AppShellProps) {
  const { isCollapsed } = useSidebarState()

  return (
    <div className="min-h-screen">
      <Sidebar isCollapsed={isCollapsed} />
      <Topbar sidebarCollapsed={isCollapsed} />
      
      <main
        className={cn(
          'pt-16 transition-all',
          isCollapsed ? 'pl-16' : 'pl-64'
        )}
      >
        <div className="container mx-auto p-6">
          {children}
        </div>
      </main>
    </div>
  )
}
