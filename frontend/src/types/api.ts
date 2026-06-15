export interface ApiClientConfig {
  baseURL: string
  apiKey?: string
  bearerToken?: string
}

export interface RequestConfig {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  headers?: Record<string, string>
  body?: unknown
  params?: Record<string, string | number | boolean>
}

export interface ApiResponse<T = unknown> {
  data: T
  status: number
  message?: string
}
