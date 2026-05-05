import { api } from '@/lib/api'
import type { LogFilter } from '@/types'

export async function getUserLogs(params?: LogFilter) {
  const res = await api.get('/logs', { params })
  return res.data
}

export async function getUserLogStats(params?: LogFilter) {
  const res = await api.get('/logs/stats', { params })
  return res.data
}
