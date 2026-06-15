import { NextResponse } from 'next/server'

export const dynamic = 'force-dynamic'

function normalizeEndpoint(raw: string): string {
  const value = raw.trim().replace(/\/+$/, '')
  if (!value) return ''
  if (/^\d+$/.test(value)) {
    return `http://localhost:${value}`
  }
  if (value.startsWith('localhost:') || value.startsWith('127.0.0.1:')) {
    return `http://${value}`
  }
  if (!/^https?:\/\//i.test(value)) {
    return `http://${value}`
  }
  return value
}

export async function GET(request: Request) {
  try {
    const { searchParams } = new URL(request.url)
    const endpoint = searchParams.get('endpoint') || ''
    if (!endpoint) {
      return NextResponse.json({ ok: false, error: 'Missing endpoint' }, { status: 400 })
    }

    const normalized = normalizeEndpoint(endpoint)
    if (!normalized) {
      return NextResponse.json({ ok: false, error: 'Invalid endpoint' }, { status: 400 })
    }
    const url = `${normalized}/healthz`
    const start = Date.now()
    try {
      const res = await fetch(url, { method: 'GET', signal: AbortSignal.timeout(5000) })
      const duration = Date.now() - start
      if (res.ok) {
        return NextResponse.json({ ok: true, responseTimeMs: duration })
      }
      return NextResponse.json(
        { ok: false, error: `HTTP ${res.status}`, responseTimeMs: duration },
        { status: 502 }
      )
    } catch (err) {
      const duration = Date.now() - start
      const message = err instanceof Error ? err.message : 'Network error'
      return NextResponse.json(
        { ok: false, error: message, responseTimeMs: duration },
        { status: 502 }
      )
    }
  } catch {
    return NextResponse.json({ ok: false, error: 'Invalid request' }, { status: 400 })
  }
}
