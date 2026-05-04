import { api } from '@/lib/api'
import type { ListParams } from '@/types'

export async function getAdminStats() {
  const res = await api.get('/admin/stats')
  return res.data
}

export async function getAdminUsers(params?: ListParams) {
  const res = await api.get('/admin/users', { params })
  return res.data
}

export async function updateAdminUser(id: number, data: { status?: number; role?: string; email?: string }) {
  const res = await api.put(`/admin/users/${id}`, data)
  return res.data
}

export async function getAdminLogs(params?: ListParams) {
  const res = await api.get('/admin/logs', { params })
  return res.data
}
