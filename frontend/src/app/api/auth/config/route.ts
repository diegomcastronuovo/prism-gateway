import { NextResponse } from 'next/server'

/**
 * Public bootstrap endpoint for OIDC configuration
 * Does NOT require authentication - used by /login page
 */
export async function GET() {
  try {
    // Try to fetch from backend global config (may fail if not configured)
    const backendUrl = process.env.GATEWAY_ADMIN_URL || 'http://localhost:8000'
    
    try {
      const res = await fetch(`${backendUrl}/admin/config/global`, {
        headers: {
          'Content-Type': 'application/json',
          ...(process.env.GATEWAY_ADMIN_API_KEY && {
            'X-API-Key': process.env.GATEWAY_ADMIN_API_KEY,
          }),
        },
        cache: 'no-store',
      })

      if (res.ok) {
        const data = await res.json()
        const issuer = data?.config?.auth?.jwt?.issuer
        
        if (issuer) {
          return NextResponse.json({
            issuer,
            clientId: 'router-ui',
            configured: true,
          })
        }
      }
    } catch {
      // Backend not available or config not set, use fallback
    }

    // Fallback to Keycloak via public issuer (for browser)
    // Use KEYCLOAK_ISSUER_PUBLIC if set (Docker), otherwise localhost
    const publicIssuer = process.env.KEYCLOAK_ISSUER_PUBLIC || 'http://localhost:8080/realms/router'
    const clientId = process.env.KEYCLOAK_CLIENT_ID || 'router-ui'

    return NextResponse.json({
      issuer: publicIssuer,
      clientId,
      configured: true,
    })
  } catch (error) {
    console.error('Auth config error:', error)

    // Always return public issuer on error to allow login to proceed
    const publicIssuer = process.env.KEYCLOAK_ISSUER_PUBLIC || 'http://localhost:8080/realms/router'
    const clientId = process.env.KEYCLOAK_CLIENT_ID || 'router-ui'

    return NextResponse.json({
      issuer: publicIssuer,
      clientId,
      configured: true,
    })
  }
}
