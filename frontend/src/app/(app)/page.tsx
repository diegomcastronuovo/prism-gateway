'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { ROUTES } from '@/lib/utils/constants'

export default function AppIndexPage() {
  const router = useRouter()

  useEffect(() => {
    router.push(ROUTES.DASHBOARD)
  }, [router])

  return null
}
