import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface SystemConfig {
  systemName: string
}

interface SystemConfigState {
  config: SystemConfig
  setConfig: (config: Partial<SystemConfig>) => void
}

export const useSystemConfigStore = create<SystemConfigState>()(
  persist(
    (set) => ({
      config: { systemName: 'NewMCP' },
      setConfig: (partial) =>
        set((state) => ({
          config: { ...state.config, ...partial },
        })),
    }),
    { name: 'newmcp-system-config' }
  )
)
