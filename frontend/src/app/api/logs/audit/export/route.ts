import { NextResponse } from 'next/server'
import { requireAdminBearer } from '@/lib/server/require-admin-bearer'

export const dynamic = 'force-dynamic'

export async function GET(request: Request) {
  const auth = await requireAdminBearer(request)
  if ('response' in auth) return auth.response

  const { searchParams } = new URL(request.url)
  const baseUrl = process.env.GATEWAY_BASE_URL

  if (!baseUrl) {
    return NextResponse.json({ error: 'Missing GATEWAY_BASE_URL' }, { status: 500 })
  }

  const url = `${baseUrl}/admin/audit/requests/export.csv?${searchParams.toString()}`

  try {
    const resp = await fetch(url, {
      headers: {
        Authorization: `Bearer ${auth.token}`,
      },
      cache: 'no-store',
    })

    if (!resp.ok) {
      const text = await resp.text()
      return NextResponse.json(
        { error: `Upstream error (${resp.status})`, details: text },
        { status: resp.status }
      )
    }

    const body = await resp.arrayBuffer()
    const contentType = resp.headers.get('content-type') ?? 'text/csv; charset=utf-8'
    const contentDisposition = resp.headers.get('content-disposition')
    return new NextResponse(body, {
      headers: {
        'Content-Type': contentType,
        'Content-Disposition': contentDisposition ?? 'attachment; filename="audit_export.csv"',
        'Cache-Control': 'no-store',
      },
    })
  } catch (error) {
    console.error('Failed to fetch audit export', error)
    return NextResponse.json({ error: 'Failed to fetch audit export' }, { status: 500 })
  }
}
