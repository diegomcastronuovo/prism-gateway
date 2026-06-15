import { NextResponse } from 'next/server'

const KEYCLOAK_ISSUER_INTERNAL = process.env.KEYCLOAK_ISSUER_INTERNAL || 'http://localhost:8080/realms/router'

function formEncode(data: Record<string, string>) {
  return Object.entries(data)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&')
}

export async function POST(request: Request) {
  try {
    console.log('[OIDC] CALLBACK_START')
    const body = (await request.json().catch(() => ({}))) as {
      code?: string
      verifier?: string
      issuer?: string
      client_id?: string
      redirect_uri?: string
    }

    const { code, verifier, client_id, redirect_uri } = body

    console.log('[OIDC] CALLBACK_PARAMS code=' + (code ? 'yes' : 'no') + ' verifier=' + (verifier ? 'yes' : 'no') + ' client_id=' + client_id + ' redirect_uri=' + (redirect_uri ? 'yes' : 'no'))

    if (!code || !verifier || !client_id || !redirect_uri) {
      if (process.env.NODE_ENV !== 'production') {
        console.error('[OIDC] ❌ Validation failed:', {
          missing: [
            !code && 'code',
            !verifier && 'verifier',
            !client_id && 'client_id',
            !redirect_uri && 'redirect_uri',
          ].filter(Boolean),
        })
      }
      return NextResponse.json(
        { error: 'Missing required fields', details: {
          code: !!code,
          verifier: !!verifier,
          client_id: !!client_id,
          redirect_uri: !!redirect_uri,
        }},
        { status: 400 }
      )
    }

    // Use server-side internal Keycloak URL for token exchange (not the public URL from client)
    const tokenEndpoint = `${KEYCLOAK_ISSUER_INTERNAL.replace(/\/$/, '')}/protocol/openid-connect/token`

    // DEV: log sanitized payload to help diagnose redirect_uri / PKCE issues
    if (process.env.NODE_ENV !== 'production') {
      try {
        console.log('[OIDC] ✅ All fields present, exchanging code at', tokenEndpoint)
        console.log('[OIDC] Payload (sanitized):', {
          grant_type: 'authorization_code',
          client_id,
          code: `len:${code.length}`,
          redirect_uri,
          code_verifier: `len:${verifier.length}`,
        })
      } catch {}
    }

    const res = await fetch(tokenEndpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formEncode({
        grant_type: 'authorization_code',
        client_id,
        code,
        redirect_uri,
        code_verifier: verifier,
      }),
      cache: 'no-store',
    })

    console.log('[OIDC] KEYCLOAK_RESPONSE status=' + res.status)
    const data = await res.json().catch(() => ({}))
    if (!res.ok) {
      const message = data?.error_description || data?.error || `HTTP ${res.status}`
      console.error('[OIDC] KEYCLOAK_EXCHANGE_FAILED ' + message)
      return NextResponse.json({ error: message }, { status: res.status })
    }

    console.log('[OIDC] KEYCLOAK_EXCHANGE_SUCCESS has_access=' + !!data.access_token + ' has_refresh=' + !!data.refresh_token)

    const accessToken: string | undefined = data.access_token
    const idToken: string | undefined = data.id_token
    const refreshToken: string | undefined = data.refresh_token
    const expiresIn: number | undefined = data.expires_in
    const refreshExpiresIn: number | undefined = data.refresh_expires_in

    if (!accessToken) {
      return NextResponse.json({ error: 'No access token returned' }, { status: 502 })
    }

    // Only add Secure flag if running on HTTPS. localhost (even in production Docker) is HTTP.
    const origin = request.headers.get('origin') || request.headers.get('referer') || ''
    const isLocalhost = origin.includes('localhost')
    const secure = process.env.NODE_ENV === 'production' && !isLocalhost
    const accessTokenMaxAge = expiresIn || 3600
    const refreshTokenMaxAge = refreshExpiresIn || 604800 // 7 days default

    const resp = NextResponse.json({
      access_token: accessToken,
      id_token: idToken,
      refresh_token: refreshToken,
      expires_in: expiresIn,
      refresh_expires_in: refreshExpiresIn,
    })

    // Set httpOnly cookies so server-side API proxies can use them
    const cookieSameSite = 'SameSite=Lax'
    const cookiePath = 'Path=/'
    const cookieHttpOnly = 'HttpOnly'
    const cookieSecure = secure ? 'Secure' : ''

    const accessCookieValue = `admin_access_token=${encodeURIComponent(accessToken)}; ${cookiePath}; ${cookieHttpOnly}; ${cookieSameSite}; Max-Age=${accessTokenMaxAge}${cookieSecure ? '; ' + cookieSecure : ''}`
    console.log('[OAuth Callback] SETTING_COOKIE access_token secure=' + secure + ' max_age=' + accessTokenMaxAge)
    resp.headers.append('Set-Cookie', accessCookieValue)

    if (refreshToken) {
      const refreshCookieValue = `admin_refresh_token=${encodeURIComponent(refreshToken)}; ${cookiePath}; ${cookieHttpOnly}; ${cookieSameSite}; Max-Age=${refreshTokenMaxAge}${cookieSecure ? '; ' + cookieSecure : ''}`
      console.log('[OAuth Callback] SETTING_COOKIE refresh_token secure=' + secure + ' max_age=' + refreshTokenMaxAge)
      resp.headers.append('Set-Cookie', refreshCookieValue)
    }

    console.log('[OAuth Callback] CALLBACK_COMPLETE returning_tokens')
    return resp
  } catch (error) {
    console.error('[OAuth Callback] ❌ Unexpected error:', error)
    const message = error instanceof Error ? error.message : String(error)
    return NextResponse.json({ error: 'Unexpected error', details: message }, { status: 500 })
  }
}
