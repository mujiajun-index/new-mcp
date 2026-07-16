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
  // 商业化公开键(§15)
  billingEnabled: boolean
  displayCurrency: string
  selfUseModeEnabled: boolean
  redemptionEnabled: boolean
  userOwnedServicesEnabled: boolean
}

interface SystemConfigState {
  config: SystemConfig
  setConfig: (config: Partial<SystemConfig>) => void
  fetchPublicSettings: () => Promise<void>
}

const boolOr = (data: Record<string, unknown>, key: string, fallback: boolean): boolean =>
  data[key] === undefined ? fallback : data[key] === 'true' || data[key] === true

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
        // 商业化默认(与后端 model/option.go defaultOptions 一致)
        billingEnabled: false,
        displayCurrency: 'CNY',
        selfUseModeEnabled: false,
        redemptionEnabled: true,
        userOwnedServicesEnabled: true,
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
              registerEnabled: boolOr(data, 'RegisterEnabled', state.config.registerEnabled),
              emailVerificationEnabled: boolOr(data, 'EmailVerificationEnabled', state.config.emailVerificationEnabled),
              smtpConfigured: boolOr(data, 'SMTPConfigured', state.config.smtpConfigured),
              userGroupOptions: parseGroupOptions(data.UserGroupOptions),
              // 商业化
              billingEnabled: boolOr(data, 'BillingEnabled', state.config.billingEnabled),
              displayCurrency: (data.DisplayCurrency as string) ?? state.config.displayCurrency,
              selfUseModeEnabled: boolOr(data, 'SelfUseModeEnabled', state.config.selfUseModeEnabled),
              redemptionEnabled: boolOr(data, 'RedemptionEnabled', state.config.redemptionEnabled),
              userOwnedServicesEnabled: boolOr(data, 'UserOwnedServicesEnabled', state.config.userOwnedServicesEnabled),
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
