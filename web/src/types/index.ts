export interface User {
  id: number
  username: string
  email: string
  role: 'user' | 'admin'
  status: number
  created_at: string
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message: string
  data: T
}

export interface ListParams {
  page?: number
  page_size?: number
  keyword?: string
}

export interface PaginatedData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}
