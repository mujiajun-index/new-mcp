import { useState, useEffect, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { toast } from 'sonner'
import { CreditCard, Coins, ScrollText, SlidersHorizontal, Info } from 'lucide-react'

interface SettingItem { key: string; value: string }

async function getAdminSettings(): Promise<Record<string, string>> {
  const res = await api.get('/admin/settings')
  const items: SettingItem[] = res.data.data
  const map: Record<string, string> = {}
  for (const item of items) map[item.key] = item.value
  return map
}

function updateSetting(key: string, value: string) {
  return api.put('/admin/settings', { key, value })
}

function parseRatio(json: string): Record<string, number> {
  try {
    const obj = JSON.parse(json || '{}')
    const out: Record<string, number> = {}
    for (const [k, v] of Object.entries(obj)) out[k] = Number(v) || 0
    return out
  } catch {
    return {}
  }
}

export function AdminBillingPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fetchPublicSettings = useSystemConfigStore((s) => s.fetchPublicSettings)
  const userGroupOptions = useSystemConfigStore((s) => s.config.userGroupOptions)

  const { data: settings, isLoading } = useQuery({
    queryKey: ['admin-settings'],
    queryFn: getAdminSettings,
  })

  const [localValues, setLocalValues] = useState<Record<string, string>>({})

  useEffect(() => {
    if (settings) setLocalValues({ ...settings })
  }, [settings])

  const updateMutation = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) => updateSetting(key, value),
    onSuccess: () => {
      toast.success(t('settings.saveSuccess'))
      queryClient.invalidateQueries({ queryKey: ['admin-settings'] })
      fetchPublicSettings()
    },
    onError: (err: any) => toast.error(err?.response?.data?.message || t('settings.saveFailed')),
  })

  const updateLocal = (key: string, value: string) => setLocalValues((prev) => ({ ...prev, [key]: value }))

  const saveField = useCallback((key: string) => {
    const newValue = localValues[key] ?? ''
    const oldValue = settings?.[key] ?? ''
    if (newValue === oldValue) return
    updateMutation.mutate({ key, value: newValue })
  }, [localValues, settings, updateMutation])

  const toggleBool = (key: string) => {
    const next = localValues[key] !== 'true'
    updateMutation.mutate({ key, value: String(next) })
  }

  // Group ratio editor
  const [ratio, setRatio] = useState<Record<string, number>>({})
  useEffect(() => {
    setRatio(parseRatio(localValues.GroupRatio ?? ''))
  }, [localValues.GroupRatio])

  const saveRatio = (next: Record<string, number>) => {
    setRatio(next)
    const json = JSON.stringify(next)
    updateLocal('GroupRatio', json)
    updateMutation.mutate({ key: 'GroupRatio', value: json })
  }

  const updateGroupRatio = (name: string, value: number) => saveRatio({ ...ratio, [name]: value })

  if (isLoading) {
    return <div className="p-6 lg:p-8"><p className="text-sm text-muted-foreground">{t('common.loading')}</p></div>
  }

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('nav.adminBilling')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('billing.enabled')} / {t('billing.disabled')}</p>
      </div>

      <Tabs defaultValue="basics">
        <TabsList className="flex w-full max-w-2xl flex-wrap h-auto gap-1">
          <TabsTrigger value="basics" className="gap-1.5"><CreditCard className="h-3.5 w-3.5" />{t('billing.enabled')}</TabsTrigger>
          <TabsTrigger value="charging" className="gap-1.5"><Coins className="h-3.5 w-3.5" />{t('marketplace.pricing')}</TabsTrigger>
          <TabsTrigger value="quota" className="gap-1.5"><SlidersHorizontal className="h-3.5 w-3.5" />{t('wallet.balance')}</TabsTrigger>
          <TabsTrigger value="advanced" className="gap-1.5"><ScrollText className="h-3.5 w-3.5" />{t('billing.scopeGlobal')}</TabsTrigger>
        </TabsList>

        {/* Basics: master switch, currency, per-unit */}
        <TabsContent value="basics">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">{t('billing.enabled')}</p>
                <p className="text-xs text-muted-foreground">BillingEnabled</p>
              </div>
              <Switch checked={localValues.BillingEnabled === 'true'} onCheckedChange={() => toggleBool('BillingEnabled')} />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>DisplayCurrency</Label>
                <Select value={localValues.DisplayCurrency ?? 'CNY'} onValueChange={(v) => { updateLocal('DisplayCurrency', v); }}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="CNY">CNY (¥)</SelectItem>
                    <SelectItem value="USD">USD ($)</SelectItem>
                    <SelectItem value="EUR">EUR (€)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>QuotaPerUnit</Label>
                <Input
                  type="number"
                  value={localValues.QuotaPerUnit ?? '500000'}
                  onChange={(e) => updateLocal('QuotaPerUnit', e.target.value)}
                  onBlur={() => saveField('QuotaPerUnit')}
                />
              </div>
            </div>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">{t('marketplace.clone')}</p>
                <p className="text-xs text-muted-foreground">RedemptionEnabled</p>
              </div>
              <Switch checked={localValues.RedemptionEnabled === 'true'} onCheckedChange={() => toggleBool('RedemptionEnabled')} />
            </div>
          </div>
        </TabsContent>

        {/* Charging rules: default price, group ratio, charge flags */}
        <TabsContent value="charging">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>BillingDefaultType</Label>
                <Select
                  value={localValues.BillingDefaultType ?? 'per_call'}
                  onValueChange={(v) => updateLocal('BillingDefaultType', v)}
                >
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="per_call">{t('marketplace.billingPerCall')}</SelectItem>
                    <SelectItem value="free">{t('marketplace.billingFree')}</SelectItem>
                  </SelectContent>
                </Select>
                <Button variant="ghost" size="sm" className="h-6 px-2 text-xs" onClick={() => saveField('BillingDefaultType')}>
                  {t('common.save')}
                </Button>
              </div>
              <div className="space-y-2">
                <Label>BillingDefaultPricePerCall</Label>
                <Input
                  type="number"
                  step="0.0001"
                  value={localValues.BillingDefaultPricePerCall ?? '0'}
                  onChange={(e) => updateLocal('BillingDefaultPricePerCall', e.target.value)}
                  onBlur={() => saveField('BillingDefaultPricePerCall')}
                />
              </div>
            </div>

            <div className="space-y-2 border-t pt-4">
              <Label>GroupRatio</Label>
              <p className="text-xs text-muted-foreground">{t('pricing.defaultDesc')}</p>
              <div className="space-y-2">
                {userGroupOptions.map((g) => (
                  <div key={g} className="flex items-center gap-2">
                    <Input value={g} disabled className="w-32 bg-muted" />
                    <Input
                      type="number"
                      step="0.01"
                      value={ratio[g] ?? 1}
                      onChange={(e) => updateGroupRatio(g, Number(e.target.value))}
                      className="w-28"
                    />
                  </div>
                ))}
              </div>
            </div>

            <div className="space-y-3 border-t pt-4">
              {[
                { key: 'ChargeAdmin', label: 'ChargeAdmin' },
                { key: 'ChargeOnClientError', label: 'ChargeOnClientError' },
                { key: 'ChargeOnTimeout', label: 'ChargeOnTimeout' },
                { key: 'BillingFailOpen', label: 'BillingFailOpen' },
              ].map((row) => (
                <div key={row.key} className="flex items-center justify-between">
                  <p className="text-sm font-medium">{row.label}</p>
                  <Switch checked={localValues[row.key] === 'true'} onCheckedChange={() => toggleBool(row.key)} />
                </div>
              ))}
            </div>
          </div>
        </TabsContent>

        {/* Quota: trust, new user, remind threshold */}
        <TabsContent value="quota">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>TrustQuota</Label>
                <Input
                  type="number"
                  value={localValues.TrustQuota ?? '5000000'}
                  onChange={(e) => updateLocal('TrustQuota', e.target.value)}
                  onBlur={() => saveField('TrustQuota')}
                />
              </div>
              <div className="space-y-2">
                <Label>QuotaForNewUser</Label>
                <Input
                  type="number"
                  value={localValues.QuotaForNewUser ?? '0'}
                  onChange={(e) => updateLocal('QuotaForNewUser', e.target.value)}
                  onBlur={() => saveField('QuotaForNewUser')}
                />
              </div>
              <div className="space-y-2">
                <Label>QuotaRemindThreshold</Label>
                <Input
                  type="number"
                  value={localValues.QuotaRemindThreshold ?? '0'}
                  onChange={(e) => updateLocal('QuotaRemindThreshold', e.target.value)}
                  onBlur={() => saveField('QuotaRemindThreshold')}
                />
              </div>
            </div>
          </div>
        </TabsContent>

        {/* Advanced: self-use, owned services, logs */}
        <TabsContent value="advanced">
          <div className="rounded-xl border bg-card p-5 space-y-5 max-w-2xl">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-sm font-medium">SelfUseModeEnabled</p>
                <p className="text-xs text-muted-foreground">{t('pricing.selfUseNote')}</p>
              </div>
              <Switch checked={localValues.SelfUseModeEnabled === 'true'} onCheckedChange={() => toggleBool('SelfUseModeEnabled')} />
            </div>
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-sm font-medium">UserOwnedServicesEnabled</p>
                <p className="text-xs text-muted-foreground">{t('pricing.commercialNote')}</p>
              </div>
              <Switch checked={localValues.UserOwnedServicesEnabled === 'true'} onCheckedChange={() => toggleBool('UserOwnedServicesEnabled')} />
            </div>

            <div className="space-y-3 border-t pt-4">
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium">LogPayloadEnabled</p>
                <Switch checked={localValues.LogPayloadEnabled === 'true'} onCheckedChange={() => toggleBool('LogPayloadEnabled')} />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>LogRetentionDays</Label>
                  <Input
                    type="number"
                    value={localValues.LogRetentionDays ?? '30'}
                    onChange={(e) => updateLocal('LogRetentionDays', e.target.value)}
                    onBlur={() => saveField('LogRetentionDays')}
                  />
                </div>
              </div>
              <div className="flex items-start gap-2 rounded-lg bg-muted/40 p-3 text-xs text-muted-foreground">
                <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span>LogRetentionDays=0 {t('billing.scopeGlobal')} · TTL {t('settings.maintenance')}</span>
              </div>
            </div>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}
