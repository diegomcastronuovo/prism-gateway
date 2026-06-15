import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import { decodeJwtPayload } from '@/lib/auth/oidc'

export async function GET() {
  try {
    const cookieStore = await cookies()
    const token = cookieStore.get('admin_access_token')?.value

    if (!token) {
      return NextResponse.json({ authenticated: false })
    }

    // Decode token to extract user info
    try {
      const claims = decodeJwtPayload(token)
      const preferred = claims?.preferred_username
      const email = claims?.email || ''
      const sub = claims?.sub || 'me'
      const displayName = preferred || email || sub

      // Check if token is expired
      const exp = claims?.exp
      if (exp && exp * 1000 < Date.now()) {
        return NextResponse.json({ authenticated: false })
      }

      return NextResponse.json({
        authenticated: true,
        user: {
          id: sub,
          name: displayName,
          email,
        },
      })
    } catch {
      // Invalid token
      return NextResponse.json({ authenticated: false })
    }
  } catch (error) {
    console.error('Session check error:', error)
    return NextResponse.json({ authenticated: false })
  }
}
