'use client'

import { useEffect, useMemo, useState } from 'react'
import type { ElementType } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
  LayoutDashboard,
  Users,
  Layers,
  Boxes,
  GitBranch,
  Brain,
  Wrench,
  BarChart3,
  Wallet,
  Activity,
  PlayCircle,
  Settings,
  Info,
  History,
  ScrollText,
  Globe,
  ChevronDown,
  BookOpen,
} from 'lucide-react'
import { cn } from '@/lib/utils/cn'
import { ROUTES } from '@/lib/utils/constants'
import { useAuth } from '@/hooks/use-auth'
import type { CanonicalRole } from '@/lib/auth/roles'
import { AboutDialog } from './about-dialog'
import { ThemeToggle } from './theme-toggle'
import { ThemeAccentSwitcher } from '@/components/shared/theme-accent-switcher'
import { WHITE_LABEL } from '@/lib/config/branding'

interface NavigationItem {
  name: string
  href: string
  icon: ElementType
  requiresRole?: CanonicalRole[]
}

interface NavGroup {
  id: string
  label: string
  /** Emphasize this group (AI Routing). */
  prominent?: boolean
  items: NavigationItem[]
}

const navigationGroups: NavGroup[] = [
  {
    id: 'core',
    label: 'Configuration',
    items: [
      { name: 'Global Config', href: ROUTES.GLOBAL_CONFIG, icon: Globe, requiresRole: ['admin'] },
      { name: 'Tenants', href: ROUTES.TENANTS, icon: Users, requiresRole: ['admin', 'local_admin', 'user'] },
      { name: 'Models', href: ROUTES.MODELS, icon: Layers, requiresRole: ['admin'] },
      { name: 'Providers', href: ROUTES.PROVIDERS, icon: Boxes, requiresRole: ['admin'] },
      { name: 'Route Groups', href: ROUTES.ROUTE_GROUPS, icon: GitBranch, requiresRole: ['admin'] },
    ],
  },
  {
    id: 'ai-routing',
    label: 'Routing Intelligence',
    prominent: true,
    items: [
      { name: 'Routing', href: ROUTES.ROUTING, icon: GitBranch, requiresRole: ['admin'] },
      { name: 'Semantic', href: ROUTES.SEMANTIC, icon: Brain, requiresRole: ['admin'] },
      { name: 'Tool Routing', href: ROUTES.TOOLS, icon: Wrench, requiresRole: ['admin'] },
    ],
  },
  {
    id: 'monitoring',
    label: 'Monitoring',
    items: [
      { name: 'Observability', href: ROUTES.OBSERVABILITY, icon: Activity, requiresRole: ['admin', 'audit', 'finance'] },
      { name: 'Logs', href: ROUTES.LOGS, icon: ScrollText, requiresRole: ['admin', 'audit'] },
      { name: 'Replay', href: ROUTES.REPLAY, icon: PlayCircle, requiresRole: ['admin'] },
      { name: 'LLM Benchmarks', href: ROUTES.BENCHMARKS, icon: BarChart3, requiresRole: ['admin'] },
    ],
  },
  {
    id: 'governance',
    label: 'Governance',
    items: [
      { name: 'Budgets', href: ROUTES.BUDGETS, icon: Wallet, requiresRole: ['admin', 'finance'] },
    ],
  },
  {
    id: 'system',
    label: 'System',
    items: [
      { name: 'Config History', href: ROUTES.CONFIG_HISTORY, icon: History, requiresRole: ['admin'] },
      { name: 'Settings', href: ROUTES.SETTINGS, icon: Settings, requiresRole: ['admin'] },
      { name: 'Usage Doc', href: ROUTES.USAGE_DOC, icon: BookOpen, requiresRole: ['admin'] },
    ],
  },
]

const dashboardItem: NavigationItem = {
  name: 'Dashboard',
  href: ROUTES.DASHBOARD,
  icon: LayoutDashboard,
  requiresRole: ['admin', 'local_admin', 'user', 'audit', 'finance'],
}

function filterItems(items: NavigationItem[], role: string | null): NavigationItem[] {
  return items.filter((item) => {
    if (!item.requiresRole) return true
    if (!role) return false
    return item.requiresRole.includes(role as CanonicalRole)
  })
}

interface SidebarProps {
  isCollapsed?: boolean
}

