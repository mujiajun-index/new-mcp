import axios from 'axios'
import { toast } from 'sonner'
import i18n from '@/i18n/config'
import { useAuthStore } from '@/stores/auth-store'

const pendingRequests = new Map<string, AbortController>()

function getRequestKey(config: { method?: string; url?: string; params?: unknown }): string {
  return `${config.method}:${config.url}?${JSON.stringify(config.params)}`
}

export const api = axios.create({
  baseURL: '/api/v1',
  headers: { 'Cache-Control': 'no-store' },
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('newmcp-token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }

  if (config.method === 'get' && !(config as any).disableDuplicate) {
    const key = getRequestKey(config)
    if (pendingRequests.has(key)) {
      const controller = new AbortController()
      controller.abort()
      return { ...config, signal: controller.signal }
    }
    const controller = new AbortController()
    config.signal = controller.signal
    pendingRequests.set(key, controller)
  }
  return config
})

api.interceptors.response.use(
  (response) => {
    const key = getRequestKey(response.config)
    pendingRequests.delete(key)

    if ((response.config as any).skipErrorHandler) return response

    const { success, message } = response.data
    if (success === false) {
      toast.error(message || i18n.t('common.requestFailed'))
      return Promise.reject(new Error(message))
    }
    return response
  },
  (error) => {
    const status = error.response?.status
    const message = error.response?.data?.message

    if (status === 401) {
      const hasToken = !!localStorage.getItem('newmcp-token')
      if (hasToken) {
        useAuthStore.getState().auth.reset()
        localStorage.removeItem('newmcp-token')
        toast.error(i18n.t('common.sessionExpired'))
        window.location.href = '/sign-in'
      } else if (message) {
        toast.error(message)
      }
    } else if (message) {
      toast.error(message)
    } else if (!axios.isCancel(error)) {
      toast.error(i18n.t('common.networkError'))
    }
    return Promise.reject(error)
  }
)

export async function getSelf() {
  const res = await api.get('/auth/profile')
  return res.data
}

// 系统信息：当前版本与启动时间
export async function getSystemInfo() {
  const res = await api.get('/admin/system/info')
  return res.data
}

// 检查更新：后端代理请求 GitHub 最新 release
export async function checkSystemUpdate() {
  const res = await api.get('/admin/system/check-update')
  return res.data
}
