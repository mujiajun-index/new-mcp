import { api } from '@/lib/api'
import type { RedemptionCreateReq } from '@/types'

export async function listRedemptions(params?: {
  page?: number
  page_size?: number
  keyword?: string
  status?: number
}) {
  const res = await api.get('/admin/redemptions', { params })
  return res.data
}

export async function createRedemptions(data: RedemptionCreateReq) {
  const res = await api.post('/admin/redemptions', data)
  return res.data
}

export async function updateRedemptionStatus(id: number, status: number) {
  const res = await api.put(`/admin/redemptions/${id}`, { status })
  return res.data
}

export async function deleteRedemption(id: number) {
  const res = await api.delete(`/admin/redemptions/${id}`)
  return res.data
}
