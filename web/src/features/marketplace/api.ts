import { api } from '@/lib/api'
import type { MarketplaceListParams } from '@/types'

export async function getMarketplaceItems(params?: MarketplaceListParams) {
  const res = await api.get('/marketplace', { params })
  return res.data
}

export async function getMarketplaceItem(id: number) {
  const res = await api.get(`/marketplace/${id}`)
  return res.data
}

// 启用分组(公开端点,供广场左侧筛选)
export async function getMarketplaceGroups() {
  const res = await api.get('/marketplace-groups')
  return res.data
}

// 引用式安装(D3/§11):把市场项添加为用户的引用服务(source=marketplace,空 config,平台托管)。
// 旧 POST /marketplace/install(复制配置)已废弃,改为 POST /marketplace/:id/add。
export async function addToMyServices(itemId: number) {
  const res = await api.post(`/marketplace/${itemId}/add`)
  return res.data
}

export async function createReview(itemId: number, data: { rating: number; review_text?: string }) {
  const res = await api.post(`/marketplace/${itemId}/review`, data)
  return res.data
}

