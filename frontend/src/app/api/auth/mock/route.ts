import { NextResponse } from 'next/server'

const COOKIE = 'is_mock_session'
const PATH = 'Path=/; HttpOnly; SameSite=Lax'

/** POST /api/auth/mock — signals mock session to server-side API routes. */
export async function POST() {
  const resp = NextResponse.json({ ok: true })
  resp.headers.set('Set-Cookie', `${COOKIE}=1; ${PATH}; Max-Age=86400`)
  return resp
}

/** DELETE /api/auth/mock — clears mock session cookie on logout. */
export async function DELETE() {
  const resp = NextResponse.json({ ok: true })
  resp.headers.set(
    'Set-Cookie',
    `${COOKIE}=; ${PATH}; Max-Age=0; Expires=Thu, 01 Jan 1970 00:00:00 GMT`
  )
  return resp
}
