import { useState, useCallback } from 'react'

export interface HealthStatus {
  status: 'ok' | 'error' | 'disabled' | 'loading'
  error?: string
}

export interface TableHealthStatus extends HealthStatus {
  missing?: string[]
  found?: number
  expected?: number
}

export interface SystemHealth {
  gateway: HealthStatus
  postgres: HealthStatus
  redis: HealthStatus
  keycloak: HealthStatus
  tables: TableHealthStatus
}

async function fetchSystemHealth(): Promise<SystemHealth> {
  const res = await fetch('/api/system-health')
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { message: 'Unknown error' } }))
    throw new Error(err.error?.message || 'Health check failed')
  }
  return res.json()
}

export function useSystemHealth() {
  const [data, setData] = useState<SystemHealth | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const run = useCallback(async () => {
    setIsLoading(true)
    setError(null)
    try {
      const result = await fetchSystemHealth()
      setData(result)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Health check failed')
    } finally {
      setIsLoading(false)
    }
  }, [])

  return { data, isLoading, error, run }
}
