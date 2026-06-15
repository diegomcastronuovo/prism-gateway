import { apiClient } from './client'

export async function fetcher<T>(url: string): Promise<T> {
  const response = await apiClient.request<T>(url)
  return response.data
}

export async function mutationFetcher<T, D = unknown>(
  url: string,
  data: D,
  method: 'POST' | 'PUT' | 'PATCH' | 'DELETE' = 'POST'
): Promise<T> {
  const response = await apiClient.request<T>(url, {
    method,
    body: data,
  })
  return response.data
}
