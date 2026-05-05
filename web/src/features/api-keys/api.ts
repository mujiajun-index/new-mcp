import { api } from '@/lib/api'
import type { CreateApiKeyReq, UpdateApiKeyReq } from '@/types'

export async function getApiKeys() {
  const res = await api.get('/api-keys')
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
