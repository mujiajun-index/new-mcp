import { api } from '@/lib/api'
import type { RedeemReq } from '@/types'

// 我的额度概览
export async function getWalletOverview() {
  const res = await api.get('/wallet')
  return res.data
}

// 用量统计(今日/本周/累计消费 quota)
export async function getWalletUsageStats() {
  const res = await api.get('/wallet/usage/stats')
  return res.data
}

// 兑换码兑换
export async function redeemCode(data: RedeemReq) {
  const res = await api.post('/redemptions/redeem', data)
  return res.data
}
