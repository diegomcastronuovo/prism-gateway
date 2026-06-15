'use client'

import { Suspense } from 'react'
import { useEffect, useRef, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { decodeJwtPayload } from '@/lib/auth/oidc'
import { extractRolesFromClaims, deriveCanonicalRole } from '@/lib/auth/roles'
import { clearSession, setStoredSession } from '@/lib/auth/session'
import { ROUTES } from '@/lib/utils/constants'
import { useAuth } from '@/hooks/use-auth'

function OidcCallbackContent() {
  const router = useRouter()
  const search = useSearchParams()
  const [message, setMessage] = useState('Completing login...')
  const { refreshSession } = useAuth()
  
  // Use ref to prevent double exchange in React StrictMode
  const exchangeStartedRef = useRef(false)

  useEffect(() => {
    const run = async () => {
      const code = search.get('code')
      const state = search.get('state')
      const storedState = localStorage.getItem('oidc_state')
      const verifier = localStorage.getItem('oidc_pkce_verifier') || ''
      const issuer = localStorage.getItem('oidc_issuer') || ''
      const clientId = localStorage.getItem('oidc_client_id') || 'router-ui'
      const redirectUriStored = localStorage.getItem('oidc_redirect_uri')
      const redirectUri = redirectUriStored || `${window.location.origin}/auth/callback`

      if (process.env.NODE_ENV !== 'production') {
        console.log('[Callback] 🔄 useEffect triggered', {
          code: code ? `${code.substring(0, 20)}...` : 'MISSING',
          state: state ? `${state.substring(0, 20)}...` : 'MISSING',
          exchangeAlreadyStarted: exchangeStartedRef.current,
        })
      }

      // Prevent double exchange if already started in this component instance
      if (exchangeStartedRef.current) {
        if (process.env.NODE_ENV !== 'production') {
          console.log('[Callback] ⚠️ Exchange already started (useRef protection), skipping')
        }
        return
      }

      if (!code || !state) {
        setMessage('Invalid login response. Missing parameters.')
        return
      }
      if (!storedState || storedState !== state) {
        setMessage('Invalid login state. Please try again.')
        return
      }
      // Ensure PKCE verifier and exact redirect_uri are available
      if (!verifier) {
        setMessage('Missing PKCE verifier. Please restart the login process.')
        return
      }
      if (!redirectUriStored) {
        setMessage('Missing redirect URI. Please restart the login process.')
        return
      }
      
      // Prevent double exchange for the same state (cross-page protection)
      const exchangedKey = `oidc_exchanged_${state}`
      const alreadyExchanged = localStorage.getItem(exchangedKey)
      
      if (process.env.NODE_ENV !== 'production') {
        console.log('[Callback] 🔍 localStorage check:', {
          exchangedKey,
          storedValue: alreadyExchanged,
          willSkip: alreadyExchanged === '1',
          allKeys: Object.keys(localStorage).filter(k => k.includes('oidc') || k.includes('exchanged')),
        })
      }

      if (alreadyExchanged === '1') {
        if (process.env.NODE_ENV !== 'production') {
          console.log('[Callback] ✅ Code already exchanged (found in localStorage), skipping POST')
        }
        setMessage('Login already completed. Redirecting...')
        router.replace(ROUTES.DASHBOARD)
        return
      }

      // Mark that we're starting the exchange to prevent React StrictMode double-call
      exchangeStartedRef.current = true

      if (process.env.NODE_ENV !== 'production') {
        console.log('[Callback] 📤 Making POST to /api/auth/callback...')
      }

      try {
        console.log('[Callback] POST_CALLING /api/auth/callback')
        const res = await fetch('/api/auth/callback', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ code, verifier, issuer, client_id: clientId, redirect_uri: redirectUri }),
        })
        console.log('[Callback] POST_RESPONSE status=' + res.status)
        const data = await res.json().catch(() => ({}))
        if (!res.ok) {
          if (process.env.NODE_ENV !== 'production') {
            console.error('[Callback] ❌ POST failed:', {
              status: res.status,
              error: data?.error,
              debug: data?.debug,
            })
          }
          setMessage(data?.error || 'Failed to complete login.')
          return
        }
        const accessToken: string | undefined = data?.access_token
        const idToken: string | undefined = data?.id_token
        const refreshToken: string | undefined = data?.refresh_token
        
        if (process.env.NODE_ENV !== 'production') {
          console.log('[Callback] 🎫 Tokens received from /api/auth/callback:')
          console.log('[Callback]   - access_token:', accessToken ? `present (${accessToken.length} chars)` : 'MISSING')
          console.log('[Callback]   - refresh_token:', refreshToken ? `present (${refreshToken.length} chars)` : 'MISSING')
          console.log('[Callback]   - id_token:', idToken ? 'present' : 'none')
          console.log('[Callback]   - expires_in:', data?.expires_in)
          console.log('[Callback]   - refresh_expires_in:', data?.refresh_expires_in)
        }
        
        if (!accessToken) {
          setMessage('Failed to receive access token.')
          return
        }
        // Build FE session for client use (token also set in httpOnly cookie by the API route)
        const claims = idToken ? decodeJwtPayload(idToken) : (accessToken ? decodeJwtPayload(accessToken) : null)
        const preferred = claims?.preferred_username
        const email = claims?.email || ''
        const sub = claims?.sub || 'me'
        const displayName = preferred || email || sub
        // SPEC_87: derive canonical role from token claims
        const roles = extractRolesFromClaims(claims)
        const canonicalRole = deriveCanonicalRole(roles) || 'user'
        const expiresIn = data?.expires_in || 3600
        const refreshExpiresIn = data?.refresh_expires_in
        const exp = claims?.exp ? claims.exp * 1000 : Date.now() + expiresIn * 1000
        const refreshExp = refreshExpiresIn ? Date.now() + refreshExpiresIn * 1000 : undefined
        
        if (process.env.NODE_ENV !== 'production') {
          console.log('[Callback] 💾 Storing session with refresh_token:', refreshToken ? 'YES' : 'NO')
        }

        // Clear any stale session before writing fresh tokens from this login (same-session consistency).
        clearSession()
        console.log('[Callback] STORING_SESSION')
        setStoredSession({
          user: { id: sub, name: displayName, email, role: canonicalRole },
          accessToken,
          idToken,
          refreshToken,
          expiresAt: exp,
          refreshExpiresAt: refreshExp,
        })

        // Mark as exchanged to avoid double POSTs (cross-page protection)
        localStorage.setItem(exchangedKey, '1')
        if (process.env.NODE_ENV !== 'production') {
          console.log('[Callback] 🔒 Marked code as exchanged in localStorage:', {
            key: exchangedKey,
            value: '1',
            verified: localStorage.getItem(exchangedKey),
          })
        }

        // Cleanup temp storage
        localStorage.removeItem('oidc_pkce_verifier')
        localStorage.removeItem('oidc_state')
        localStorage.removeItem('oidc_nonce')
        localStorage.removeItem('oidc_issuer')
        localStorage.removeItem('oidc_client_id')
        localStorage.removeItem('oidc_redirect_uri')

        // Refresh in-memory auth immediately to avoid guard/login race
        try { await refreshSession() } catch {}
        router.replace(ROUTES.DASHBOARD)
      } catch (e) {
        if (process.env.NODE_ENV !== 'production') {
          console.error('[Callback] ❌ Exception during exchange:', e)
        }
        setMessage('Failed to complete login.')
      }
    }
    run()
  }, [router, search])

  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="text-sm text-muted-foreground">{message}</div>
    </div>
  )
}

export default function OidcCallbackPage() {
  return (
    <Suspense fallback={<div className="flex min-h-screen items-center justify-center"><div className="text-sm text-muted-foreground">Loading...</div></div>}>
      <OidcCallbackContent />
    </Suspense>
  )
}
