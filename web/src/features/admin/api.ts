import { api } from '@/lib/api'
import type { AdminCreateUserReq, AdminUpdateUserReq } from '@/types'

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

