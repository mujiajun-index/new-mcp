import { api } from '@/lib/api'
import type { KeysApi } from '@/components/service-keys-card'
import type {
  BatchGroupsTagsReq, BatchPricingReq, CloneMarketplaceReq, MarketplaceEntryPrice, ProcessControlAction,
  UpdateServiceKeysReq,
} from '@/types'

// 管理员市场项列表(全量,含未发布);status=0 不过滤,keyword 匹配标识/显示名/描述,
// category/group_id/tag 筛选与广场已发布列表同口径
export async function adminListMarketplace(params?: {
  page?: number
  page_size?: number
  status?: number
  keyword?: string
  category?: string
  group_id?: number
  tag?: string
}) {
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

// 条目进程视图(仅 stdio 条目):共享=平台唯一进程;独占=安装引用行服务端分页
// (page/page_size/username,username 匹配用户名/服务名;默认每页 18 条)
export async function adminGetMarketplaceProcess(
  id: number,
  params?: { page?: number; page_size?: number; username?: string },
) {
  const res = await api.get(`/admin/marketplace/${id}/process`, { params })
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

// 批量设置分组/标签(替换语义):PUT /admin/marketplace/groups-tags/batch
// group_ids/tags 均可选:缺省=不动该字段;[]=清空;两者须至少传一个
export async function adminBatchSetGroupsTags(data: BatchGroupsTagsReq) {
  const res = await api.put('/admin/marketplace/groups-tags/batch', data)
  return res.data
}

// 条目级定价全量替换(§5.2):PUT /admin/marketplace/:id/entry-prices。
// 载荷为期望的完整条目价列表,不在其中的条目回退(工具→服务价,资源/提示→免费)。
export async function adminSetEntryPrices(id: number, prices: MarketplaceEntryPrice[]) {
  const res = await api.put(`/admin/marketplace/${id}/entry-prices`, { prices })
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

// --- 条目级多秘钥管理(/admin/marketplace/:id/keys) ---
// 一份池对全部安装用户全局轮换;DTO 与服务级(/services/:id/keys)一致。

// 条目秘钥池视图(掩码值 + 模式 + 统计)
export async function adminGetMarketplaceKeys(id: number) {
  const res = await api.get(`/admin/marketplace/${id}/keys`)
  return res.data
}

// 更新条目秘钥:追加(去重保状态)/ 替换全部
export async function adminUpdateMarketplaceKeys(id: number, data: UpdateServiceKeysReq) {
  const res = await api.put(`/admin/marketplace/${id}/keys`, data)
  return res.data
}

// 条目模式切换:单↔多、随机↔轮询
export async function adminUpdateMarketplaceKeyConfig(id: number, data: { key_mode: 'single' | 'random' | 'polling'; header_name?: string }) {
  const res = await api.put(`/admin/marketplace/${id}/keys/config`, data)
  return res.data
}

// 启用/禁用单把条目秘钥
export async function adminSetMarketplaceKeyStatus(id: number, keyID: number, status: 'enabled' | 'disabled') {
  const res = await api.put(`/admin/marketplace/${id}/keys/${keyID}`, { status })
  return res.data
}

// 删除单把条目秘钥
export async function adminDeleteMarketplaceKey(id: number, keyID: number) {
  const res = await api.delete(`/admin/marketplace/${id}/keys/${keyID}`)
  return res.data
}

// 批量操作:全部启用 / 删除已禁用
export async function adminBatchMarketplaceKeys(id: number, action: 'enable_all' | 'delete_disabled') {
  const res = await api.post(`/admin/marketplace/${id}/keys/batch`, { action })
  return res.data
}

// marketplaceKeysApi 构造通用秘钥管理卡片(ServiceKeysCard)的条目级端点适配。
export function marketplaceKeysApi(id: number): KeysApi {
  return {
    list: () => adminGetMarketplaceKeys(id),
    updateKeys: (data) => adminUpdateMarketplaceKeys(id, data),
    updateConfig: (data) => adminUpdateMarketplaceKeyConfig(id, data),
    setKeyStatus: (keyID, status) => adminSetMarketplaceKeyStatus(id, keyID, status),
    deleteKey: (keyID) => adminDeleteMarketplaceKey(id, keyID),
    batch: (action) => adminBatchMarketplaceKeys(id, action),
  }
}
