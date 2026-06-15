import { ApiClientConfig, RequestConfig, ApiResponse } from '@/types/api'
import { API_BASE_URL } from '@/lib/utils/constants'

class ApiClient {
  private config: ApiClientConfig
  private isRefreshing = false
  private refreshQueue: Array<() => void> = []

  constructor(config?: Partial<ApiClientConfig>) {
    this.config = {
      baseURL: config?.baseURL || API_BASE_URL,
      apiKey: config?.apiKey,
      bearerToken: config?.bearerToken,
    }
  }

  private getHeaders(customHeaders?: Record<string, string>): HeadersInit {
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
      ...customHeaders,
    }

    if (this.config.apiKey) {
      headers['X-API-Key'] = this.config.apiKey
    }

    if (this.config.bearerToken) {
      headers['Authorization'] = `Bearer ${this.config.bearerToken}`
    }

    return headers
  }

  private async handleTokenRefresh(): Promise<boolean> {
    if (this.isRefreshing) {
      return new Promise((resolve) => {
        this.refreshQueue.push(() => resolve(true))
      })
    }

    this.isRefreshing = true
    try {
      if (typeof window === 'undefined') {
        return false
      }

      const stored = localStorage.getItem('ai_gateway_session')
      if (!stored) {
        if (process.env.NODE_ENV !== 'production') {
          console.log('[ApiClient] ❌ No session in localStorage')
        }
        return false
      }

      const session = JSON.parse(stored)
      const refreshToken = session?.refreshToken

      if (!refreshToken) {
        if (process.env.NODE_ENV !== 'production') {
          console.log('[ApiClient] ❌ No refreshToken in session')
        }
        return false
      }

      const refreshPayload = { refresh_token: refreshToken }
      const refreshBody = JSON.stringify(refreshPayload)

      if (process.env.NODE_ENV !== 'production') {
        console.log('[ApiClient] 📤 Sending token refresh request with payload:', {
          url: '/api/auth/refresh',
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          bodyLength: refreshBody.length,
          bodyPreview: refreshBody.substring(0, 100),
          hasRefreshToken: 'refresh_token' in refreshPayload,
          refreshTokenLength: refreshToken.length,
        })
      }

      const response = await fetch('/api/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: refreshBody,
      })

      console.log('[ApiClient] REFRESH_RESPONSE status=' + response.status)

      if (response.ok) {
        const data = await response.json()

        console.log('[ApiClient] REFRESH_RESPONSE_BODY ok=' + data.ok + ' has_token=' + !!data.access_token + ' expires_in=' + data.expires_in)

        if (data.ok && data.access_token) {
          const expiresIn = data.expires_in || 3600
          const refreshExpiresIn = data.refresh_expires_in
          const updatedSession = {
            ...session,
            accessToken: data.access_token,
            refreshToken: data.refresh_token || refreshToken,
            expiresAt: Date.now() + expiresIn * 1000,
            refreshExpiresAt: refreshExpiresIn ? Date.now() + refreshExpiresIn * 1000 : session.refreshExpiresAt,
          }
          localStorage.setItem('ai_gateway_session', JSON.stringify(updatedSession))
          if (process.env.NODE_ENV !== 'production') {
            console.log('[ApiClient] ✅ Token refresh succeeded, session updated, expires in:', expiresIn, 'seconds')
          }
          this.refreshQueue.forEach(cb => cb())
          this.refreshQueue = []
          return true
        }
      }

      // response.ok is false OR response parsed but data.ok is false
      try {
        const errorData = await response.json()
        console.error('[ApiClient] REFRESH_FAILED status=' + response.status + ' error=' + errorData.error)
      } catch {
        console.error('[ApiClient] REFRESH_FAILED status=' + response.status + ' could_not_parse_response')
      }
      this.refreshQueue.forEach(cb => cb())
      this.refreshQueue = []
      return false
    } catch (error) {
      if (process.env.NODE_ENV !== 'production') {
        console.error('[ApiClient] ❌ Token refresh error:', error)
      }
      this.refreshQueue = []
      return false
    } finally {
      this.isRefreshing = false
    }
  }

  async request<T = unknown>(
    endpoint: string,
    config?: RequestConfig
  ): Promise<ApiResponse<T>> {
    const url = new URL(endpoint, this.config.baseURL)

    if (config?.params) {
      Object.entries(config.params).forEach(([key, value]) => {
        url.searchParams.append(key, String(value))
      })
    }

    const response = await fetch(url.toString(), {
      method: config?.method || 'GET',
      headers: this.getHeaders(config?.headers),
      body: config?.body ? JSON.stringify(config.body) : undefined,
    })

    if (response.status === 401) {
      if (process.env.NODE_ENV !== 'production') {
        console.log('[ApiClient] 🔄 Received 401, attempting token refresh')
      }
      const refreshed = await this.handleTokenRefresh()
      if (refreshed) {
        if (process.env.NODE_ENV !== 'production') {
          console.log('[ApiClient] ✅ Token refreshed, retrying request')
        }
        const retryResponse = await fetch(url.toString(), {
          method: config?.method || 'GET',
          headers: this.getHeaders(config?.headers),
          body: config?.body ? JSON.stringify(config.body) : undefined,
        })

        if (!retryResponse.ok) {
          throw new Error(`API Error: ${retryResponse.status} ${retryResponse.statusText}`)
        }

        const data = await retryResponse.json()
        return {
          data,
          status: retryResponse.status,
        }
      } else {
        if (process.env.NODE_ENV !== 'production') {
          console.log('[ApiClient] ❌ Token refresh failed, redirecting to login')
        }
        if (typeof window !== 'undefined') {
          window.location.href = '/login'
        }
        throw new Error('Session expired, please login again')
      }
    }

    if (!response.ok) {
      throw new Error(`API Error: ${response.status} ${response.statusText}`)
    }

    const data = await response.json()

    return {
      data,
      status: response.status,
    }
  }

  setApiKey(apiKey: string) {
    this.config.apiKey = apiKey
  }

  setBearerToken(token: string) {
    this.config.bearerToken = token
  }
}

export const apiClient = new ApiClient()
