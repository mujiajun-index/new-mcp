import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { api } from '@/lib/api'

interface SystemConfig {
  systemName: string
  serverAddress: string
  footer: string
}

interface SystemConfigState {
  config: SystemConfig
  setConfig: (config: Partial<SystemConfig>) => void
  fetchPublicSettings: () => Promise<void>
}

export const useSystemConfigStore = create<SystemConfigState>()(
  persist(
    (set) => ({
      config: {
        systemName: 'NewMCP',
        serverAddress: 'http://localhost:3000',
        footer: '',
      },
      setConfig: (partial) =>
        set((state) => ({
          config: { ...state.config, ...partial },
        })),
      fetchPublicSettings: async () => {
        try {
          const res = await api.get('/settings/public', { skipErrorHandler: true } as any)
          const data = res.data?.data || {}
          set((state) => ({
            config: {
              ...state.config,
              systemName: data.SystemName ?? state.config.systemName,
              serverAddress: data.ServerAddress ?? state.config.serverAddress,
              footer: data.Footer ?? state.config.footer,
            },
          }))
        } catch {
          // silently fallback to persisted / default values
        }
      },
    }),
    { name: 'newmcp-system-config' }
  )
)
