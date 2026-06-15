import { NextResponse } from 'next/server'

// Server-side issuer for token exchange (inside container, use host.docker.internal)
// Browser-side issuer is http://localhost:8080 (works from host)
const KEYCLOAK_ISSUER = process.env.KEYCLOAK_ISSUER_INTERNAL || 'http://localhost:8080/realms/router'
const KEYCLOAK_ISSUER_PUBLIC = process.env.KEYCLOAK_ISSUER_PUBLIC || 'http://localhost:8080/realms/router'
const KEYCLOAK_CLIENT_ID = process.env.KEYCLOAK_CLIENT_ID || 'router-ui'

type JwtLikePayload = {
  typ?: string
  azp?: string
  iss?: string
  sid?: string
}

function asString(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined
}

function asNumber(value: unknown): number | undefined {
  return typeof value === 'number' ? value : undefined
}

function sanitizeTokenResponseBody(data: Record<string, unknown>) {
  return {
    ...data,
    access_token: data.access_token ? '[redacted]' : undefined,
    refresh_token: data.refresh_token ? '[redacted]' : undefined,
    id_token: data.id_token ? '[redacted]' : undefined,
  }
}

function decodeJwtPayload(token: string): JwtLikePayload | null {
  try {
    const parts = token.split('.')
    if (parts.length < 2) return null
    const payload = JSON.parse(Buffer.from(parts[1], 'base64url').toString('utf8'))
    return payload as JwtLikePayload
  } catch {
    return null
  }
}

function formEncode(data: Record<string, string>) {
  return Object.entries(data)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&')
}

