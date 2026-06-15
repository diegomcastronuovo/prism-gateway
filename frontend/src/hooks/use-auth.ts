import { useAuthContext } from '@/lib/auth/auth-context'

export function useAuth() {
  return useAuthContext()
}
