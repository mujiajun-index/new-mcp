import { api } from '@/lib/api'

// Mirrors controller.uploadListItem. The `url` is a short capability handle
// (/u/<sid>) stable for the row's lifetime. `expires_at` is an estimate
// (created_at + retention) for display only.
export interface UploadListItem {
  id: number
  user_id: number
  short_id: string
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

export interface BatchDeleteResponse {
  success: boolean
  deleted: number
  failed: number
}

// Batch delete (collection-level DELETE with an ids body). Non-owned or missing
// ids count as `failed` server-side (no existence leak).
export async function adminBatchDeleteUploads(ids: number[]): Promise<BatchDeleteResponse> {
  const res = await api.delete('/admin/vision/uploads', { data: { ids } })
  return res.data
}

// --- Admin: transform preview (what actually gets sent upstream) ---

export interface UploadPreviewImageInfo {
  width: number
  height: number
  size: number
  mime: string
}

export interface UploadPreviewSettings {
  resize_enabled: boolean
  resize_max_edge: number
  compress_enabled: boolean
  jpeg_quality: number
}

// Mirrors controller.uploadPreviewResponse. `unchanged` is true when the optimized
// bytes equal the original (settings off / image already small / GIF / fail-open);
// then `data_url` is empty and the caller renders the row's own signed URL.
export interface UploadPreviewResponse {
  settings: UploadPreviewSettings
  original: UploadPreviewImageInfo
  optimized: UploadPreviewImageInfo
  unchanged: boolean
  data_url: string
}

// Dry-run: apply the CURRENT read-path transform settings to one stored image and
// return before/after stats + the optimized image as a data URL. Never writes back.
export async function adminPreviewUpload(id: number): Promise<UploadPreviewResponse> {
  const res = await api.get(`/admin/vision/uploads/${id}/preview`)
  return res.data
}
