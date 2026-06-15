'use client'

import { Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { UserMenu } from './user-menu'
import { cn } from '@/lib/utils/cn'
import { WHITE_LABEL, BRAND_NAME } from '@/lib/config/branding'

interface TopbarProps {
  sidebarCollapsed?: boolean
}

export function Topbar({ sidebarCollapsed = false }: TopbarProps) {
  return (
    <header
      className={cn(
        'fixed top-0 z-30 h-16 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 transition-all',
        sidebarCollapsed ? 'left-16' : 'left-64',
        'right-0'
      )}
    >
      <div className="container mx-auto flex h-full items-center gap-6 px-4">
        {!WHITE_LABEL ? (
          <div className="flex items-center gap-1">
            <img src="/logo.svg" alt="Logo" className="h-16 w-auto mt-4" />
            <span className="text-4xl font-semibold tracking-wide">arkana</span>
          </div>
        ) : BRAND_NAME ? (
          <div className="flex items-center gap-1">
            <span className="text-2xl font-semibold tracking-wide">{BRAND_NAME}</span>
          </div>
        ) : null}

        <div className="ml-auto flex items-center">
          <UserMenu />
        </div>
      </div>
    </header>
  )
}
