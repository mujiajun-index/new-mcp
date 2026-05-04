import { api } from '@/lib/api'
import type { MarketplaceListParams, InstallReq } from '@/types'

export async function getMarketplaceItems(params?: MarketplaceListParams) {
  const res = await api.get('/marketplace', { params })
  return res.data
}

export async function getMarketplaceItem(id: number) {
  const res = await api.get(`/marketplace/${id}`)
  return res.data
}

export async function installFromMarketplace(data: InstallReq) {
  const res = await api.post('/marketplace/install', data)
  return res.data
}

export async function createReview(itemId: number, data: { rating: number; review_text?: string }) {
  const res = await api.post(`/marketplace/${itemId}/review`, data)
  return res.data
}
