import { api } from '@/lib/api'

export interface VisionConfigListItem {
  id: number
  name: string
  provider: string
  model_name: string
  endpoint_url: string
  auto_register: boolean
  registered_service_id: number | null
  status: number
  created_at: string
}

export interface VisionConfigDetail {
  id: number
  name: string
  description: string
  provider: string
  model_name: string
  endpoint_url: string
  system_prompt: string
  max_tokens: number
  auto_register: boolean
  registered_service_id: number | null
  analyze_image_name: string
  analyze_image_desc: string
  describe_scene_name: string
  describe_scene_desc: string
  extra_config: string
  status: number
  created_at: string
  updated_at: string
}

export interface CreateVisionConfigReq {
  name: string
  description?: string
  provider: string
  model_name: string
  endpoint_url: string
  api_key: string
  system_prompt?: string
  max_tokens?: number
}

export interface UpdateVisionConfigReq {
  name?: string
  description?: string
  provider?: string
  model_name?: string
  endpoint_url?: string
  api_key?: string
  system_prompt?: string
  max_tokens?: number
  analyze_image_name?: string
  analyze_image_desc?: string
  describe_scene_name?: string
  describe_scene_desc?: string
  status?: number
}

export interface TestVisionReq {
  endpoint_url: string
  api_key: string
  model_name: string
}

export interface TestVisionResult {
  success: boolean
  result?: string
  error?: string
}

export async function getVisionConfigs() {
  const res = await api.get('/vision')
  return res.data
}

export async function getVisionConfig(id: number) {
  const res = await api.get(`/vision/${id}`)
  return res.data
}

export async function createVisionConfig(data: CreateVisionConfigReq) {
  const res = await api.post('/vision', data)
  return res.data
}

export async function updateVisionConfig(id: number, data: UpdateVisionConfigReq) {
  const res = await api.put(`/vision/${id}`, data)
  return res.data
}

export async function deleteVisionConfig(id: number) {
  const res = await api.delete(`/vision/${id}`)
  return res.data
}

export async function testVisionConfig(data: TestVisionReq) {
  const res = await api.post('/vision/test', data)
  return res.data
}

export async function enableVisionConfig(id: number) {
  const res = await api.post(`/vision/${id}/enable`)
  return res.data
}

export async function disableVisionConfig(id: number) {
  const res = await api.post(`/vision/${id}/disable`)
  return res.data
}
