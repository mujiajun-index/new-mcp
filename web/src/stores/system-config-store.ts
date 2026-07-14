import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { api } from '@/lib/api'

// parseGroupOptions splits the comma-separated "UserGroupOptions" setting into
// a deduplicated list. Always returns at least ["default"].
function parseGroupOptions(raw: unknown): string[] {
  if (typeof raw !== 'string' || raw.trim() === '') return ['default']
  const seen = new Set<string>()
  const opts: string[] = []
  for (const part of raw.split(',')) {
    const name = part.trim()
    if (name && !seen.has(name)) {
      seen.add(name)
      opts.push(name)
    }
  }
  return opts.length ? opts : ['default']
}

interface SystemConfig {
  systemName: string
  serverAddress: string
  footer: string
  registerEnabled: boolean
  emailVerificationEnabled: boolean
  smtpConfigured: boolean
  userGroupOptions: string[]
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
        registerEnabled: true,
        emailVerificationEnabled: false,
        smtpConfigured: false,
        userGroupOptions: ['default', 'vip', 'svip'],
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
              registerEnabled: data.RegisterEnabled === undefined ? state.config.registerEnabled : data.RegisterEnabled === 'true',
              emailVerificationEnabled: data.EmailVerificationEnabled === 'true',
              smtpConfigured: data.SMTPConfigured === 'true',
              userGroupOptions: parseGroupOptions(data.UserGroupOptions),
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
