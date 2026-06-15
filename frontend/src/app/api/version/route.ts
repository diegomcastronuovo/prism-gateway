import { NextResponse } from 'next/server'
import { promises as fs } from 'fs'
import path from 'path'

function parseProperties(content: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const rawLine of content.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const eq = line.indexOf('=')
    if (eq === -1) continue
    const key = line.slice(0, eq).trim()
    const value = line.slice(eq + 1).trim()
    if (key) out[key] = value
  }
  return out
}

export async function GET() {
  try {
    const versionPath = path.join(process.cwd(), 'public', 'version', 'version.txt')
    const buf = await fs.readFile(versionPath)
    const props = parseProperties(buf.toString('utf8'))
    const version = props['version'] || null
    const details = props['details'] || null
    return NextResponse.json({ version, details })
  } catch (err) {
    if (process.env.NODE_ENV !== 'production') {
      console.error('[API /version] error reading version file:', err)
    }
    return NextResponse.json({ version: null, details: null }, { status: 200 })
  }
}
