import { NextResponse } from 'next/server'

export async function POST() {
  const resp = NextResponse.json({ ok: true })
  const secure = process.env.NODE_ENV === 'production'
  const expired = 'Max-Age=0; Expires=Thu, 01 Jan 1970 00:00:00 GMT'
  const flags = `Path=/; HttpOnly; SameSite=Lax; ${expired}${secure ? '; Secure' : ''}`
  resp.headers.append('Set-Cookie', `admin_access_token=; ${flags}`)
  resp.headers.append('Set-Cookie', `admin_refresh_token=; ${flags}`)
  return resp
}
