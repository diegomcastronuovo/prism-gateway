'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useAuthContext } from './auth-context'
import { LoadingScreen } from '@/components/shared/loading-screen'
import { ROUTES } from '@/lib/utils/constants'

interface AuthGuardProps {
  children: React.ReactNode
}

export function AuthGuard({ children }: AuthGuardProps) {
  const { isAuthenticated, isLoading } = useAuthContext()
  const router = useRouter()

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push(ROUTES.LOGIN)
    }
  }, [isAuthenticated, isLoading, router])

  if (isLoading) {
    return <LoadingScreen message="Checking session..." />
  }

  if (!isAuthenticated) {
    return null
  }

  return <>{children}</>
}
