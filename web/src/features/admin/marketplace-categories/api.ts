import { api } from '@/lib/api'

// --- 市场分组(业务分类) ---
export async function adminListMarketplaceGroups() {
  const res = await api.get('/admin/marketplace-groups/all')
  return res.data
}
export async function adminCreateMarketplaceGroup(data: Record<string, unknown>) {
  const res = await api.post('/admin/marketplace-groups', data)
  return res.data
}
export async function adminUpdateMarketplaceGroup(id: number, data: Record<string, unknown>) {
  const res = await api.put(`/admin/marketplace-groups/${id}`, data)
  return res.data
}
export async function adminDeleteMarketplaceGroup(id: number) {
  const res = await api.delete(`/admin/marketplace-groups/${id}`)
  return res.data
}

// --- 市场标签(字典) ---
export async function adminListMarketplaceTags(params?: { page?: number; page_size?: number; status?: number }) {
  const res = await api.get('/admin/marketplace-tags', { params })
  return res.data
}
export async function adminCreateMarketplaceTag(data: Record<string, unknown>) {
  const res = await api.post('/admin/marketplace-tags', data)
  return res.data
}
export async function adminUpdateMarketplaceTag(id: number, data: Record<string, unknown>) {
  const res = await api.put(`/admin/marketplace-tags/${id}`, data)
  return res.data
}
export async function adminDeleteMarketplaceTag(id: number) {
  const res = await api.delete(`/admin/marketplace-tags/${id}`)
  return res.data
}
