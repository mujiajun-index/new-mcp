import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { SectionCard } from '@/components/section-card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from 'sonner'
import { KeyRound, Loader2, Plus, ShieldAlert, Trash2 } from 'lucide-react'
import type { ServiceKeysResp, ServiceKeyItem, UpdateServiceKeysReq, UpdateServiceKeysResult } from '@/types'

// statusLabel 状态徽章(1启用/2手动禁用/3自动禁用)
function statusBadge(t: (k: string) => string, status: number) {
  if (status === 1) return <Badge className="bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">{t('services.keys.statusEnabled')}</Badge>
  if (status === 3) return <Badge className="bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300">{t('services.keys.statusAutoDisabled')}</Badge>
  return <Badge variant="secondary">{t('services.keys.statusDisabled')}</Badge>
}

/**
 * KeysApi 秘钥卡片的端点适配:卡片本体与服务详情页(/services/:id/keys)和
 * 市场管理详情页(/admin/marketplace/:id/keys)共用,仅端点与鉴权语境不同,
 * 由使用方构造注入。list/updateKeys 返回接口信封(卡片读 .data)。
 */
export interface KeysApi {
  list: () => Promise<{ data?: ServiceKeysResp }>
  updateKeys: (data: UpdateServiceKeysReq) => Promise<{ data?: UpdateServiceKeysResult }>
  updateConfig: (data: { key_mode: 'single' | 'random' | 'polling'; header_name?: string }) => Promise<unknown>
  setKeyStatus: (keyID: number, status: 'enabled' | 'disabled') => Promise<unknown>
  deleteKey: (keyID: number) => Promise<unknown>
  batch: (action: 'enable_all' | 'delete_disabled') => Promise<unknown>
}

/**
 * 秘钥管理卡片:单↔多模式与策略切换、池表格(掩码/状态/原因/启禁删)、
 * 添加秘钥(追加/替换)、批量操作。渲染条件由父组件把关(服务详情页:自有
 * HTTP 类服务;市场管理详情:instant HTTP 类条目——一份池对全部安装用户全局轮换)。
 * onModeChanged:模式/池变化后失效宿主页面的详情查询(徽章与认证区联动)。
 */
