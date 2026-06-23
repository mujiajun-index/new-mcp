import { api } from '@/lib/api'
import type {
  ServiceListParams,
  CreateServiceReq, UpdateServiceReq,
} from '@/types'

export async function getServices(params?: ServiceListParams) {
  const res = await api.get('/services', { params })
  return res.data
}

export async function getService(id: number) {
  const res = await api.get(`/services/${id}`)
  return res.data
}

export async function createService(data: CreateServiceReq) {
  const res = await api.post('/services', data)
  return res.data
}

export async function updateService(id: number, data: UpdateServiceReq) {
  const res = await api.put(`/services/${id}`, data)
  return res.data
}

export async function deleteService(id: number) {
  const res = await api.delete(`/services/${id}`)
  return res.data
}

export async function testService(id: number) {
  const res = await api.post(`/services/${id}/test`)
  return res.data
}

export async function testConnection(data: { transport_type: string; config: Record<string, unknown> }) {
  const res = await api.post('/services/test-connection', data)
  return res.data
}

export async function refreshTools(id: number) {
  const res = await api.post(`/services/${id}/refresh-tools`)
  return res.data
}

export async function getServiceTools(id: number) {
  const res = await api.get(`/services/${id}/tools`)
  return res.data
}

export async function getServiceHealth(id: number) {
  const res = await api.get(`/services/${id}/health`)
  return res.data
}
