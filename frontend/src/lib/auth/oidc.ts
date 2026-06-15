export function randomString(length = 32): string {
  const array = new Uint8Array(length)
  if (typeof window !== 'undefined' && window.crypto?.getRandomValues) {
    window.crypto.getRandomValues(array)
  } else {
    for (let i = 0; i < length; i++) array[i] = Math.floor(Math.random() * 256)
  }
  return base64UrlEncode(array)
}

export async function sha256(input: string): Promise<ArrayBuffer> {
  const encoder = new TextEncoder()
  const data = encoder.encode(input)
  if (typeof window !== 'undefined' && window.crypto?.subtle) {
    return window.crypto.subtle.digest('SHA-256', data)
  }
  throw new Error('SHA-256 not available in this environment')
}

export function base64UrlEncode(buffer: ArrayBuffer | Uint8Array): string {
  const bytes = buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer)
  let binary = ''
  bytes.forEach((b) => (binary += String.fromCharCode(b)))
  const b64 = btoa(binary)
  return b64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export async function generatePkce(): Promise<{ verifier: string; challenge: string }> {
  const verifier = randomString(64)
  const challengeBuffer = await sha256(verifier)
  const challenge = base64UrlEncode(challengeBuffer)
  return { verifier, challenge }
}

export function buildAuthUrl(params: {
  issuer: string
  clientId: string
  redirectUri: string
  scope?: string
  state: string
  nonce: string
  codeChallenge: string
}): string {
  const authEndpoint = `${params.issuer.replace(/\/$/, '')}/protocol/openid-connect/auth`
  const qs = new URLSearchParams({
    client_id: params.clientId,
    redirect_uri: params.redirectUri,
    response_type: 'code',
    scope: params.scope || 'openid profile email',
    state: params.state,
    nonce: params.nonce,
    code_challenge: params.codeChallenge,
    code_challenge_method: 'S256',
  })
  return `${authEndpoint}?${qs.toString()}`
}

export function decodeJwtPayload(token: string): any | null {
  try {
    const parts = token.split('.')
    if (parts.length < 2) return null
    const payload = parts[1]
    const json = JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/')))
    return json
  } catch {
    return null
  }
}
