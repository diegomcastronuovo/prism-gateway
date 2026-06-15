'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/hooks/use-auth'
import { APP_NAME, ROUTES } from '@/lib/utils/constants'

export default function LoginPage() {
  const { login, isAuthenticated, isLoading } = useAuth()
  const [isRedirectingKeycloak, setIsRedirectingKeycloak] = useState(false)
  const [isRedirectingCognito, setIsRedirectingCognito] = useState(false)
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
          <div className="mx-auto mb-4 h-20 w-28 flex items-center justify-center">
            <img src="/logo.svg" alt="AI Gateway Logo" className="h-full w-full" />
          </div>
          <CardTitle className="text-2xl text-center">{"arkana"}</CardTitle>
          <CardDescription className="text-center">
            AI Control Plane - Sign in to continue
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-3">
            <Button
              className="w-full"
              variant="outline"
              loading={isRedirectingKeycloak}
              loadingText="Redirecting..."
              onClick={() => {
                if (isRedirectingKeycloak) return
                setIsRedirectingKeycloak(true)
                login('keycloak')
              }}
            >
              Continue with Keycloak
            </Button>
            
            
            
            
          </div>

          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <span className="w-full border-t" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-card px-2 text-muted-foreground">
                Development Only
              </span>
            </div>
          </div>

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
            Continue with Mock Session
          </Button>

          <p className="text-center text-xs text-muted-foreground">
            Use mock session for local development without Keycloak.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