export async function POST(request: Request) {
  try {
    // ALWAYS log for debugging refresh issues
    console.log('[API /auth/refresh] 📥 Incoming request headers:', {
      'content-type': request.headers.get('content-type'),
      'content-length': request.headers.get('content-length'),
    })

    let body: any = {}
    let parseError: any = null

    try {
      body = await request.json()
      console.log('[API /auth/refresh] 📦 Successfully parsed JSON body:', {
        keys: Object.keys(body),
        hasRefreshToken: 'refresh_token' in body,
        refreshTokenLength: body.refresh_token ? String(body.refresh_token).length : 0,
      })
    } catch (err) {
      parseError = err
      console.error('[API /auth/refresh] ❌ Failed to parse JSON:', err instanceof Error ? err.message : String(err))
      body = {}
    }

    const { refresh_token } = body
    if (!refresh_token) {
      console.log('[API /auth/refresh] ❌ Missing required fields', {
        parseError: parseError ? (parseError instanceof Error ? parseError.message : String(parseError)) : null,
        bodyKeys: Object.keys(body),
        bodyContent: JSON.stringify(body),
      })
      return NextResponse.json(
        { ok: false, error: 'Missing refresh_token', debug: process.env.NODE_ENV !== 'production' ? { parseError: String(parseError), bodyKeys: Object.keys(body) } : undefined },
        { status: 400 }
      )
    }

    const refreshPayload = decodeJwtPayload(refresh_token)
    const tokenEndpoint = `${KEYCLOAK_ISSUER}/protocol/openid-connect/token`
    const matchesClient = refreshPayload?.azp === KEYCLOAK_CLIENT_ID
    // Token issuer may be PUBLIC or INTERNAL URL depending on where auth happened
    const matchesIssuer = refreshPayload?.iss === KEYCLOAK_ISSUER || refreshPayload?.iss === KEYCLOAK_ISSUER_PUBLIC
    const tokenType = refreshPayload?.typ
    const isSupportedRefreshType = tokenType === 'Refresh' || tokenType === 'Offline'

    // Log validation ALWAYS (even in production) — this is critical for debugging refresh failures
    console.log('[API /auth/refresh] VALIDATION_CHECK issuer_expected_primary=' + KEYCLOAK_ISSUER)
    console.log('[API /auth/refresh] VALIDATION_CHECK issuer_expected_fallback=' + KEYCLOAK_ISSUER_PUBLIC)
    console.log('[API /auth/refresh] VALIDATION_CHECK issuer_actual=' + (refreshPayload?.iss || 'MISSING'))
    console.log('[API /auth/refresh] VALIDATION_CHECK client_expected=' + KEYCLOAK_CLIENT_ID)
    console.log('[API /auth/refresh] VALIDATION_CHECK client_actual=' + (refreshPayload?.azp || 'MISSING'))
    console.log('[API /auth/refresh] VALIDATION_CHECK token_type=' + (refreshPayload?.typ || 'MISSING'))
    console.log('[API /auth/refresh] VALIDATION_CHECK results match_issuer=' + matchesIssuer + ' match_client=' + matchesClient + ' match_type=' + isSupportedRefreshType)

    if (!isSupportedRefreshType || !matchesClient || !matchesIssuer) {
      const failureReason = !isSupportedRefreshType ? 'invalid_token_type' : !matchesClient ? 'client_mismatch' : 'issuer_mismatch'
      console.error('[API /auth/refresh] VALIDATION_FAILED reason=' + failureReason + ' match_type=' + isSupportedRefreshType + ' match_client=' + matchesClient + ' match_issuer=' + matchesIssuer)
      console.error('[API /auth/refresh] VALIDATION_FAILED issuer_expected_vals=[' + KEYCLOAK_ISSUER + ', ' + KEYCLOAK_ISSUER_PUBLIC + '] issuer_actual=' + (refreshPayload?.iss || 'MISSING'))
      return NextResponse.json(
        { ok: false, error: 'Refresh token does not match router-ui router realm context', debug: {
          matchesClient,
          matchesIssuer,
          isSupportedRefreshType,
          expectedIssuers: [KEYCLOAK_ISSUER, KEYCLOAK_ISSUER_PUBLIC],
          actualIssuer: refreshPayload?.iss,
        }},
        { status: 400 }
      )
    }

    const res = await fetch(tokenEndpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formEncode({
        grant_type: 'refresh_token',
        client_id: KEYCLOAK_CLIENT_ID,
        refresh_token,
      }),
      cache: 'no-store',
    })

    const data = (await res.json().catch(() => ({}))) as Record<string, unknown>
    if (!res.ok) {
      const message = data?.error_description || data?.error || `HTTP ${res.status}`
      console.error('[API /auth/refresh] KEYCLOAK_EXCHANGE_FAILED status=' + res.status + ' error=' + message)
      return NextResponse.json({ ok: false, error: message }, { status: res.status })
    }

    console.log('[API /auth/refresh] KEYCLOAK_EXCHANGE_SUCCESS has_access_token=' + !!data.access_token + ' has_refresh_token=' + !!data.refresh_token)

    const accessToken = asString(data.access_token)
    const newRefreshToken = asString(data.refresh_token)
    const expiresIn = asNumber(data.expires_in)
    const refreshExpiresIn = asNumber(data.refresh_expires_in)

    if (!accessToken) {
      if (process.env.NODE_ENV !== 'production') {
        console.log('[API /auth/refresh] ❌ No access token in response')
      }
      return NextResponse.json({ ok: false, error: 'No access token returned' }, { status: 502 })
    }

    const resp = NextResponse.json({
      ok: true,
      access_token: accessToken,
      refresh_token: newRefreshToken || refresh_token,
      expires_in: expiresIn,
      refresh_expires_in: refreshExpiresIn,
    })

    // Update httpOnly cookies with new tokens
    // Only add Secure flag if running on HTTPS. localhost (even in production Docker) is HTTP.
    const origin = request.headers.get('origin') || request.headers.get('referer') || ''
    const isLocalhost = origin.includes('localhost')
    const secure = process.env.NODE_ENV === 'production' && !isLocalhost
    const cookieMaxAge = expiresIn || 3600
    const refreshCookieMaxAge = refreshExpiresIn || 604800

    resp.headers.append(
      'Set-Cookie',
      `admin_access_token=${encodeURIComponent(accessToken)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${cookieMaxAge}${secure ? '; Secure' : ''}`
    )

    if (newRefreshToken) {
      resp.headers.append(
        'Set-Cookie',
        `admin_refresh_token=${encodeURIComponent(newRefreshToken)}; Path=/; HttpOnly; SameSite=Lax; Max-Age=${refreshCookieMaxAge}${secure ? '; Secure' : ''}`
      )
    }

    console.log('[API /auth/refresh] COOKIES_SET access_max_age=' + cookieMaxAge + ' refresh_max_age=' + refreshCookieMaxAge + ' secure=' + secure)

    return resp
  } catch (error) {
    if (process.env.NODE_ENV !== 'production') {
      console.error('[API /auth/refresh] ❌ Unexpected error:', error)
    }
    return NextResponse.json({ ok: false, error: 'Unexpected error' }, { status: 500 })
  }
}
