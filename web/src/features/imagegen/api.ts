import { api } from '@/lib/api'
import type { ToolCallResult } from '@/types'

export interface ImageGenConfig {
  id: number
  name: string
  description: string
  provider: 'openai' | 'gemini'
  model_name: string
  endpoint_url: string
  operation: 'generate' | 'edit' | 'both'
  default_params: Record<string, unknown>
  timeout_seconds: number
  generate_image_desc: string
  edit_image_desc: string
  generate_image_desc_default: string
  edit_image_desc_default: string
  upload_image_desc: string
  has_api_key: boolean
  auto_register: boolean
  registered_service_id: number | null
  status: number
  created_at: string
}
export type ImageGenConfigListItem = ImageGenConfig
export type ConfigInput = Partial<
  Pick<
    ImageGenConfig,
    | 'name'
    | 'description'
    | 'provider'
    | 'model_name'
    | 'endpoint_url'
    | 'operation'
    | 'default_params'
    | 'timeout_seconds'
    | 'generate_image_desc'
    | 'edit_image_desc'
  >
> & { api_key?: string }
export interface ProbeInput extends ConfigInput {
  config_id?: number
  tool?: string
  arguments?: Record<string, unknown>
}
export async function getImageGenConfigs(): Promise<{
  data: ImageGenConfig[]
}> {
  return (await api.get('/imagegen')).data
}
export async function getImageGenConfig(
  id: number
): Promise<{ data: ImageGenConfig }> {
  return (await api.get(`/imagegen/${id}`)).data
}
export async function createImageGenConfig(
  data: ConfigInput
): Promise<{ data: ImageGenConfig }> {
  return (await api.post('/imagegen', data)).data
}
export async function updateImageGenConfig(id: number, data: ConfigInput) {
  return (await api.put(`/imagegen/${id}`, data)).data
}
export async function deleteImageGenConfig(id: number) {
  return (await api.delete(`/imagegen/${id}`)).data
}
export async function enableImageGenConfig(id: number) {
  return (await api.post(`/imagegen/${id}/enable`)).data
}
export async function disableImageGenConfig(id: number) {
  return (await api.post(`/imagegen/${id}/disable`)).data
}
export async function listImageGenModels(
  data: ProbeInput
): Promise<{ data: Array<{ id: string; name: string }> }> {
  return (await api.post('/imagegen/models', data, { timeout: 20_000 })).data
}
export async function testImageGenConfig(
  data: ProbeInput
): Promise<{ data: ToolCallResult }> {
  const seconds = data.timeout_seconds ?? 180
  return (
    await api.post('/imagegen/test', data, {
      timeout: seconds === 0 ? 0 : (seconds + 5) * 1000,
    })
  ).data
}

export function imageEndpoint(
  base: string,
  provider: string,
  model: string,
  operation = 'generate'
) {
  const root = base
    .trim()
    .replace(/\/+$/, '')
    .replace(/\/(v1|v1beta)$/, '')
  return provider === 'gemini'
    ? `${root}/v1beta/models/${encodeURIComponent(model.replace(/^models\//, '') || '{model}')}:generateContent`
    : `${root}/v1/images/${operation === 'edit' ? 'edits' : 'generations'}`
}
