export interface BaseEntity {
  id: string
  createdAt: string
  updatedAt: string
}

export type Status = 'active' | 'inactive' | 'pending' | 'error'

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
}

export interface ApiError {
  message: string
  code?: string
  details?: Record<string, unknown>
}
