import { api } from '@/lib/api'

// Mirrors controller.uploadListItem. The `url` is a short capability handle
// (/u/<sid>) stable for the row's lifetime. `expires_at` is an estimate
// (created_at + retention) for display only.
export interface UploadListItem {
  id: number
  user_id: number
  key: string
  url: string
  mime: string
  size: number
  backend: string
  status: string
  created_at: string
  expires_at: string
}

export interface UploadListResponse {
  success: boolean
  data: UploadListItem[]
  pagination: {
    page: number
    page_size: number
    total: number
    total_pages: number
  }
}

// --- Admin: all users' uploads ---

export async function adminGetUploads(
  params: { user_id?: number; page?: number; page_size?: number } = {},
): Promise<UploadListResponse> {
  const res = await api.get('/admin/vision/uploads', { params })
  return res.data
}

export async function adminDeleteUpload(id: number) {
  const res = await api.delete(`/admin/vision/uploads/${id}`)
  return res.data
}
