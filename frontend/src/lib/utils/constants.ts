export const APP_NAME = 'PrismGateway'
export const APP_DESCRIPTION = 'AI Gateway Control Plane'

export const ROUTES = {
  HOME: '/',
  LOGIN: '/login',
  DASHBOARD: '/dashboard',
  TENANTS: '/tenants',
  MODELS: '/models',
  PROVIDERS: '/providers',
  ROUTE_GROUPS: '/route-groups',
  ROUTING: '/routing',
  SEMANTIC: '/semantic',
  TOOLS: '/tools',
  BENCHMARKS: '/benchmarks',
  BUDGETS: '/budgets',
  OBSERVABILITY: '/observability',
  REPLAY: '/routing/replay',
  GLOBAL_CONFIG: '/global-config',
  CONFIG_HISTORY: '/config-history',
  LOGS: '/logs',
  SETTINGS: '/settings',
  USAGE_DOC: '/usage-doc',
} as const

function normalizeEndpoint(raw?: string | null): string | undefined {
  if (!raw) return undefined
  const value = raw.trim().replace(/\/+$/, '')
  if (!value) return undefined
  if (/^\d+$/.test(value)) return `http://localhost:${value}`
  if (value.startsWith('localhost:') || value.startsWith('127.0.0.1:')) {
    return `http://${value}`
  }
  if (!/^https?:\/\//i.test(value)) return `http://${value}`
  return value
}

// Resolve API base URL at runtime per SPEC_63
// Priority:
// 1) localStorage.router_api_url (new key)
// 2) localStorage.gateway_api_endpoint (legacy key kept for backward-compat)
// 3) NEXT_PUBLIC_API_BASE_URL (if set)
// 4) NEXT_PUBLIC_API_URL (legacy env)
// 5) default http://localhost:8000
export function resolveApiBaseUrl(): string {
  // Browser-only: check localStorage
  if (typeof window !== 'undefined') {
    try {
      const lsUrl = normalizeEndpoint(
        window.localStorage.getItem('router_api_url') ||
        window.localStorage.getItem('gateway_api_endpoint')
      )
      if (lsUrl) {
        return lsUrl
      }
    } catch {
      // ignore storage access errors
    }
  }
  // Fallback to env vars
  return (
    normalizeEndpoint(process.env.NEXT_PUBLIC_API_BASE_URL) ||
    normalizeEndpoint(process.env.NEXT_PUBLIC_API_URL) ||
    'http://localhost:8000'
  )
}

export const API_BASE_URL = resolveApiBaseUrl()
