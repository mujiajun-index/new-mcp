import { api } from '@/lib/api'
import type {
  BatchPricingReq, CloneMarketplaceReq, ProcessControlAction,
} from '@/types'

// 管理员市场项列表(全量,含未发布)
export async function adminListMarketplace(params?: { page?: number; page_size?: number }) {
  const res = await api.get('/admin/marketplace', { params })
  return res.data
}

// 平台级健康:全部条目同条目下全部用户引用行的真实调用聚合(30s 缓存,30s 轮询对齐)
export async function adminGetMarketplaceHealth() {
  const res = await api.get('/admin/marketplace/health')
  return res.data
}

export async function adminGetMarketplace(id: number) {
  const res = await api.get(`/admin/marketplace/${id}`)
  return res.data
}

export async function adminUpdateMarketplace(id: number, data: Record<string, unknown>) {
  const res = await api.put(`/admin/marketplace/${id}`, data)
  return res.data
}

export async function adminDeleteMarketplace(id: number) {
  const res = await api.delete(`/admin/marketplace/${id}`)
  return res.data
}

// 手动刷新快照(仅平台托管项):POST /admin/marketplace/:id/refresh
export async function adminRefreshMarketplace(id: number) {
  const res = await api.post(`/admin/marketplace/${id}/refresh`)
  return res.data
}

// 条目进程视图(仅 stdio 条目):共享=平台唯一进程;独占=按安装用户逐行枚举
export async function adminGetMarketplaceProcess(id: number) {
  const res = await api.get(`/admin/marketplace/${id}/process`)
  return res.data
}

// 条目进程启停:共享模式忽略 serviceId(start=预热);独占模式指定目标安装引用行
export async function adminControlMarketplaceProcess(
  id: number,
  action: ProcessControlAction,
  serviceId?: number,
) {
  const res = await api.post(`/admin/marketplace/${id}/process/control`, {
    action,
    service_id: serviceId ?? 0,
  })
  return res.data
}

// 批量定价(§5.5):PUT /admin/marketplace/pricing/batch
export async function adminBatchPricing(data: BatchPricingReq) {
  const res = await api.put('/admin/marketplace/pricing/batch', data)
  return res.data
}

// 从自有服务克隆上架(D14):POST /admin/marketplace/clone
export async function adminCloneMarketplace(data: CloneMarketplaceReq) {
  const res = await api.post('/admin/marketplace/clone', data)
  return res.data
}

// 可克隆来源服务列表(克隆来源选择用):GET /admin/marketplace/clone-sources
export async function adminListCloneSources() {
  const res = await api.get('/admin/marketplace/clone-sources')
  return res.data
}
