import { api } from '@/lib/api'

export interface CameraListItem {
  id: number
  name: string
  vision_config_id: number | null
  vision_config_name: string
  auto_register: boolean
  registered_service_id: number | null
  streaming: boolean
  has_stream_key: boolean
  stream_key_expires_at: string
  status: number
  created_at: string
}

export interface CameraDetail {
  id: number
  name: string
  description: string
  vision_config_id: number | null
  vision_config_name: string
  auto_register: boolean
  registered_service_id: number | null
  capture_name: string
  capture_desc: string
  analyze_name: string
  analyze_desc: string
  extra_config: string
  streaming: boolean
  has_stream_key: boolean
  stream_key_expires_at: string
  status: number
  created_at: string
  updated_at: string
}

export interface CameraStreamKey {
  stream_key: string
  stream_url: string
  expires_at: string // 空串=永久
  has_key: boolean
}

export interface CreateCameraReq {
  name: string
  description?: string
  vision_config_id: number
}

export interface UpdateCameraReq {
  name?: string
  description?: string
  vision_config_id?: number
  capture_name?: string
  capture_desc?: string
  analyze_name?: string
  analyze_desc?: string
  status?: number
}

export async function getCameras() {
  const res = await api.get('/cameras')
  return res.data
}

export async function getCamera(id: number) {
  const res = await api.get(`/cameras/${id}`)
  return res.data
}

export async function createCamera(data: CreateCameraReq) {
  const res = await api.post('/cameras', data)
  return res.data
}

export async function updateCamera(id: number, data: UpdateCameraReq) {
  const res = await api.put(`/cameras/${id}`, data)
  return res.data
}

export async function deleteCamera(id: number) {
  const res = await api.delete(`/cameras/${id}`)
  return res.data
}

export async function enableCamera(id: number) {
  const res = await api.post(`/cameras/${id}/enable`)
  return res.data
}

export async function disableCamera(id: number) {
  const res = await api.post(`/cameras/${id}/disable`)
  return res.data
}

export async function getCameraStreamKey(id: number): Promise<CameraStreamKey> {
  const res = await api.get(`/cameras/${id}/stream-key`)
  return res.data.data
}

export async function generateCameraStreamKey(id: number, expiresIn: number): Promise<CameraStreamKey> {
  const res = await api.post(`/cameras/${id}/stream-key`, { expires_in: expiresIn })
  return res.data.data
}

export async function revokeCameraStreamKey(id: number) {
  const res = await api.delete(`/cameras/${id}/stream-key`)
  return res.data
}
