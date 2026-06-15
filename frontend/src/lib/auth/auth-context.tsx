'use client'

import { createContext, useContext, useState, useEffect, useCallback, useRef } from 'react'
import { useRouter } from 'next/navigation'
import { useQueryClient } from '@tanstack/react-query'
import { AuthContextValue, AuthProviderProps } from './types'
import { Session, type AuthProvider } from '@/types/auth'
import { getStoredSession, setStoredSession, clearSession, createMockSession } from './session'
import { ROUTES } from '@/lib/utils/constants'
import { generatePkce, buildAuthUrl, randomString } from './oidc'

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function AuthProvider({ children }: AuthProviderProps) {
  const [session, setSession] = useState<Session | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isRefreshingSession, setIsRefreshingSession] = useState(false)
  const router = useRouter()
  const queryClient = useQueryClient()
  const refreshPromiseRef = useRef<Promise<boolean> | null>(null)

  const clearGlobalConfigQueries = useCallback(() => {
    queryClient.removeQueries({ queryKey: ['globalConfig'] })
    queryClient.removeQueries({ queryKey: ['globalConfigChanges'] })
  }, [queryClient])

  /** Invalidate RBAC access probes so they refetch with the new Bearer/cookie after refresh. */
  const invalidateGlobalAccessQueries = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['replayGlobalAccess'] })
    queryClient.invalidateQueries({ queryKey: ['observabilityGlobalAccess'] })
  }, [queryClient])

  // SPEC_83 + SPEC_84: Session restoration on app init with refresh support
  useEffect(() => {
    const restoreSession = async () => {
      try {
        console.log('[AuthProvider] SESSION_RESTORE_START')
        // First try localStorage
        const storedSession = getStoredSession()
        if (storedSession) {
          const now = Date.now()
          console.log('[AuthProvider] SESSION_FOUND has_token=' + !!storedSession.accessToken + ' has_refresh=' + !!storedSession.refreshToken + ' expires_at=' + storedSession.expiresAt + ' now=' + now)

          // Case A: Access token still valid
          if (storedSession.expiresAt && storedSession.expiresAt > now) {
            console.log('[AuthProvider] SESSION_VALID token_expires_in_ms=' + (storedSession.expiresAt - now))
            setSession(storedSession)
            setIsLoading(false)
            return
          }
          
          // Case B: Access token expired but refresh token might be valid
          if (storedSession.refreshToken) {
            console.log('[Session Restore] ACCESS_TOKEN_EXPIRED checking_refresh_token')
            // Check if refresh token is still valid
            if (!storedSession.refreshExpiresAt || storedSession.refreshExpiresAt > now) {
              console.log('[Session Restore] ATTEMPTING_REFRESH_ON_INIT')
              // Attempt refresh
              try {
                const refreshRes = await fetch('/api/auth/refresh', {
                  method: 'POST',
                  headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify({
                    refresh_token: storedSession.refreshToken,
                  }),
                })
                
                const refreshData = await refreshRes.json()
                console.log('[Session Restore] REFRESH_RESPONSE status=' + refreshRes.status + ' ok=' + refreshData.ok + ' has_token=' + !!refreshData.access_token)
                if (refreshRes.ok && refreshData.ok && refreshData.access_token) {
                  console.log('[Session Restore] REFRESH_SUCCESS')
                  // Refresh succeeded, update session
                  const expiresIn = refreshData.expires_in || 3600
                  const refreshExpiresIn = refreshData.refresh_expires_in
                  const refreshedSession: Session = {
                    ...storedSession,
                    accessToken: refreshData.access_token,
                    refreshToken: refreshData.refresh_token || storedSession.refreshToken,
                    expiresAt: Date.now() + expiresIn * 1000,
                    refreshExpiresAt: refreshExpiresIn ? Date.now() + refreshExpiresIn * 1000 : storedSession.refreshExpiresAt,
                  }
                  setStoredSession(refreshedSession)
                  setSession(refreshedSession)
                  invalidateGlobalAccessQueries()
                  setIsLoading(false)
                  return
                } else {
                  console.error('[Session Restore] REFRESH_FAILED status=' + refreshRes.status + ' ok=' + refreshData.ok + ' error=' + (refreshData.error || 'UNKNOWN'))
                  clearSession()
                  setSession(null)
                }
              } catch (err) {
                console.error('[Session Restore] REFRESH_ERROR ' + (err instanceof Error ? err.message : String(err)))
                clearSession()
                setSession(null)
              }
            } else {
              if (process.env.NODE_ENV !== 'production') {
                console.log('[Session Restore] ❌ Refresh token also expired')
              }
              clearSession()
              setSession(null)
            }
          } else {
            clearSession()
            setSession(null)
          }
        }

        // Case C: No session found
        // During login: the callback will save to localStorage, then page redirects - localStorage persists across reloads
        // After reload: localStorage should still have tokens
        // Cookies are used automatically by browser in API requests, but not needed for session restoration
        console.log('[AuthProvider] SESSION_RESTORE_NO_SESSION')
        clearSession()
        setSession(null)
      } catch (error) {
        console.error('[AuthProvider] SESSION_RESTORE_ERROR ' + (error instanceof Error ? error.message : String(error)))
        clearSession()
        setSession(null)
      } finally {
        setIsLoading(false)
      }
    }

    restoreSession()
  }, [invalidateGlobalAccessQueries])

  const login = useCallback(async (provider: AuthProvider) => {
    setIsLoading(true)
    
    try {
      if (provider === 'mock') {
        const mockSession = createMockSession()
        setStoredSession(mockSession)
        setSession(mockSession)
        // Set server-side cookie so API routes bypass Bearer auth and use GATEWAY_ADMIN_API_KEY.
        await fetch('/api/auth/mock', { method: 'POST' }).catch(() => {})
        invalidateGlobalAccessQueries()
        router.push(ROUTES.DASHBOARD)
      } else {
        // SPEC_82: Keycloak OIDC Authorization Code + PKCE
        // Use public bootstrap endpoint (no auth required)
        const res = await fetch('/api/auth/config')
        if (!res.ok) throw new Error('Failed to load auth config')
        const authConfig = await res.json()
        const issuer: string = authConfig?.issuer || 'http://localhost:8080/realms/router'
        const clientId: string = authConfig?.clientId || 'router-ui'

        const redirectUri = typeof window !== 'undefined' ? `${window.location.origin}/auth/callback` : ''
        const { verifier, challenge } = await generatePkce()
        const state = randomString(24)
        const nonce = randomString(24)

        // Store temporary OIDC params for callback validation
        if (typeof window !== 'undefined') {
          localStorage.setItem('oidc_pkce_verifier', verifier)
          localStorage.setItem('oidc_state', state)
          localStorage.setItem('oidc_nonce', nonce)
          localStorage.setItem('oidc_issuer', issuer)
          localStorage.setItem('oidc_client_id', clientId)
          localStorage.setItem('oidc_redirect_uri', redirectUri)
        }

        const authUrl = buildAuthUrl({
          issuer,
          clientId,
          redirectUri,
          state,
          nonce,
          codeChallenge: challenge,
          scope: 'openid profile email offline_access',
        })
        if (typeof window !== 'undefined') {
          window.location.href = authUrl
        }
      }
    } catch (error) {
      console.error('Login error:', error)
    } finally {
      setIsLoading(false)
    }
  }, [router, invalidateGlobalAccessQueries])

  const logout = useCallback(async () => {
    // Read idToken BEFORE clearing session — needed for Keycloak end_session
    const idToken = getStoredSession()?.idToken

    // Clear FE session
    clearSession()
    setSession(null)
    clearGlobalConfigQueries()
    queryClient.removeQueries({ queryKey: ['replayGlobalAccess'] })
    queryClient.removeQueries({ queryKey: ['observabilityGlobalAccess'] })
    // Clear server cookies
    try {
      await fetch('/api/auth/logout', { method: 'POST' })
      await fetch('/api/auth/mock', { method: 'DELETE' }).catch(() => {})
    } catch {}

    // Invalidate the Keycloak SSO session so the next login prompts for credentials
    try {
      const cfg = await fetch('/api/auth/config').then((r) => r.json())
      const issuer: string = cfg?.issuer
      const clientId: string = cfg?.clientId
      if (issuer) {
        const endSession = new URL(`${issuer}/protocol/openid-connect/logout`)
        if (idToken) {
          endSession.searchParams.set('id_token_hint', idToken)
        } else if (clientId) {
          endSession.searchParams.set('client_id', clientId)
        }
        endSession.searchParams.set('post_logout_redirect_uri', `${window.location.origin}${ROUTES.LOGIN}`)
        window.location.href = endSession.toString()
        return
      }
    } catch {
      // fall through to router.push if config fetch fails
    }

    router.push(ROUTES.LOGIN)
  }, [clearGlobalConfigQueries, queryClient, router])

  useEffect(() => {
    // Prevent stale global config data from previous admin session being visible.
    if (session?.user.role !== 'admin') {
      clearGlobalConfigQueries()
    }
  }, [clearGlobalConfigQueries, session?.user.role])

  const refreshSession = useCallback(async () => {
    const storedSession = getStoredSession()
    setSession(storedSession)
  }, [])

  // SPEC_84: Refresh access token using refresh_token. Single in-flight refresh to avoid race conditions.
  const refreshAccessToken = useCallback(async (): Promise<boolean> => {
    if (refreshPromiseRef.current) return refreshPromiseRef.current
    const runRefresh = async (): Promise<boolean> => {
      setIsRefreshingSession(true)
      try {
        const currentSession = session || getStoredSession()
        if (!currentSession?.refreshToken) {
          if (process.env.NODE_ENV !== 'production') {
            console.log('[Refresh] ❌ No refresh token available')
          }
          return false
        }

        if (process.env.NODE_ENV !== 'production') {
          console.log('[Refresh] 🔄 Calling /api/auth/refresh')
          console.log('[Refresh] 📍 Using fixed issuer context: http://localhost:8080/realms/router')
          console.log('[Refresh] 🔑 Using fixed client_id context: router-ui')
          console.log('[Refresh] 🎫 Refresh token from session:', currentSession.refreshToken ? `present (${currentSession.refreshToken.length} chars)` : 'MISSING')
        }

        const res = await fetch('/api/auth/refresh', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            refresh_token: currentSession.refreshToken,
          }),
        })

        const data = await res.json()
        if (!res.ok || !data.ok || !data.access_token) {
          if (process.env.NODE_ENV !== 'production') {
            console.log('[Refresh] ❌ Refresh failed:', data.error || `HTTP ${res.status}`)
          }
          clearSession()
          setSession(null)
          return false
        }

        if (process.env.NODE_ENV !== 'production') {
          console.log('[Refresh] ✅ Refresh succeeded, updating session')
        }

        const expiresIn = data.expires_in || 3600
        const refreshExpiresIn = data.refresh_expires_in
        const newSession: Session = {
          ...currentSession,
          accessToken: data.access_token,
          refreshToken: data.refresh_token || currentSession.refreshToken,
          expiresAt: Date.now() + expiresIn * 1000,
          refreshExpiresAt: refreshExpiresIn ? Date.now() + refreshExpiresIn * 1000 : currentSession.refreshExpiresAt,
        }

        setStoredSession(newSession)
        setSession(newSession)
        invalidateGlobalAccessQueries()

        if (process.env.NODE_ENV !== 'production') {
          console.log('[Refresh] ✅ FE session updated, new expiresAt:', new Date(newSession.expiresAt).toISOString())
        }

        return true
      } catch (error) {
        if (process.env.NODE_ENV !== 'production') {
          console.error('[Refresh] ❌ Token refresh error:', error)
        }
        clearSession()
        setSession(null)
        return false
      } finally {
        setIsRefreshingSession(false)
        refreshPromiseRef.current = null
      }
    }
    const promise = runRefresh()
    refreshPromiseRef.current = promise
    return promise
  }, [session, invalidateGlobalAccessQueries])

  // SPEC_84: Ensure session is valid, refresh if needed
  const ensureValidSession = useCallback(async (): Promise<boolean> => {
    if (process.env.NODE_ENV !== 'production') {
      console.log('[Refresh] 🔍 ensureValidSession() called')
    }
    
    const currentSession = session || getStoredSession()
    if (!currentSession) {
      if (process.env.NODE_ENV !== 'production') {
        console.log('[Refresh] ❌ No session found')
      }
      return false
    }

    // Check if token is expired or about to expire (within 60 seconds)
    const now = Date.now()
    const timeUntilExpiry = currentSession.expiresAt - now
    const needsRefresh = currentSession.expiresAt <= now + 60000

    if (process.env.NODE_ENV !== 'production') {
      console.log('[Refresh] ⏰ Access token expires in:', Math.floor(timeUntilExpiry / 1000), 'seconds')
      console.log('[Refresh] 🔄 Needs refresh:', needsRefresh)
    }

    if (!needsRefresh) {
      if (process.env.NODE_ENV !== 'production') {
        console.log('[Refresh] ✅ Token still valid, no refresh needed')
      }
      return true
    }

    // Check if refresh token is still valid
    if (currentSession.refreshExpiresAt && currentSession.refreshExpiresAt <= now) {
      if (process.env.NODE_ENV !== 'production') {
        console.log('[Refresh] ❌ Refresh token expired, logging out')
      }
      // Refresh token expired, logout
      clearSession()
      setSession(null)
      return false
    }

    if (process.env.NODE_ENV !== 'production') {
      console.log('[Refresh] 🔄 Access token expired/expiring, attempting refresh')
    }

    // Attempt refresh
    return await refreshAccessToken()
  }, [refreshAccessToken, session])

  // Proactively refresh before expiry so global pages don't use stale RBAC with expired cookies.
  useEffect(() => {
    if (!session) return
    const tick = () => {
      const now = Date.now()
      if (session.expiresAt <= now + 60_000) {
        void ensureValidSession()
      }
    }
    const id = setInterval(tick, 30_000)
    tick()
    return () => clearInterval(id)
  }, [session, ensureValidSession])

  const value: AuthContextValue = {
    session,
    user: session?.user || null,
    isAuthenticated: !!session,
    isLoading,
    isRefreshingSession,
    login,
    logout,
    refreshSession,
    ensureValidSession,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuthContext(): AuthContextValue {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuthContext must be used within an AuthProvider')
  }
  return context
}
