import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'

const KEYCLOAK_ISSUER = process.env.KEYCLOAK_ISSUER_INTERNAL || 'http://localhost:8080/realms/router'
const KEYCLOAK_CLIENT_ID = process.env.KEYCLOAK_CLIENT_ID || 'router-ui'

function formEncode(data: Record<string, string>) {
  return Object.entries(data)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&')
}

/**
 * Server-only endpoint to refresh access token using refresh_token cookie.
 * Called internally by server-side API routes when they get a 401.
 * Returns new tokens and updates httpOnly cookies.
 */
export async function POST() {
  try {
    const cookieStore = await cookies()
    const refreshToken = cookieStore.get('admin_refresh_token')?.value

    if (!refreshToken) {
      if (process.env.NODE_ENV !== 'production') {
        console.log('[API /auth/refresh-server] ❌ No refresh_token cookie found')
      }
      return NextResponse.json(
        { ok: false, error: 'No refresh token available' },
        { status: 401 }
      )
    }

    const tokenEndpoint = `${KEYCLOAK_ISSUER}/protocol/openid-connect/token`

    if (process.env.NODE_ENV !== 'production') {
      console.log('[API /auth/refresh-server] 🔄 Exchanging refresh_token from server')
      console.log('[API /auth/refresh-server] 📍 Issuer:', KEYCLOAK_ISSUER)
    }

    const res = await fetch(tokenEndpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formEncode({
        grant_type: 'refresh_token',
        client_id: KEYCLOAK_CLIENT_ID,
        refresh_token: refreshToken,
      }),
      cache: 'no-store',
    })

    const data = (await res.json().catch(() => ({}))) as Record<string, unknown>
    if (!res.ok) {
      const message = data?.error_description || data?.error || `HTTP ${res.status}`
      if (process.env.NODE_ENV !== 'production') {
        console.log('[API /auth/refresh-server] ❌ Keycloak refresh failed:', message)
      }
      return NextResponse.json({ ok: false, error: message }, { status: res.status })
    }

    const accessToken = typeof data.access_token === 'string' ? data.access_token : null
    const newRefreshToken = typeof data.refresh_token === 'string' ? data.refresh_token : null
    const expiresIn = typeof data.expires_in === 'number' ? data.expires_in : 3600

    if (!accessToken) {
      return NextResponse.json({ ok: false, error: 'No access token returned' }, { status: 502 })
    }

    const resp = NextResponse.json({
      ok: true,
      access_token: accessToken,
      refresh_token: newRefreshToken || refreshToken,
      expires_in: expiresIn,
    })

    const secure = process.env.NODE_ENV === 'production'
    resp.headers.append(
      'Set-Cookie',
      `admin_access_token=${encodeURIComponent(accessToken)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${expiresIn}${secure ? '; Secure' : ''}`
    )

    if (newRefreshToken) {
      resp.headers.append(
        'Set-Cookie',
        `admin_refresh_token=${encodeURIComponent(newRefreshToken)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=604800${secure ? '; Secure' : ''}`
      )
    }

    if (process.env.NODE_ENV !== 'production') {
      console.log('[API /auth/refresh-server] ✅ Token refresh succeeded, cookies updated')
    }

    return resp
  } catch (error) {
    if (process.env.NODE_ENV !== 'production') {
      console.error('[API /auth/refresh-server] ❌ Error:', error)
    }
    return NextResponse.json({ ok: false, error: 'Unexpected error' }, { status: 500 })
  }
}
