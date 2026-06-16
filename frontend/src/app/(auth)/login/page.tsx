'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/hooks/use-auth'
import { APP_NAME, ROUTES } from '@/lib/utils/constants'

export default function LoginPage() {
  const { login, isAuthenticated, isLoading } = useAuth()
  const [isMocking, setIsMocking] = useState(false)
  const router = useRouter()

  // SPEC_83: Redirect authenticated users to dashboard
  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.push(ROUTES.DASHBOARD)
    }
  }, [isAuthenticated, isLoading, router])

  // Show loading while checking session
  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-muted/40">
        <div className="text-muted-foreground">Checking session...</div>
      </div>
    )
  }

  // Don't show login UI if already authenticated
  if (isAuthenticated) {
    return null
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/40">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl text-center">{APP_NAME}</CardTitle>
          <CardDescription className="text-center">
            LLM Gateway — Sign in to continue
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Button
            className="w-full"
            loading={isMocking}
            loadingText="Signing in..."
            onClick={async () => {
              if (isMocking) return
              setIsMocking(true)
              await login('mock')
              setIsMocking(false)
            }}
          >
            Sign in
          </Button>

          <p className="text-center text-xs text-muted-foreground">
            To use SSO, configure an OIDC provider in Settings → Auth.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
