import { NextResponse } from 'next/server'
import { getAdminAuthToken } from '@/lib/server/gateway-admin-client'

/**
 * FinOps and other RBAC-sensitive admin routes must call the gateway with the user's
 * Bearer token — never fall back to GATEWAY_ADMIN_API_KEY from the browser.
 */
export async function requireAdminBearer(request: Request): Promise<
  | { token: string }
  | { response: NextResponse }
> {
  const token = await getAdminAuthToken(request)
  if (!token || token.length === 0) {
    return {
      response: NextResponse.json({ error: 'Authentication required' }, { status: 401 }),
    }
  }
  return { token }
}
