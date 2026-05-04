import { api } from '@/lib/api'
import type { ListParams, CreateConnectionReq, UpdateConnectionReq } from '@/types'

export async function getConnections(params?: ListParams) {
  const res = await api.get('/connections', { params })
  return res.data
}

export async function getConnection(id: number) {
  const res = await api.get(`/connections/${id}`)
  return res.data
}

export async function createConnection(data: CreateConnectionReq) {
  const res = await api.post('/connections', data)
  return res.data
}

export async function updateConnection(id: number, data: UpdateConnectionReq) {
  const res = await api.put(`/connections/${id}`, data)
  return res.data
}

export async function deleteConnection(id: number) {
  const res = await api.delete(`/connections/${id}`)
  return res.data
}

export async function connectConnection(id: number) {
  const res = await api.post(`/connections/${id}/connect`)
  return res.data
}

export async function disconnectConnection(id: number) {
  const res = await api.post(`/connections/${id}/disconnect`)
  return res.data
}

export async function bindConnectionApiKey(id: number, apiKeyId: number) {
  const res = await api.put(`/connections/${id}/bind-apikey`, { api_key_id: apiKeyId })
  return res.data
}
