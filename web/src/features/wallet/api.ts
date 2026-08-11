import { api } from '@/lib/api'
import type { RedeemReq, TransferAffQuotaReq } from '@/types'

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

// 我的邀请概览(邀请码/链接/奖励余额)
export async function getInviteOverview() {
  const res = await api.get('/invite/overview')
  return res.data
}

// 邀请奖励待提取余额转入钱包
export async function transferAffQuota(data: TransferAffQuotaReq) {
  const res = await api.post('/invite/transfer', data)
  return res.data
}