export function ServiceKeysCard({
  id, api, onModeChanged,
}: {
  id: number
  api: KeysApi
  onModeChanged: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [addOpen, setAddOpen] = useState(false)
  const [addValues, setAddValues] = useState('')
  const [addMode, setAddMode] = useState<'append' | 'replace'>('append')
  const [replaceConfirm, setReplaceConfirm] = useState(false)
  const [downgradeConfirm, setDowngradeConfirm] = useState(false)
  const [deleteDisabledConfirm, setDeleteDisabledConfirm] = useState(false)
  const [customHeader, setCustomHeader] = useState('')
  // 待启用/待禁用的策略暂存:custom 单→多时先填注入头
  const [pendingMode, setPendingMode] = useState<'random' | 'polling' | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['service-keys', id],
    queryFn: () => api.list(),
  })
  const resp = data?.data
  const keys: ServiceKeyItem[] = resp?.keys || []
  const isMulti = resp?.key_mode === 'random' || resp?.key_mode === 'polling'
  const enabledCount = resp?.enabled || 0
  const disabledKeys = keys.filter((k) => k.status !== 1).length

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ['service-keys', id] })
    onModeChanged()
  }

  const modeMutation = useMutation({
    mutationFn: (payload: { key_mode: 'single' | 'random' | 'polling'; header_name?: string }) =>
      api.updateConfig(payload),
    onSuccess: (_res, payload) => {
      toast.success(payload.key_mode === 'single' ? t('services.keys.downgraded') : t('services.keys.modeSwitched'))
      setPendingMode(null)
      setCustomHeader('')
      invalidate()
    },
  })

  const updateMutation = useMutation({
    mutationFn: () => api.updateKeys({
      mode: addMode,
      values: addValues.split('\n').map((s) => s.trim()).filter(Boolean),
    }),
    onSuccess: (res) => {
      const r = res?.data
      toast.success(t('services.keys.updated', { added: r?.added ?? 0, skipped: r?.skipped ?? 0 }))
      setAddOpen(false)
      setAddValues('')
      invalidate()
    },
  })

  const keyActionMutation = useMutation({
    mutationFn: ({ key, action }: { key: ServiceKeyItem; action: 'enable' | 'disable' | 'delete' }) => {
      if (action === 'delete') return api.deleteKey(key.id)
      return api.setKeyStatus(key.id, action === 'enable' ? 'enabled' : 'disabled')
    },
    onSuccess: (_d, { action }) => {
      toast.success(action === 'delete' ? t('services.keys.deleted') : t('services.keys.statusUpdated'))
      invalidate()
    },
  })

  const batchMutation = useMutation({
    mutationFn: (action: 'enable_all' | 'delete_disabled') => api.batch(action),
    onSuccess: () => {
      setDeleteDisabledConfirm(false)
      toast.success(t('common.success'))
      invalidate()
    },
  })

  // 单→多:custom 认证需指定注入头——端点响应里已带 header_name(服务侧来自
  // auth_config 落库、条目侧来自模板反推)则直接切换;否则弹框让用户填写
  function requestUpgrade(mode: 'random' | 'polling') {
    if (resp?.auth_type === 'custom' && !isMulti) {
      if (resp?.header_name) {
        modeMutation.mutate({ key_mode: mode, header_name: resp.header_name })
        return
      }
      setPendingMode(mode)
      return
    }
    modeMutation.mutate({ key_mode: mode })
  }

  function confirmUpgrade() {
    if (!pendingMode) return
    if (!customHeader.trim()) {
      toast.error(t('services.keys.headerNameRequired'))
      return
    }
    modeMutation.mutate({ key_mode: pendingMode, header_name: customHeader.trim() })
  }

  return (
    <SectionCard
      defaultOpen={false}
      title={
        <span className="flex items-center gap-2">
          <KeyRound className="h-4 w-4 text-muted-foreground" />
          {t('services.keys.cardTitle')}
          {isMulti && (
            <Badge variant="outline" className="ml-1">
              {resp?.key_mode === 'random' ? t('services.keys.modeRandom') : t('services.keys.modePolling')}
              {' · '}
              {enabledCount}/{resp?.total ?? 0}
            </Badge>
          )}
        </span>
      }
      actions={isMulti ? (
        <div className="flex gap-2">
          <Button size="sm" variant="outline" className="gap-1" onClick={() => { setAddMode('append'); setAddOpen(true) }}>
            <Plus className="h-3.5 w-3.5" />
            {t('services.keys.addBtn')}
          </Button>
          <Button size="sm" variant="outline" onClick={() => batchMutation.mutate('enable_all')} disabled={disabledKeys === 0 || batchMutation.isPending}>
            {t('services.keys.enableAll')}
          </Button>
          <Button size="sm" variant="outline" className="text-destructive" onClick={() => setDeleteDisabledConfirm(true)} disabled={disabledKeys === 0 || batchMutation.isPending}>
            {t('services.keys.deleteDisabled')}
          </Button>
        </div>
      ) : undefined}
    >
      {isLoading ? (
        <div className="flex items-center justify-center py-6 text-muted-foreground text-sm">{t('common.loading')}</div>
      ) : !isMulti ? (
        /* 单秘钥态:提供切换入口 */
        <div className="space-y-3">
          <p className="text-sm text-muted-foreground">{t('services.keys.singleHint')}</p>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => requestUpgrade('random')} disabled={modeMutation.isPending}>
              {t('services.keys.switchToMulti')} · {t('services.keys.modeRandom')}
            </Button>
            <Button size="sm" variant="outline" onClick={() => requestUpgrade('polling')} disabled={modeMutation.isPending}>
              {t('services.keys.switchToMulti')} · {t('services.keys.modePolling')}
            </Button>
          </div>
        </div>
      ) : (
        <div className="space-y-3">
          {/* 全部禁用警示 */}
          {enabledCount === 0 && (
            <div className="flex items-start gap-2 rounded-lg border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300">
              <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t('services.keys.allDisabledWarning')}</span>
            </div>
          )}

          {/* 策略切换 + 降级 */}
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm text-muted-foreground">{t('services.keys.strategyLabel')}</span>
            {(['random', 'polling'] as const).map((m) => (
              <button
                key={m}
                type="button"
                disabled={resp?.key_mode === m || modeMutation.isPending}
                onClick={() => modeMutation.mutate({ key_mode: m })}
                className={`rounded-lg border px-2.5 py-1 text-sm transition-all ${
                  resp?.key_mode === m
                    ? 'border-primary bg-primary/5 text-foreground'
                    : 'text-muted-foreground hover:border-primary/30'
                }`}
              >
                {m === 'random' ? t('services.keys.modeRandom') : t('services.keys.modePolling')}
              </button>
            ))}
            <Button size="sm" variant="ghost" className="ml-auto text-muted-foreground" onClick={() => setDowngradeConfirm(true)} disabled={modeMutation.isPending}>
              {t('services.keys.switchToSingle')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            {resp?.key_mode === 'random' ? t('services.keys.modeRandomHint') : t('services.keys.modePollingHint')}
            {t('services.keys.headerLabel')}
            {resp?.header_name && (
              <code className="ml-1 rounded bg-primary/10 px-1.5 py-0.5 font-mono text-[11px] font-semibold text-primary dark:bg-primary/20">
                {resp.header_name}
              </code>
            )}
          </p>

          {/* 池表格 */}
          <div className="space-y-1.5">
            {keys.map((k) => (
              <div key={k.id} className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
                <div className="flex min-w-0 items-center gap-3">
                  <span className="w-7 shrink-0 text-right font-mono text-xs text-muted-foreground">#{k.sort_order}</span>
                  <span className="truncate font-mono text-xs">{k.masked_value}</span>
                  {statusBadge(t, k.status)}
                  {k.status !== 1 && k.disabled_reason && (
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="cursor-help text-xs text-muted-foreground underline decoration-dotted">
                            {t('services.keys.failureReason')}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          <p className="max-w-xs text-xs">
                            {k.disabled_reason}
                            {k.disabled_at ? ` · ${new Date(k.disabled_at).toLocaleString()}` : ''}
                          </p>
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  )}
                </div>
                <div className="flex shrink-0 gap-1">
                  {k.status === 1 ? (
                    <Button size="sm" variant="ghost" className="h-7 px-2" disabled={keyActionMutation.isPending} onClick={() => keyActionMutation.mutate({ key: k, action: 'disable' })}>
                      {t('services.keys.disable')}
                    </Button>
                  ) : (
                    <Button size="sm" variant="ghost" className="h-7 px-2 text-emerald-600" disabled={keyActionMutation.isPending} onClick={() => keyActionMutation.mutate({ key: k, action: 'enable' })}>
                      {t('services.keys.enable')}
                    </Button>
                  )}
                  <Button size="sm" variant="ghost" className="h-7 px-2 text-destructive" disabled={keyActionMutation.isPending} onClick={() => keyActionMutation.mutate({ key: k, action: 'delete' })}>
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 添加秘钥:追加/替换 */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('services.keys.addDialogTitle')}</DialogTitle>
            <DialogDescription>{t('services.keys.addDialogDesc')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            {/* 选项框按文字宽度自适应(与策略切换按钮同规格);? 在框外,hover 展开提示 */}
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
              {(['append', 'replace'] as const).map((m) => (
                <div key={m} className="flex items-center gap-1">
                  <button
                    type="button"
                    onClick={() => setAddMode(m)}
                    className={`rounded-lg border px-2.5 py-1 text-sm font-medium transition-all ${
                      addMode === m ? 'border-primary bg-primary/5' : 'text-muted-foreground hover:border-primary/30'
                    }`}
                  >
                    {m === 'append' ? t('services.keys.updateAppend') : t('services.keys.updateReplace')}
                  </button>
                  <TooltipProvider>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="inline-flex h-4 w-4 shrink-0 cursor-help items-center justify-center rounded-full border border-muted-foreground/40 text-[10px] leading-none text-muted-foreground">
                          ?
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        <p className="max-w-xs text-xs">
                          {m === 'append' ? t('services.keys.updateAppendHint') : t('services.keys.updateReplaceHint')}
                        </p>
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                </div>
              ))}
            </div>
            <div className="space-y-2">
              <Label>{t('services.keys.valuesLabel')}</Label>
              <Textarea rows={6} placeholder={t('services.keys.textareaPlaceholder')} value={addValues} onChange={(e) => setAddValues(e.target.value)} />
            </div>
          </div>
          <DialogFooter className="mt-4">
            <Button variant="outline" onClick={() => setAddOpen(false)}>{t('common.cancel')}</Button>
            <Button
              disabled={updateMutation.isPending || !addValues.trim()}
              onClick={() => (addMode === 'replace' ? setReplaceConfirm(true) : updateMutation.mutate())}
            >
              {updateMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {addMode === 'append' ? t('services.keys.addBtn') : t('services.keys.replaceBtn')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 替换确认 */}
      <AlertDialog open={replaceConfirm} onOpenChange={setReplaceConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('services.keys.replaceConfirmTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('services.keys.replaceConfirmDesc', { count: resp?.total ?? 0 })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => updateMutation.mutate()}>{t('services.keys.replaceBtn')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* 切回单秘钥确认 */}
      <AlertDialog open={downgradeConfirm} onOpenChange={setDowngradeConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('services.keys.downgradeConfirmTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('services.keys.downgradeConfirmDesc')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => modeMutation.mutate({ key_mode: 'single' })}>
              {t('services.keys.switchToSingle')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* 删除已禁用确认 */}
      <AlertDialog open={deleteDisabledConfirm} onOpenChange={setDeleteDisabledConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('services.keys.deleteDisabledConfirmTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('services.keys.deleteDisabledConfirmDesc', { count: disabledKeys })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction className="bg-destructive text-destructive-foreground" onClick={() => batchMutation.mutate('delete_disabled')}>
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* custom 认证单→多:先指定注入头 */}
      <Dialog open={pendingMode !== null} onOpenChange={(open) => !open && setPendingMode(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('services.keys.headerNameDialogTitle')}</DialogTitle>
            <DialogDescription>{t('services.keys.headerNameHint')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label>{t('services.keys.headerNameLabel')}</Label>
            <Input placeholder="X-Custom-Auth" value={customHeader} onChange={(e) => setCustomHeader(e.target.value)} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingMode(null)}>{t('common.cancel')}</Button>
            <Button onClick={confirmUpgrade} disabled={modeMutation.isPending}>
              {modeMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('common.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
