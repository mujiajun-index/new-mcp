import { api } from '@/lib/api'
import type { CreateApiKeyReq, UpdateApiKeyReq } from '@/types'

export async function getApiKeys(keyword?: string) {
  const res = await api.get('/api-keys', { params: keyword ? { keyword } : {} })
  return res.data
}

export async function createApiKey(data: CreateApiKeyReq) {
  const res = await api.post('/api-keys', data)
  return res.data
}

export async function updateApiKey(id: number, data: UpdateApiKeyReq) {
  const res = await api.put(`/api-keys/${id}`, data)
  return res.data
}

export async function deleteApiKey(id: number) {
  const res = await api.delete(`/api-keys/${id}`)
  return res.data
}

export async function getApiKeyFullKey(id: number) {
  const res = await api.post(`/api-keys/${id}/key`)
  return res.data
}

export async function batchDeleteApiKeys(ids: number[]) {
  const res = await api.post('/api-keys/batch-delete', { ids })
  return res.data
}

export async function batchUpdateApiKeyStatus(ids: number[], status: number) {
  const res = await api.post('/api-keys/batch-status', { ids, status })
  return res.data
}
