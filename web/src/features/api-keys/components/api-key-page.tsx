import { useState, useCallback, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getApiKeys, createApiKey, updateApiKey, deleteApiKey, getApiKeyFullKey, batchDeleteApiKeys, batchUpdateApiKeyStatus } from '../api'
import { getGroups } from '@/features/groups/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Tooltip, TooltipContent, TooltipTrigger, TooltipProvider } from '@/components/ui/tooltip'
import { toast } from 'sonner'
import {
  Plus, Trash2, Copy, Key, X, Pencil, ToggleLeft, ToggleRight,
  Infinity, Search, CheckSquare, Square, Clock, Shield, Loader2,
  ChevronDown, Eye, Check,
} from 'lucide-react'
import type { ApiKeyListItem, GroupListItem } from '@/types'

function formatRelativeTime(dateStr: string, localeStr: string, t: (k: string, opts?: any) => string) {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) return t('apiKeys.justNow')
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return t('apiKeys.minutesAgo', { count: minutes })
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return t('apiKeys.hoursAgo', { count: hours })
  const days = Math.floor(hours / 24)
  if (days < 30) return t('apiKeys.daysAgo', { count: days })
  return date.toLocaleDateString(localeStr)
}

function isExpired(key: ApiKeyListItem) {
  if (!key.expires_at) return false
  return new Date(key.expires_at) < new Date()
}

function copyText(text: string) {
  if (navigator.clipboard) {
    return navigator.clipboard.writeText(text)
  }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
  return Promise.resolve()
}

function StatusBadge({ status, expired, t }: { status: number; expired: boolean; t: (k: string) => string }) {
  if (expired) {
    return <Badge variant="outline" className="text-amber-600 border-amber-300 bg-amber-50 dark:bg-amber-950/30">{t('apiKeys.expired')}</Badge>
  }
  if (status === 1) {
    return <Badge variant="outline" className="text-emerald-600 border-emerald-300 bg-emerald-50 dark:bg-emerald-950/30">{t('apiKeys.enabled')}</Badge>
  }
  return <Badge variant="outline" className="text-zinc-500 border-zinc-300 bg-zinc-50 dark:bg-zinc-950/30">{t('apiKeys.disabled')}</Badge>
}