export function Sidebar({ isCollapsed = false }: SidebarProps) {
  const pathname = usePathname()
  const [aboutOpen, setAboutOpen] = useState(false)
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({})
  const { user } = useAuth()
  const role = user?.role ?? null

  const visibleGroups = useMemo(() => {
    return navigationGroups
      .map((g) => ({
        ...g,
        items: filterItems(g.items, role),
      }))
      .filter((g) => g.items.length > 0)
  }, [role])

  const visibleDashboard = filterItems([dashboardItem], role).length > 0

  useEffect(() => {
    setOpenGroups((prev) => {
      const next = { ...prev }
      for (const group of visibleGroups) {
        const hasActive = group.items.some((item) => pathname === item.href)
        if (hasActive) {
          next[group.id] = true
        }
      }
      return next
    })
  }, [pathname, visibleGroups])

  const flatNavItems = useMemo(() => {
    return visibleGroups.flatMap((g) => g.items)
  }, [visibleGroups])

  const toggleGroup = (id: string) => {
    setOpenGroups((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const isGroupOpen = (id: string) => openGroups[id] === true

  const DashboardIcon = dashboardItem.icon

  return (
    <aside
      className={cn(
        'fixed left-0 top-0 z-40 flex h-screen flex-col border-r bg-card transition-all',
        isCollapsed ? 'w-16' : 'w-64'
      )}
    >
      {!WHITE_LABEL && (
        <div className="flex flex-col items-center justify-center border-b pb-4 pt-4">
          <Link href={ROUTES.DASHBOARD} className="text-lg font-bold tracking-tight text-foreground">
            PrismGateway
          </Link>
        </div>
      )}

      <nav className="flex min-h-0 flex-1 flex-col space-y-1 overflow-y-auto p-4">
        {isCollapsed ? (
          <>
            {visibleDashboard && (
              <Link
                href={dashboardItem.href}
                className={cn(
                  'flex items-center justify-center rounded-lg p-2 text-sm font-medium transition-colors',
                  pathname === dashboardItem.href
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                )}
                title={dashboardItem.name}
              >
                <DashboardIcon className="h-5 w-5 flex-shrink-0" />
              </Link>
            )}
            {flatNavItems.map((item) => {
              const Icon = item.icon
              const isActive = pathname === item.href
              return (
                <Link
                  key={`${item.href}-${item.name}`}
                  href={item.href}
                  className={cn(
                    'flex items-center justify-center rounded-lg p-2 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )}
                  title={item.name}
                >
                  <Icon className="h-5 w-5 flex-shrink-0" />
                </Link>
              )
            })}
            <button
              type="button"
              onClick={() => setAboutOpen(true)}
              className={cn(
                'flex items-center justify-center rounded-lg p-2 text-sm font-medium transition-colors',
                'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
              )}
              title="About"
            >
              <Info className="h-5 w-5 flex-shrink-0" />
            </button>
          </>
        ) : (
          <>
            {visibleDashboard && (
              <div className="mb-2">
                <Link
                  href={dashboardItem.href}
                  className={cn(
                    'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                    pathname === dashboardItem.href
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )}
                >
                  <DashboardIcon className="h-5 w-5 flex-shrink-0" />
                  <span>{dashboardItem.name}</span>
                </Link>
              </div>
            )}

            {visibleGroups.map((group) => {
              const open = isGroupOpen(group.id)
              const hasActiveChild = group.items.some((item) => pathname === item.href)
              const isProminent = group.prominent

              return (
                <div
                  key={group.id}
                  className={cn(
                    'mb-1 space-y-0.5',
                    isProminent &&
                      hasActiveChild &&
                      'rounded-lg border border-primary/25 bg-primary/[0.06] p-1.5'
                  )}
                >
                  <button
                    type="button"
                    onClick={() => toggleGroup(group.id)}
                    className={cn(
                      'flex w-full items-center justify-between gap-2 rounded-md px-2 py-2 text-left text-xs font-semibold uppercase tracking-wide text-muted-foreground',
                      'hover:bg-accent/60 hover:text-accent-foreground',
                      hasActiveChild && 'text-foreground'
                    )}
                    aria-expanded={open}
                  >
                    <span>{group.label}</span>
                    <ChevronDown
                      className={cn('h-4 w-4 shrink-0 transition-transform', open ? 'rotate-180' : 'rotate-0')}
                      aria-hidden
                    />
                  </button>
                  {open && (
                    <div className="space-y-0.5 pb-2 pl-1">
                      {group.items.map((item) => {
                        const isActive = pathname === item.href
                        const Icon = item.icon
                        return (
                          <Link
                            key={`${group.id}-${item.name}`}
                            href={item.href}
                            className={cn(
                              'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                              isActive
                                ? 'bg-primary text-primary-foreground'
                                : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                            )}
                          >
                            <Icon className="h-5 w-5 flex-shrink-0" />
                            <span>{item.name}</span>
                          </Link>
                        )
                      })}
                      {group.id === 'system' && (
                        <button
                          type="button"
                          onClick={() => setAboutOpen(true)}
                          className={cn(
                            'flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                            'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                          )}
                        >
                          <Info className="h-5 w-5 flex-shrink-0" />
                          <span>About</span>
                        </button>
                      )}
                    </div>
                  )}
                </div>
              )
            })}
          </>
        )}
      </nav>

      <div
        className={cn(
          'border-t p-4',
          isCollapsed
            ? 'flex flex-col items-center gap-3'
            : 'flex items-center justify-between gap-3'
        )}
      >
        <ThemeAccentSwitcher />
        <ThemeToggle />
      </div>

      <AboutDialog open={aboutOpen} onOpenChange={setAboutOpen} />
    </aside>
  )
}
