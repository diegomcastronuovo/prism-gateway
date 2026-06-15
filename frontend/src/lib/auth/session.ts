import { Session, User } from '@/types/auth'

const SESSION_KEY = 'ai_gateway_session'

/**
 * Returns the persisted session without invalidating it on access token expiry.
 * Callers (auth-context) must use this to attempt refresh when access is expired
 * but refresh_token may still be valid. Do not clear session here so refresh can run.
 */
export function getStoredSession(): Session | null {
  if (typeof window === 'undefined') return null

  try {
    const stored = localStorage.getItem(SESSION_KEY)
    if (!stored) return null

    const session = JSON.parse(stored) as Session
    return session
  } catch {
    return null
  }
}

export function setStoredSession(session: Session): void {
  if (typeof window === 'undefined') return
  localStorage.setItem(SESSION_KEY, JSON.stringify(session))
}

export function clearSession(): void {
  if (typeof window === 'undefined') return
  localStorage.removeItem(SESSION_KEY)
}

export function createMockSession(): Session {
  const mockUser: User = {
    id: 'mock-user-1',
    email: 'dev@aigateway.local',
    name: 'Development User',
    role: 'admin',
  }

  return {
    user: mockUser,
    accessToken: 'mock-access-token',
    expiresAt: Date.now() + 24 * 60 * 60 * 1000,
    isMock: true,
  }
}
