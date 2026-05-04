import axios from 'axios'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'

const pendingRequests = new Map<string, AbortController>()

function getRequestKey(config: { method?: string; url?: string; params?: unknown }): string {
  return `${config.method}:${config.url}?${JSON.stringify(config.params)}`
}

export const api = axios.create({
  baseURL: '',
  withCredentials: true,
  headers: { 'Cache-Control': 'no-store' },
})

api.interceptors.request.use((config) => {
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
      toast.error(message || '请求失败')
      return Promise.reject(new Error(message))
    }
    return response
  },
  (error) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().auth.reset()
      toast.error('会话已过期')
      window.location.href = '/sign-in'
    }
    return Promise.reject(error)
  }
)

export async function getSelf() {
  const res = await api.get('/api/user/self', { skipErrorHandler: true } as any)
  return res.data
}