function QuotaDisplay({ used, total, unlimited, t }: { used: number; total: number; unlimited: boolean; t: (k: string, opts?: any) => string }) {
  if (unlimited) {
    return (
      <Badge variant="secondary" className="gap-1">
        <Infinity className="h-3 w-3" />{t('apiKeys.unlimited')}
      </Badge>
    )
  }
  const pct = total > 0 ? (used / total) * 100 : 0
  const remaining = total - used
  const color = remaining <= 0 ? 'text-red-500' : pct > 70 ? 'text-amber-500' : 'text-emerald-500'
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className="flex items-center gap-2 min-w-[100px]">
            <div className="flex-1 h-1.5 bg-muted rounded-full overflow-hidden">
              <div
                className={`h-full rounded-full ${remaining <= 0 ? 'bg-red-500' : pct > 70 ? 'bg-amber-500' : 'bg-emerald-500'}`}
                style={{ width: `${Math.min(pct, 100)}%` }}
              />
            </div>
            <span className={`text-xs tabular-nums ${color}`}>{used}/{total}</span>
          </div>
        </TooltipTrigger>
        <TooltipContent>{t('apiKeys.usedTotal', { used, total, remaining: remaining > 0 ? remaining : 0 })}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function KeyCell({ apiKey }: { apiKey: ApiKeyListItem }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [fullKey, setFullKey] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const fetchKey = useCallback(async () => {
    if (fullKey) return
    setLoading(true)
    try {
      const res = await getApiKeyFullKey(apiKey.id)
      if (res?.data?.key) {
        setFullKey(res.data.key)
      }
    } catch {
      toast.error(t('apiKeys.fetchKeyFailed'))
    } finally {
      setLoading(false)
    }
  }, [apiKey.id, fullKey, t])

  const handleCopy = useCallback(async () => {
    const keyToCopy = fullKey || apiKey.key_prefix
    await copyText(keyToCopy)
    setCopied(true)
    toast.success(t('common.copied'))
    setTimeout(() => setCopied(false), 2000)
  }, [fullKey, apiKey.key_prefix, t])

  const handleOpenChange = useCallback((nextOpen: boolean) => {
    setOpen(nextOpen)
    if (nextOpen && !fullKey) {
      fetchKey()
    }
  }, [fullKey, fetchKey])

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <button className="flex items-center gap-1.5 group cursor-pointer">
          <code className="text-xs font-mono text-muted-foreground group-hover:text-foreground transition-colors">
            {apiKey.key_prefix}{apiKey.key_prefix.length < 8 ? '' : '****'}
          </code>
          <Eye className="h-3 w-3 text-muted-foreground group-hover:text-foreground transition-colors" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-[min(360px,calc(100vw-2rem))] p-3" align="start">
        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">{t('apiKeys.keyLabel')}</p>
          <div className="flex items-center gap-2">
            <input
              ref={inputRef}
              type="text"
              readOnly
              value={loading ? '...' : (fullKey || apiKey.key_prefix)}
              className="flex-1 rounded-md bg-muted px-3 py-1.5 text-sm font-mono border-0 outline-none"
              title={t('common.clickToSelect')}
              aria-label={t('apiKeys.keyLabel')}
              onClick={() => inputRef.current?.select()}
            />
            <Button variant="outline" size="sm" className="shrink-0 gap-1" onClick={handleCopy} disabled={loading}>
              {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
              {copied ? t('common.copied') : t('common.copy')}
            </Button>
          </div>
          <p className="text-[10px] text-muted-foreground">{t('apiKeys.keySecurityTip')}</p>
        </div>
      </PopoverContent>
    </Popover>
  )
}

function formatLastUsed(value: unknown, t: (k: string, opts?: any) => string, localeStr: string): string {
  if (!value) return '-'
  if (typeof value === 'string') {
    return formatRelativeTime(value, localeStr, t)
  }
  return String(value)
}

function GroupMultiSelect({
  options,
  value,
  onChange,
  label,
  placeholder,
  emptyText,
  removeLabel,
}: {
  options: { value: string; label: string }[]
  value: string[]
  onChange: (value: string[]) => void
  label: string
  placeholder: string
  emptyText: string
  removeLabel: string
}) {
  const [open, setOpen] = useState(false)

  const toggle = (val: string) => {
    onChange(value.includes(val) ? value.filter(v => v !== val) : [...value, val])
  }

  const remove = (val: string) => {
    onChange(value.filter(v => v !== val))
  }

  const labelOf = (val: string) => options.find(o => o.value === val)?.label || val

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <div
          role="combobox"
          aria-expanded={open}
          aria-haspopup="listbox"
          tabIndex={0}
          className="flex min-h-[38px] w-full cursor-pointer flex-wrap items-center gap-1 rounded-md border border-input bg-transparent px-2 py-1 text-sm shadow-sm transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {value.length > 0 ? (
            value.map(v => (
              <span
                key={v}
                className="inline-flex max-w-full items-center gap-0.5 rounded bg-primary/10 px-1.5 py-0.5 text-xs text-primary"
              >
                <span className="max-w-[160px] truncate">{labelOf(v)}</span>
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label={removeLabel}
                  className="inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-sm hover:bg-primary/20"
                  onPointerDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation()
                    e.preventDefault()
                    remove(v)
                  }}
                >
                  <X className="h-3 w-3" />
                </button>
              </span>
            ))
          ) : (
            <span className="px-1 py-0.5 text-muted-foreground">{placeholder}</span>
          )}
          <ChevronDown className="ml-auto h-4 w-4 shrink-0 opacity-50" />
        </div>
      </PopoverTrigger>
      <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
        {options.length === 0 ? (
          <div className="py-6 text-center text-sm text-muted-foreground">{emptyText}</div>
        ) : (
          <div role="listbox" aria-label={label} className="max-h-60 overflow-auto p-1">
            {options.map(o => {
              const checked = value.includes(o.value)
              return (
                <button
                  key={o.value}
                  type="button"
                  role="option"
                  aria-selected={checked}
                  className={cn(
                    'flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted',
                    checked ? 'font-medium text-primary' : 'text-foreground'
                  )}
                  onClick={() => toggle(o.value)}
                >
                  <span className="truncate text-left">{o.label}</span>
                  {checked && <Check className="h-4 w-4 shrink-0" />}
                </button>
              )
            })}
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}

export function ApiKeyPage() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [showCreate, setShowCreate] = useState(false)
  const [editingKey, setEditingKey] = useState<ApiKeyListItem | null>(null)
  const [newKey, setNewKey] = useState<string | null>(null)
  const [newKeyCopied, setNewKeyCopied] = useState(false)
  const [showBatchMenu, setShowBatchMenu] = useState(false)
  const [form, setForm] = useState<{
    name: string
    groups: string[]
    quota: string
    unlimited_quota: boolean
    allow_ips: string
    expires_at: string
    never_expires: boolean
  }>({
    name: '',
    groups: [],
    quota: '',
    unlimited_quota: true,
    allow_ips: '',
    expires_at: '',
    never_expires: true,
  })

  const { data: keysData, isLoading } = useQuery({
    queryKey: ['api-keys', search],
    queryFn: () => getApiKeys(search || undefined),
  })

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: () => getGroups(),
  })

  const resetForm = () => setForm({ name: '', groups: [], quota: '', unlimited_quota: true, allow_ips: '', expires_at: '', never_expires: true })

  const createMutation = useMutation({
    mutationFn: () => createApiKey({
      name: form.name,
      groups: form.groups,
      quota: form.unlimited_quota ? undefined : (parseInt(form.quota) || undefined),
      unlimited_quota: form.unlimited_quota,
      allow_ips: form.allow_ips || undefined,
      expires_at: form.never_expires ? undefined : (form.expires_at || undefined),
    }),
    onSuccess: (res) => {
      toast.success(t('apiKeys.createSuccess'))
      setNewKey(res.data?.key || null)
      setNewKeyCopied(false)
      setShowCreate(false)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: () => toast.error(t('apiKeys.createFailed')),
  })

  const updateMutation = useMutation({
    mutationFn: (data: { id: number; body: any }) => updateApiKey(data.id, data.body),
    onSuccess: () => {
      toast.success(t('common.success'))
      setEditingKey(null)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: () => toast.error(t('common.error')),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteApiKey,
    onSuccess: () => {
      toast.success(t('apiKeys.deleteSuccess'))
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  const batchDeleteMutation = useMutation({
    mutationFn: (ids: number[]) => batchDeleteApiKeys(ids),
    onSuccess: () => {
      toast.success(t('apiKeys.batchDeleteSuccess'))
      setSelected(new Set())
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  const batchStatusMutation = useMutation({
    mutationFn: ({ ids, status }: { ids: number[]; status: number }) => batchUpdateApiKeyStatus(ids, status),
    onSuccess: () => {
      toast.success(t('common.success'))
      setSelected(new Set())
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  const toggleStatus = (key: ApiKeyListItem) => {
    updateMutation.mutate({ id: key.id, body: { status: key.status === 1 ? 2 : 1 } })
  }

  const startEdit = (key: ApiKeyListItem) => {
    setEditingKey(key)
    setForm({
      name: key.name,
      groups: key.groups || [],
      quota: key.unlimited_quota ? '' : String(key.quota),
      unlimited_quota: key.unlimited_quota,
      allow_ips: key.allow_ips || '',
      expires_at: key.expires_at || '',
      never_expires: !key.expires_at,
    })
  }

  const toggleSelect = (id: number) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleSelectAll = () => {
    const keys = keysData?.data || []
    if (selected.size === keys.length && keys.length > 0) {
      setSelected(new Set())
    } else {
      setSelected(new Set(keys.map((k: ApiKeyListItem) => k.id)))
    }
  }

  const keys = keysData?.data || []
  const groups: GroupListItem[] = groupsData?.data || []
  const allSelected = keys.length > 0 && selected.size === keys.length
  const someSelected = selected.size > 0 && !allSelected

  const locale = i18n.language?.startsWith('zh') ? 'zh-CN' : 'en-US'

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('apiKeys.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('apiKeys.subtitle')}</p>
        </div>
        <Button className="gap-2" onClick={() => { setShowCreate(true); setEditingKey(null); resetForm() }}>
          <Plus className="h-4 w-4" />{t('apiKeys.create')}
        </Button>
      </div>

      {/* New key display */}
      {newKey && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-5">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-amber-600 dark:text-amber-400">{t('apiKeys.copyNow')}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t('apiKeys.copyNowDesc')}</p>
            </div>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setNewKey(null)}>
              <X className="h-4 w-4" />
            </Button>
          </div>
          <div className="mt-3 flex items-center gap-2">
            <input
              type="text"
              readOnly
              value={newKey}
              className="flex-1 rounded-lg bg-muted px-3 py-2 text-sm font-mono border-0 outline-none"
              title={t('common.clickToSelect')}
              aria-label={t('apiKeys.keyLabel')}
              onClick={(e) => (e.target as HTMLInputElement).select()}
            />
            <Button
              variant="outline"
              size="sm"
              className="gap-1"
              onClick={() => {
                copyText(newKey)
                setNewKeyCopied(true)
                toast.success(t('common.copied'))
                setTimeout(() => setNewKeyCopied(false), 2000)
              }}
            >
              {newKeyCopied ? <Check className="h-4 w-4 text-emerald-500" /> : <Copy className="h-4 w-4" />}
              {newKeyCopied ? t('common.copied') : t('common.copy')}
            </Button>
          </div>
        </div>
      )}

      {/* Create / Edit form */}
      {(showCreate || editingKey) && (
        <div className="rounded-xl border bg-card p-5 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">{editingKey ? t('apiKeys.edit') : t('apiKeys.create')}</h2>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => { setShowCreate(false); setEditingKey(null); resetForm() }}>
              <X className="h-4 w-4" />
            </Button>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>{t('apiKeys.name')}</Label>
              <Input placeholder="my-key" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t('apiKeys.groups')} <span className="text-destructive">*</span></Label>
              <GroupMultiSelect
                options={groups.map(g => ({ value: g.name, label: g.display_name || g.name }))}
                value={form.groups}
                onChange={selected => setForm({ ...form, groups: selected })}
                label={t('apiKeys.groups')}
                placeholder={t('apiKeys.groupsPlaceholder')}
                emptyText={t('apiKeys.noGroups')}
                removeLabel={t('apiKeys.removeGroup')}
              />
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label>{t('apiKeys.quota')}</Label>
              <div className="flex items-center gap-2">
                <Input
                  placeholder={t('apiKeys.unlimited')}
                  type="number"
                  value={form.quota}
                  disabled={form.unlimited_quota}
                  onChange={e => setForm({ ...form, quota: e.target.value })}
                />
                <Button
                  type="button"
                  variant={form.unlimited_quota ? 'default' : 'outline'}
                  size="sm"
                  className="shrink-0 gap-1"
                  onClick={() => setForm({ ...form, unlimited_quota: !form.unlimited_quota })}
                >
                  <Infinity className="h-3.5 w-3.5" />
                  {t('apiKeys.unlimited')}
                </Button>
              </div>
            </div>
            <div className="space-y-2">
              <Label>{t('apiKeys.ipWhitelist')}</Label>
              <Input placeholder={t('apiKeys.ipPlaceholder')} value={form.allow_ips} onChange={e => setForm({ ...form, allow_ips: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t('apiKeys.expires')}</Label>
              <div className="flex items-center gap-2">
                <Input
                  type="datetime-local"
                  disabled={form.never_expires}
                  value={form.expires_at ? form.expires_at.slice(0, 16) : ''}
                  onChange={e => {
                    const v = e.target.value
                    setForm({ ...form, expires_at: v ? new Date(v).toISOString() : '' })
                  }}
                />
                <Button
                  type="button"
                  variant={form.never_expires ? 'default' : 'outline'}
                  size="sm"
                  className="shrink-0 gap-1"
                  onClick={() => setForm({ ...form, never_expires: !form.never_expires, expires_at: form.never_expires ? '' : form.expires_at })}
                >
                  <Shield className="h-3.5 w-3.5" />
                  {t('apiKeys.neverExpires')}
                </Button>
              </div>
            </div>
          </div>

          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => { setShowCreate(false); setEditingKey(null); resetForm() }}>{t('common.cancel')}</Button>
            <Button
              disabled={!form.name.trim() || !form.groups.length}
              onClick={() => {
                if (editingKey) {
                  updateMutation.mutate({
                    id: editingKey.id,
                    body: {
                      name: form.name,
                      groups: form.groups,
                      quota: form.unlimited_quota ? undefined : (parseInt(form.quota) || undefined),
                      unlimited_quota: form.unlimited_quota,
                      allow_ips: form.allow_ips || undefined,
                      expires_at: form.never_expires ? '' : (form.expires_at || undefined),
                    },
                  })
                } else {
                  createMutation.mutate()
                }
              }}
            >
              {editingKey ? t('common.save') : t('apiKeys.create')}
            </Button>
          </div>
        </div>
      )}

      {/* Search + Batch Actions Bar */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('apiKeys.searchPlaceholder')}
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="pl-9"
          />
          {search && (
            <button
              type="button"
              className="absolute right-2 top-1/2 z-10 -translate-y-1/2 cursor-pointer rounded-sm p-0.5 text-muted-foreground hover:text-foreground"
              title={t('common.clear')}
              aria-label={t('common.clear')}
              onClick={() => setSearch('')}
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
        {selected.size > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{t('apiKeys.selected', { count: selected.size })}</span>
            <div className="relative">
              <Button
                variant="outline"
                size="sm"
                className="gap-1"
                onClick={() => setShowBatchMenu(!showBatchMenu)}
              >
                {t('apiKeys.batchActions')}<ChevronDown className="h-3.5 w-3.5" />
              </Button>
              {showBatchMenu && (
                <div className="absolute right-0 top-full mt-1 z-50 w-40 rounded-lg border bg-popover p-1 shadow-md">
                  <button
                    className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-muted"
                    onClick={() => { batchStatusMutation.mutate({ ids: [...selected], status: 1 }); setShowBatchMenu(false) }}
                  >
                    <ToggleRight className="h-4 w-4 text-emerald-500" />{t('apiKeys.batchEnable')}
                  </button>
                  <button
                    className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-muted"
                    onClick={() => { batchStatusMutation.mutate({ ids: [...selected], status: 2 }); setShowBatchMenu(false) }}
                  >
                    <ToggleLeft className="h-4 w-4 text-zinc-400" />{t('apiKeys.batchDisable')}
                  </button>
                  <button
                    className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-destructive hover:bg-destructive/10"
                    onClick={() => {
                      if (confirm(t('apiKeys.batchDeleteConfirm', { count: selected.size }))) {
                        batchDeleteMutation.mutate([...selected])
                      }
                      setShowBatchMenu(false)
                    }}
                  >
                    <Trash2 className="h-4 w-4" />{t('apiKeys.batchDelete')}
                  </button>
                </div>
              )}
            </div>
            <Button variant="ghost" size="sm" onClick={() => setSelected(new Set())}>
              {t('apiKeys.clearSelection')}
            </Button>
          </div>
        )}
      </div>

      {/* Table */}
      <div className="rounded-xl border bg-card overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />{t('common.loading')}
          </div>
        ) : keys.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Key className="h-10 w-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">{search ? t('apiKeys.noSearchResult') : t('apiKeys.noKeys')}</p>
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {keys.map((key: ApiKeyListItem) => {
              const expired = isExpired(key)
              return (
                <MobileListCard
                  key={key.id}
                  className={selected.has(key.id) ? 'bg-muted/40' : undefined}
                  title={
                    <div className="flex items-center gap-2">
                      <button onClick={() => toggleSelect(key.id)} className="text-muted-foreground hover:text-foreground">
                        {selected.has(key.id) ? <CheckSquare className="h-4 w-4 text-primary" /> : <Square className="h-4 w-4" />}
                      </button>
                      <span className="font-medium">{key.name}</span>
                    </div>
                  }
                  badge={
                    <button onClick={() => toggleStatus(key)} title={key.status === 1 ? t('apiKeys.clickDisable') : t('apiKeys.clickEnable')}>
                      <StatusBadge status={key.status} expired={expired} t={t} />
                    </button>
                  }
                  meta={[
                    { label: t('apiKeys.key'), value: <KeyCell apiKey={key} /> },
                    { label: t('apiKeys.groups'), value: key.groups?.length ? key.groups.join(', ') : '-' },
                    {
                      label: t('apiKeys.quota'),
                      value: <QuotaDisplay used={key.used_quota} total={key.quota} unlimited={key.unlimited_quota} t={t} />,
                    },
                    {
                      label: t('apiKeys.expires'),
                      value: key.expires_at ? (
                        <span className={expired ? 'text-amber-600' : ''}>
                          {new Date(key.expires_at).toLocaleDateString(locale)}
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1">
                          <Shield className="h-3 w-3" />{t('apiKeys.neverExpires')}
                        </span>
                      ),
                    },
                    {
                      label: t('apiKeys.lastUsed'),
                      value: (
                        <span className="inline-flex items-center gap-1">
                          <Clock className="h-3 w-3" />
                          {formatLastUsed(key.last_used_at, t, locale)}
                        </span>
                      ),
                    },
                  ]}
                  actions={
                    <>
                      <Button variant="ghost" size="sm" onClick={() => startEdit(key)} title={t('common.edit')}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" onClick={() => {
                        if (confirm(t('apiKeys.deleteConfirm', { name: key.name }))) deleteMutation.mutate(key.id)
                      }} title={t('common.delete')}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </>
                  }
                />
              )
            })}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/50 hover:bg-muted/50">
                <TableHead className="w-10 px-4">
                  <button onClick={toggleSelectAll} className="text-muted-foreground hover:text-foreground">
                    {allSelected ? <CheckSquare className="h-4 w-4" /> : someSelected ? <div className="h-4 w-4 rounded border-2 border-current flex items-center justify-center text-xs">-</div> : <Square className="h-4 w-4" />}
                  </button>
                </TableHead>
                <TableHead className="px-4">{t('apiKeys.name')}</TableHead>
                <TableHead className="px-4">{t('apiKeys.key')}</TableHead>
                <TableHead className="px-4">{t('apiKeys.groups')}</TableHead>
                <TableHead className="px-4">{t('apiKeys.quota')}</TableHead>
                <TableHead className="px-4">{t('apiKeys.status')}</TableHead>
                <TableHead className="px-4">{t('apiKeys.lastUsed')}</TableHead>
                <TableHead className="px-4">{t('apiKeys.expires')}</TableHead>
                <TableHead className="px-4 text-right">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.map((key: ApiKeyListItem) => {
                const expired = isExpired(key)
                return (
                  <TableRow key={key.id} data-state={selected.has(key.id) ? 'selected' : undefined}>
                    <TableCell className="px-4">
                      <button onClick={() => toggleSelect(key.id)} className="text-muted-foreground hover:text-foreground">
                        {selected.has(key.id) ? <CheckSquare className="h-4 w-4 text-primary" /> : <Square className="h-4 w-4" />}
                      </button>
                    </TableCell>
                    <TableCell className="px-4 font-medium">{key.name}</TableCell>
                    <TableCell className="px-4">
                      <KeyCell apiKey={key} />
                    </TableCell>
                    <TableCell className="px-4">
                      {key.groups?.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {key.groups.slice(0, 2).map(g => (
                            <span key={g} className="rounded bg-muted px-1.5 py-0.5 text-[10px]">{g}</span>
                          ))}
                          {key.groups.length > 2 && (
                            <TooltipProvider>
                              <Tooltip>
                                <TooltipTrigger>
                                  <span className="rounded bg-muted px-1.5 py-0.5 text-[10px]">+{key.groups.length - 2}</span>
                                </TooltipTrigger>
                                <TooltipContent>{key.groups.join(', ')}</TooltipContent>
                              </Tooltip>
                            </TooltipProvider>
                          )}
                        </div>
                      ) : (
                        <span className="text-xs text-muted-foreground">-</span>
                      )}
                    </TableCell>
                    <TableCell className="px-4">
                      <QuotaDisplay used={key.used_quota} total={key.quota} unlimited={key.unlimited_quota} t={t} />
                    </TableCell>
                    <TableCell className="px-4">
                      <button onClick={() => toggleStatus(key)} title={key.status === 1 ? t('apiKeys.clickDisable') : t('apiKeys.clickEnable')}>
                        <StatusBadge status={key.status} expired={expired} t={t} />
                      </button>
                    </TableCell>
                    <TableCell className="px-4">
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="flex items-center gap-1 text-xs text-muted-foreground">
                              <Clock className="h-3 w-3" />
                              {formatLastUsed(key.last_used_at, t, locale)}
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>{key.last_used_at ? new Date(key.last_used_at).toLocaleString(locale) : '-'}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </TableCell>
                    <TableCell className="px-4 text-xs text-muted-foreground">
                      {key.expires_at ? (
                        <span className={expired ? 'text-amber-600' : ''}>
                          {new Date(key.expires_at).toLocaleDateString(locale)}
                        </span>
                      ) : (
                        <span className="flex items-center gap-1">
                          <Shield className="h-3 w-3" />{t('apiKeys.neverExpires')}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="px-4 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => startEdit(key)} title={t('common.edit')}>
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button variant="ghost" size="sm" className="text-destructive" onClick={() => {
                          if (confirm(t('apiKeys.deleteConfirm', { name: key.name }))) deleteMutation.mutate(key.id)
                        }} title={t('common.delete')}>
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        )}
      </div>

      {/* Click outside to close batch menu */}
      {showBatchMenu && (
        <div className="fixed inset-0 z-40" onClick={() => setShowBatchMenu(false)} />
      )}
    </div>
  )
}
