import { User, Session, AuthProvider } from '@/types/auth'

export interface AuthContextValue {
  session: Session | null
  user: User | null
  isAuthenticated: boolean
  isLoading: boolean
  /** True while access token refresh is in flight — global pages should show loading, not stale RBAC UI. */
  isRefreshingSession: boolean
  login: (provider: AuthProvider) => Promise<void>
  logout: () => Promise<void>
  refreshSession: () => Promise<void>
  ensureValidSession: () => Promise<boolean>
}

export interface AuthProviderProps {
  children: React.ReactNode
}
