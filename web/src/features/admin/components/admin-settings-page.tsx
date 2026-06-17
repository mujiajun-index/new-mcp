import { useState, useEffect, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { api, getSystemInfo, checkSystemUpdate } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from 'sonner'
import {
  Settings,
  Shield,
  Activity,
  Mail,
  Wrench,
  Plus,
  Trash2,
  RefreshCw,
  ExternalLink,
} from 'lucide-react'

interface SettingItem {
  key: string
  value: string
}

interface ReleaseInfo {
  tag_name: string
  name?: string
  body?: string
  html_url?: string
  published_at?: string
}

async function getAdminSettings(): Promise<Record<string, string>> {
  const res = await api.get('/admin/settings')
  const items: SettingItem[] = res.data.data
  const map: Record<string, string> = {}
  for (const item of items) {
    map[item.key] = item.value
  }
  return map
}

async function updateSetting(key: string, value: string) {
  const res = await api.put('/admin/settings', { key, value })
  return res.data
}

interface RateLimitGroup {
  max: number
  window: number
}

function parseGroupConfig(json: string): Record<string, RateLimitGroup> {
  try {
    return JSON.parse(json || '{}')
  } catch {
    return {}
  }
}

export function AdminSettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: settings, isLoading } = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getAdminSettings,
  })

  // 系统信息：当前版本与启动时间（独立查询，避免与可编辑设置耦合）
  const { data: systemInfoData } = useQuery({
    queryKey: ['admin-system-info'],
    queryFn: getSystemInfo,
    staleTime: 60_000,
  })
  const systemInfo = systemInfoData?.data
  const currentVersion = systemInfo?.version ?? 'v0.0.0'
  const startTime = systemInfo?.start_time

  const [release, setRelease] = useState<ReleaseInfo | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)

  const checkUpdateMutation = useMutation({
    mutationFn: checkSystemUpdate,
    onSuccess: (res) => {
      const data = res?.data
      if (!data?.has_release || !data.release) {
        // 仓库尚无任何 release
        toast.info(t('settings.noReleases'))
        return
      }
      const rel = data.release
      if (rel.tag_name && rel.tag_name === currentVersion) {
        toast.success(t('settings.upToDate', { version: rel.tag_name }))
        return
      }
      setRelease(rel)
      setDialogOpen(true)
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) => updateSetting(key, value),
    onSuccess: () => {
      toast.success(t('settings.saveSuccess'))
      queryClient.invalidateQueries({ queryKey: ['admin-settings'] })
    },
    onError: () => toast.error(t('settings.saveFailed')),
  })

  const [localValues, setLocalValues] = useState<Record<string, string>>({})

  useEffect(() => {
    if (settings) {
      setLocalValues({ ...settings })
    }
  }, [settings])

  const saveField = useCallback(
    (key: string) => {
      const newValue = localValues[key] ?? ''
      const oldValue = settings?.[key] ?? ''
      if (newValue === oldValue) return
      updateMutation.mutate({ key, value: newValue })
    },
    [localValues, settings, updateMutation]
  )

  const updateLocal = (key: string, value: string) => {
    setLocalValues((prev) => ({ ...prev, [key]: value }))
  }

  const toggleBool = (key: string) => {
    const current = localValues[key] === 'true'
    const next = !current
    updateMutation.mutate({ key, value: String(next) })
  }

  // Rate limit group config helpers
  const [groupConfig, setGroupConfig] = useState<Record<string, RateLimitGroup>>({})
  const [newGroupName, setNewGroupName] = useState('')

  useEffect(() => {
    if (localValues.RateLimitGroupConfig) {
      setGroupConfig(parseGroupConfig(localValues.RateLimitGroupConfig))
    }
  }, [localValues.RateLimitGroupConfig])

  const saveGroupConfig = (config: Record<string, RateLimitGroup>) => {
    setGroupConfig(config)
    const json = JSON.stringify(config)
    updateLocal('RateLimitGroupConfig', json)
    updateMutation.mutate({ key: 'RateLimitGroupConfig', value: json })
  }

  const addGroup = () => {
    const name = newGroupName.trim()
    if (!name || groupConfig[name]) return
    const config = { ...groupConfig, [name]: { max: 60, window: 1 } }
    saveGroupConfig(config)
    setNewGroupName('')
  }

  const removeGroup = (name: string) => {
    const config = { ...groupConfig }
    delete config[name]
    saveGroupConfig(config)
  }

  const updateGroup = (name: string, field: 'max' | 'window', value: number) => {
    const config = { ...groupConfig, [name]: { ...groupConfig[name], [field]: value } }
    saveGroupConfig(config)
  }

  if (isLoading) {
    return (
      <div className="p-6 lg:p-8">
        <p className="text-sm text-muted-foreground">{t('common.loading')}</p>
      </div>
    )
  }

  const startTimeStr = startTime ? dayjs.unix(startTime).format('YYYY-MM-DD HH:mm:ss') : '-'

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('settings.title')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('settings.subtitle')}</p>
      </div>

      <Tabs defaultValue="general">
        <TabsList>
          <TabsTrigger value="general" className="gap-1.5">
            <Settings className="h-3.5 w-3.5" />
            {t('settings.general')}
          </TabsTrigger>
          <TabsTrigger value="auth" className="gap-1.5">
            <Shield className="h-3.5 w-3.5" />
            {t('settings.auth')}
          </TabsTrigger>
          <TabsTrigger value="rateLimit" className="gap-1.5">
            <Activity className="h-3.5 w-3.5" />
            {t('settings.rateLimit')}
          </TabsTrigger>
          <TabsTrigger value="smtp" className="gap-1.5">
            <Mail className="h-3.5 w-3.5" />
            {t('settings.smtp')}
          </TabsTrigger>
          <TabsTrigger value="maintenance" className="gap-1.5">
            <Wrench className="h-3.5 w-3.5" />
            {t('settings.maintenance')}
          </TabsTrigger>
        </TabsList>

        {/* General */}
        <TabsContent value="general">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <h2 className="text-sm font-semibold">{t('settings.general')}</h2>
            <p className="text-xs text-muted-foreground">{t('settings.generalDesc')}</p>

            <div className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.systemName')}</label>
                <Input
                  value={localValues.SystemName ?? ''}
                  onChange={(e) => updateLocal('SystemName', e.target.value)}
                  onBlur={() => saveField('SystemName')}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.serverAddress')}</label>
                <Input
                  placeholder="https://example.com"
                  value={localValues.ServerAddress ?? ''}
                  onChange={(e) => updateLocal('ServerAddress', e.target.value)}
                  onBlur={() => saveField('ServerAddress')}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.footer')}</label>
                <Input
                  value={localValues.Footer ?? ''}
                  onChange={(e) => updateLocal('Footer', e.target.value)}
                  onBlur={() => saveField('Footer')}
                />
              </div>
            </div>
          </div>
        </TabsContent>

        {/* Auth */}
        <TabsContent value="auth">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <h2 className="text-sm font-semibold">{t('settings.auth')}</h2>
            <p className="text-xs text-muted-foreground">{t('settings.authDesc')}</p>

            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium">{t('settings.registerEnabled')}</p>
                  <p className="text-xs text-muted-foreground">{t('settings.registerEnabledDesc')}</p>
                </div>
                <Switch
                  checked={localValues.RegisterEnabled === 'true'}
                  onCheckedChange={() => toggleBool('RegisterEnabled')}
                />
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium">{t('settings.emailVerification')}</p>
                  <p className="text-xs text-muted-foreground">{t('settings.emailVerificationDesc')}</p>
                </div>
                <Switch
                  checked={localValues.EmailVerificationEnabled === 'true'}
                  onCheckedChange={() => toggleBool('EmailVerificationEnabled')}
                />
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium">{t('settings.emailDomainRestriction')}</p>
                  <p className="text-xs text-muted-foreground">{t('settings.emailDomainRestrictionDesc')}</p>
                </div>
                <Switch
                  checked={localValues.EmailDomainRestrictionEnabled === 'true'}
                  onCheckedChange={() => toggleBool('EmailDomainRestrictionEnabled')}
                />
              </div>
              {localValues.EmailDomainRestrictionEnabled === 'true' && (
                <div className="space-y-2">
                  <label className="text-sm font-medium">{t('settings.emailDomainWhitelist')}</label>
                  <Input
                    placeholder={t('settings.emailDomainWhitelistPlaceholder')}
                    value={localValues.EmailDomainWhitelist ?? ''}
                    onChange={(e) => updateLocal('EmailDomainWhitelist', e.target.value)}
                    onBlur={() => saveField('EmailDomainWhitelist')}
                  />
                </div>
              )}
            </div>
          </div>
        </TabsContent>

        {/* Rate Limit */}
        <TabsContent value="rateLimit">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <h2 className="text-sm font-semibold">{t('settings.rateLimit')}</h2>
            <p className="text-xs text-muted-foreground">{t('settings.rateLimitDesc')}</p>

            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium">{t('settings.rateLimitEnabled')}</p>
                </div>
                <Switch
                  checked={localValues.RateLimitEnabled === 'true'}
                  onCheckedChange={() => toggleBool('RateLimitEnabled')}
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">{t('settings.maxRequests')}</label>
                  <Input
                    type="number"
                    value={localValues.RateLimitMaxRequests ?? '60'}
                    onChange={(e) => updateLocal('RateLimitMaxRequests', e.target.value)}
                    onBlur={() => saveField('RateLimitMaxRequests')}
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{t('settings.windowMinutes')}</label>
                  <Input
                    type="number"
                    value={localValues.RateLimitWindowMinutes ?? '1'}
                    onChange={(e) => updateLocal('RateLimitWindowMinutes', e.target.value)}
                    onBlur={() => saveField('RateLimitWindowMinutes')}
                  />
                </div>
              </div>

              {/* Group-level rate limits */}
              <div className="space-y-3 pt-2 border-t">
                <h3 className="text-sm font-semibold">{t('settings.groupRateLimits')}</h3>
                <p className="text-xs text-muted-foreground">{t('settings.groupRateLimitsDesc')}</p>

                <div className="space-y-2">
                  {Object.entries(groupConfig).map(([name, cfg]) => (
                    <div key={name} className="flex items-center gap-2">
                      <Input
                        value={name}
                        disabled
                        className="w-32 bg-muted"
                      />
                      <Input
                        type="number"
                        value={cfg.max}
                        onChange={(e) => updateGroup(name, 'max', Number(e.target.value))}
                        className="w-24"
                        placeholder={t('settings.maxRequests')}
                      />
                      <span className="text-xs text-muted-foreground">{t('settings.requestsPer')}</span>
                      <Input
                        type="number"
                        value={cfg.window}
                        onChange={(e) => updateGroup(name, 'window', Number(e.target.value))}
                        className="w-20"
                        placeholder={t('settings.minutes')}
                      />
                      <Button variant="ghost" size="icon" onClick={() => removeGroup(name)}>
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </div>
                  ))}
                  <div className="flex items-center gap-2">
                    <Input
                      value={newGroupName}
                      onChange={(e) => setNewGroupName(e.target.value)}
                      className="w-32"
                      placeholder={t('settings.groupName')}
                      onKeyDown={(e) => e.key === 'Enter' && addGroup()}
                    />
                    <Button variant="outline" size="sm" onClick={addGroup} disabled={!newGroupName.trim()}>
                      <Plus className="h-4 w-4 mr-1" />
                      {t('settings.addGroup')}
                    </Button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </TabsContent>

        {/* SMTP */}
        <TabsContent value="smtp">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <h2 className="text-sm font-semibold">{t('settings.smtp')}</h2>
            <p className="text-xs text-muted-foreground">{t('settings.smtpDesc')}</p>

            <div className="space-y-4">
              <div className="grid grid-cols-3 gap-4">
                <div className="col-span-2 space-y-2">
                  <label className="text-sm font-medium">{t('settings.smtpServer')}</label>
                  <Input
                    placeholder="smtp.example.com"
                    value={localValues.SMTPServer ?? ''}
                    onChange={(e) => updateLocal('SMTPServer', e.target.value)}
                    onBlur={() => saveField('SMTPServer')}
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{t('settings.smtpPort')}</label>
                  <Input
                    type="number"
                    value={localValues.SMTPPort ?? '465'}
                    onChange={(e) => updateLocal('SMTPPort', e.target.value)}
                    onBlur={() => saveField('SMTPPort')}
                  />
                </div>
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.smtpAccount')}</label>
                <Input
                  value={localValues.SMTPAccount ?? ''}
                  onChange={(e) => updateLocal('SMTPAccount', e.target.value)}
                  onBlur={() => saveField('SMTPAccount')}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.smtpToken')}</label>
                <Input
                  type="password"
                  placeholder="***"
                  onChange={(e) => updateLocal('SMTPToken', e.target.value)}
                  onBlur={() => saveField('SMTPToken')}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.smtpFrom')}</label>
                <Input
                  placeholder="noreply@example.com"
                  value={localValues.SMTPFrom ?? ''}
                  onChange={(e) => updateLocal('SMTPFrom', e.target.value)}
                  onBlur={() => saveField('SMTPFrom')}
                />
              </div>
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium">{t('settings.smtpSSL')}</p>
                <Switch
                  checked={localValues.SMTPSSLEnabled === 'true'}
                  onCheckedChange={() => toggleBool('SMTPSSLEnabled')}
                />
              </div>
            </div>
          </div>
        </TabsContent>

        {/* Maintenance */}
        <TabsContent value="maintenance">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <h2 className="text-sm font-semibold">{t('settings.maintenance')}</h2>
            <p className="text-xs text-muted-foreground">{t('settings.maintenanceDesc')}</p>

            <div className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="rounded-lg bg-muted/50 p-4">
                  <p className="text-xs text-muted-foreground">{t('settings.version')}</p>
                  <p className="mt-1 text-lg font-semibold">{currentVersion}</p>
                </div>
                <div className="rounded-lg bg-muted/50 p-4">
                  <p className="text-xs text-muted-foreground">{t('settings.startTime')}</p>
                  <p className="mt-1 text-lg font-semibold tabular-nums">{startTimeStr}</p>
                </div>
              </div>

              <Button
                variant="outline"
                onClick={() => checkUpdateMutation.mutate()}
                disabled={checkUpdateMutation.isPending}
              >
                <RefreshCw className="h-4 w-4 mr-1.5" />
                {checkUpdateMutation.isPending
                  ? t('settings.checking')
                  : t('settings.checkUpdate')}
              </Button>
            </div>
          </div>

          <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
            <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>
                  {t('settings.newVersionAvailable', { version: release?.tag_name ?? '' })}
                </DialogTitle>
                {release?.published_at && (
                  <DialogDescription>
                    {t('settings.publishedAt')}{' '}
                    {dayjs(release.published_at).format('YYYY-MM-DD HH:mm')}
                  </DialogDescription>
                )}
              </DialogHeader>

              <div className="space-y-3">
                {release?.body ? (
                  <pre className="text-sm whitespace-pre-wrap break-words font-sans bg-muted/40 rounded-md p-3">
                    {release.body}
                  </pre>
                ) : (
                  <p className="text-sm text-muted-foreground">{t('settings.noReleaseNotes')}</p>
                )}
              </div>

              <DialogFooter>
                <Button variant="outline" onClick={() => setDialogOpen(false)}>
                  {t('common.close')}
                </Button>
                {release?.html_url && (
                  <Button
                    onClick={() =>
                      window.open(release.html_url, '_blank', 'noopener,noreferrer')
                    }
                  >
                    <ExternalLink className="h-4 w-4 mr-1.5" />
                    {t('settings.openRelease')}
                  </Button>
                )}
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </TabsContent>
      </Tabs>
    </div>
  )
}
