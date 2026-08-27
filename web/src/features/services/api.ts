import { api } from '@/lib/api'
import type {
  ServiceListParams,
  CreateServiceReq, UpdateServiceReq, PrepareStdioReq, ProcessControlAction,
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

export async function prepareStdio(data: PrepareStdioReq) {
  const res = await api.post('/services/prepare-stdio', data)
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

// 工具测试:后端 60s 超时,前端放宽到 65s 避免提前断开
export async function callServiceTool(id: number, data: { name: string; arguments: Record<string, unknown> }) {
  const res = await api.post(`/services/${id}/tools/call`, data, { timeout: 65_000 })
  return res.data
}

// 资源测试(resources/read)与提示测试(prompts/get),超时口径同工具测试
export async function readServiceResource(id: number, data: { uri: string }) {
  const res = await api.post(`/services/${id}/resources/read`, data, { timeout: 65_000 })
  return res.data
}

export async function callServicePrompt(id: number, data: { name: string; arguments: Record<string, string> }) {
  const res = await api.post(`/services/${id}/prompts/get`, data, { timeout: 65_000 })
  return res.data
}

export async function getServiceResources(id: number) {
  const res = await api.get(`/services/${id}/resources`)
  return res.data
}

export async function getServicePrompts(id: number) {
  const res = await api.get(`/services/${id}/prompts`)
  return res.data
}

export async function getServiceHealth(id: number) {
  const res = await api.get(`/services/${id}/health`)
  return res.data
}

// stdio 服务进程资源占用(详情页 5s 轮询)
export async function getServiceProcessStat(id: number) {
  const res = await api.get(`/services/${id}/process`)
  return res.data
}

// stdio 进程 启动/停止/重启:后端同步等待启动握手(npx 首拉可能要下载依赖),
// 超时口径与工具测试一致放宽到 65s
export async function controlServiceProcess(id: number, action: ProcessControlAction) {
  const res = await api.post(`/services/${id}/process/control`, { action }, { timeout: 65_000 })
  return res.data
}

// 服务总览(所有登录用户,按 user_id 只看自己的服务):统计摘要 + 全部服务资源
// 快照 + 非 stdio 健康状态条(总览页 5s 轮询)
export async function getServiceOverview() {
  const res = await api.get('/services/overview')
  return res.data
}
