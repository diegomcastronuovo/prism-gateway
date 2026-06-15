export interface User {
  id: string
  email: string
  name: string
  avatar?: string
  role: 'admin' | 'audit' | 'local_admin' | 'finance' | 'user'
}

export interface Session {
  user: User
  accessToken: string
  idToken?: string
  refreshToken?: string
  expiresAt: number
  refreshExpiresAt?: number
  /** True for local dev mock sessions — skips real backend auth checks. Remove in prod. */
  isMock?: boolean
}

export type AuthProvider = 'keycloak' | 'cognito' | 'mock'

export interface AuthState {
  session: Session | null
  isAuthenticated: boolean
  isLoading: boolean
}
