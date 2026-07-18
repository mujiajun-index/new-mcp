import { api } from '@/lib/api'
import type {
  BatchPricingReq, CloneMarketplaceReq,
} from '@/types'

// 管理员市场项列表(全量,含未发布)
export async function adminListMarketplace(params?: { page?: number; page_size?: number }) {
  const res = await api.get('/admin/marketplace', { params })
  return res.data
}

export async function adminGetMarketplace(id: number) {
  const res = await api.get(`/admin/marketplace/${id}`)
  return res.data
}

export async function adminCreateMarketplace(data: Record<string, unknown>) {
  const res = await api.post('/admin/marketplace', data)
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
