import { NextResponse } from 'next/server'
import { getAdminAuthToken } from '@/lib/server/gateway-admin-client'

/**
 * Require authenticated user (Bearer token) for tenant-scoped routes.
 * Prevents fallback to system admin API key which violates tenant isolation.
 */
export async function requireTenantAuth(request: Request): Promise<
  | { token: string }
  | { response: NextResponse }
> {
  const token = await getAdminAuthToken(request)
  if (!token || token.length === 0) {
    return {
      response: NextResponse.json(
        { error: 'Authentication required' },
        { status: 401 }
      ),
    }
  }
  return { token }
}
