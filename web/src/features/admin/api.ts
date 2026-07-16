import { api } from '@/lib/api'
import type { AdminCreateUserReq, AdminUpdateUserReq, AdminAdjustQuotaReq } from '@/types'

export async function getAdminStats() {
  const res = await api.get('/admin/stats')
  return res.data
}

export async function getAdminUsers(params?: { page?: number; page_size?: number; keyword?: string }) {
  const res = await api.get('/admin/users', { params })
  return res.data
}

export async function createAdminUser(data: AdminCreateUserReq) {
  const res = await api.post('/admin/users', data)
  return res.data
}

export async function updateAdminUser(id: number, data: AdminUpdateUserReq) {
  const res = await api.put(`/admin/users/${id}`, data)
  return res.data
}

export async function getAdminUserDetail(id: number) {
  const res = await api.get(`/admin/users/${id}`)
  return res.data
}

// 管理员调额(D13):POST /admin/users/:id/quota,mode=add/sub/set
export async function adjustUserQuota(id: number, data: AdminAdjustQuotaReq) {
  const res = await api.post(`/admin/users/${id}/quota`, data)
  return res.data
}


