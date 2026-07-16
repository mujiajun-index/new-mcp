import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  listRedemptions, createRedemptions, updateRedemptionStatus, deleteRedemption,
} from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { toast } from 'sonner'
import { Plus, Search, Trash2, Copy, Ticket, Ban, CheckCircle2 } from 'lucide-react'
import type { RedemptionItem } from '@/types'

const fmtTime = (s?: string) => (s ? new Date(s).toLocaleString() : '-')
const fmtExpiry = (unix: number) => {
  if (!unix) return null
  return new Date(unix * 1000).toLocaleString()
}

function copyText(text: string) {
  if (navigator.clipboard) return navigator.clipboard.writeText(text)
  return Promise.resolve()
}

function StatusBadge({ status, expired }: { status: number; expired: boolean }) {
  const { t } = useTranslation()
  if (status === 2) return <Badge variant="secondary">{t('redemptionCodes.statusRedeemed')}</Badge>
  if (status === 3) return <Badge variant="outline" className="text-zinc-500">{t('redemptionCodes.statusDisabled')}</Badge>
  if (expired) return <Badge variant="outline" className="text-amber-600 border-amber-300">{t('redemptionCodes.expired')}</Badge>
  return <Badge variant="outline" className="text-emerald-600 border-emerald-300 bg-emerald-50 dark:bg-emerald-950/30">{t('redemptionCodes.statusAvailable')}</Badge>
}

export function RedemptionsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const pageSize = 15

  // Create dialog
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState({ name: '', quota: '', count: '1', neverExpires: true, expires_at: '' })
  const [generated, setGenerated] = useState<RedemptionItem[] | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-redemptions', page, keyword, statusFilter],
    queryFn: () => listRedemptions({
      page,
      page_size: pageSize,
      keyword: keyword || undefined,
      status: statusFilter === 'all' ? undefined : Number(statusFilter),
    }),
  })

  const items: RedemptionItem[] = data?.data ?? []
  const pagination = data?.pagination
  const totalPages = pagination?.total_pages ?? 1

  const createMutation = useMutation({
    mutationFn: () => createRedemptions({
      name: form.name || undefined,
      quota: parseInt(form.quota) || 0,
      count: parseInt(form.count) || 1,
      expired_at: form.neverExpires ? 0 : (form.expires_at ? Math.floor(new Date(form.expires_at).getTime() / 1000) : 0),
    }),
    onSuccess: (res) => {
      const created: RedemptionItem[] = res?.data ?? []
      toast.success(t('redemptionCodes.createSuccess', { count: created.length }))
      setGenerated(created)
      setForm({ name: '', quota: '', count: '1', neverExpires: true, expires_at: '' })
      queryClient.invalidateQueries({ queryKey: ['admin-redemptions'] })
    },
    onError: () => toast.error(t('redemptionCodes.createFailed')),
  })

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) => updateRedemptionStatus(id, status),
    onSuccess: () => {
      toast.success(t('common.success'))
      queryClient.invalidateQueries({ queryKey: ['admin-redemptions'] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteRedemption,
    onSuccess: () => {
      toast.success(t('redemptionCodes.deleteSuccess'))
      queryClient.invalidateQueries({ queryKey: ['admin-redemptions'] })
    },
  })

  const nowSec = Math.floor(Date.now() / 1000)

  const copyAllCodes = () => {
    if (!generated?.length) return
    const text = generated.map((c) => c.code).join('\n')
    copyText(text).then(() => toast.success(t('common.copied')))
  }

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('redemptionCodes.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('redemptionCodes.subtitle')}</p>
        </div>
        <Button className="gap-2" onClick={() => { setCreateOpen(true); setGenerated(null) }}>
          <Plus className="h-4 w-4" />{t('redemptionCodes.create')}
        </Button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('redemptionCodes.searchPlaceholder')}
            value={keyword}
            onChange={(e) => { setKeyword(e.target.value); setPage(1) }}
            className="pl-9 h-9"
          />
        </div>
        <Select value={statusFilter} onValueChange={(v) => { setStatusFilter(v); setPage(1) }}>
          <SelectTrigger className="w-[140px] h-9">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('common.status')}</SelectItem>
            <SelectItem value="1">{t('redemptionCodes.statusAvailable')}</SelectItem>
            <SelectItem value="2">{t('redemptionCodes.statusRedeemed')}</SelectItem>
            <SelectItem value="3">{t('redemptionCodes.statusDisabled')}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Table */}
      <div className="rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Ticket className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('redemptionCodes.noCodes')}</p>
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {items.map((item) => {
              const expired = item.status === 1 && item.expired_at > 0 && item.expired_at < nowSec
              return (
                <MobileListCard
                  key={item.id}
                  title={<code className="font-mono text-xs">{item.code}</code>}
                  badge={<StatusBadge status={item.status} expired={expired} />}
                  meta={[
                    { label: t('redemptionCodes.name'), value: item.name || '-' },
                    { label: t('redemptionCodes.quota'), value: <span className="tabular-nums">{item.quota}</span> },
                    { label: t('redemptionCodes.expiry'), value: fmtExpiry(item.expired_at) || t('redemptionCodes.neverExpires') },
                    { label: t('redemptionCodes.createdAt'), value: fmtTime(item.created_at) },
                  ]}
                  actions={
                    <div className="flex items-center gap-1">
                      <Button variant="ghost" size="sm" onClick={() => { copyText(item.code); toast.success(t('common.copied')) }}>
                        <Copy className="h-3.5 w-3.5" />
                      </Button>
                      {item.status === 1 && (
                        <Button variant="ghost" size="sm" onClick={() => statusMutation.mutate({ id: item.id, status: 3 })} title={t('redemptionCodes.disable')}>
                          <Ban className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      {item.status === 3 && (
                        <Button variant="ghost" size="sm" onClick={() => statusMutation.mutate({ id: item.id, status: 1 })} title={t('redemptionCodes.enable')}>
                          <CheckCircle2 className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive"
                        onClick={() => { if (confirm(t('redemptionCodes.deleteConfirm'))) deleteMutation.mutate(item.id) }}
                        title={t('redemptionCodes.delete')}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  }
                />
              )
            })}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('redemptionCodes.code')}</TableHead>
                <TableHead>{t('redemptionCodes.name')}</TableHead>
                <TableHead>{t('redemptionCodes.quota')}</TableHead>
                <TableHead>{t('redemptionCodes.status')}</TableHead>
                <TableHead>{t('redemptionCodes.expiry')}</TableHead>
                <TableHead>{t('redemptionCodes.redeemedAt')}</TableHead>
                <TableHead className="text-right">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => {
                const expired = item.status === 1 && item.expired_at > 0 && item.expired_at < nowSec
                return (
                  <TableRow key={item.id}>
                    <TableCell>
                      <button
                        className="font-mono text-xs text-muted-foreground hover:text-foreground transition-colors"
                        title={t('redemptionCodes.copyCode')}
                        onClick={() => { copyText(item.code); toast.success(t('common.copied')) }}
                      >
                        {item.code}
                      </button>
                    </TableCell>
                    <TableCell className="text-sm">{item.name || '-'}</TableCell>
                    <TableCell className="text-sm tabular-nums">{item.quota}</TableCell>
                    <TableCell><StatusBadge status={item.status} expired={expired} /></TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {fmtExpiry(item.expired_at) || t('redemptionCodes.neverExpires')}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground tabular-nums">{fmtTime(item.redeemed_at)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        {item.status === 1 && (
                          <Button variant="ghost" size="sm" onClick={() => statusMutation.mutate({ id: item.id, status: 3 })} title={t('redemptionCodes.disable')}>
                            <Ban className="h-3.5 w-3.5" />
                          </Button>
                        )}
                        {item.status === 3 && (
                          <Button variant="ghost" size="sm" onClick={() => statusMutation.mutate({ id: item.id, status: 1 })} title={t('redemptionCodes.enable')}>
                            <CheckCircle2 className="h-3.5 w-3.5" />
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive"
                          onClick={() => { if (confirm(t('redemptionCodes.deleteConfirm'))) deleteMutation.mutate(item.id) }}
                          title={t('redemptionCodes.delete')}
                        >
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

      {/* Pagination */}
      {pagination && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">{t('common.total')} {pagination.total} {t('common.items')}</p>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>‹</Button>
            <span className="text-sm tabular-nums">{page} / {totalPages}</span>
            <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>›</Button>
          </div>
        </div>
      )}

      {/* Create dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('redemptionCodes.createTitle')}</DialogTitle>
            {!generated && <DialogDescription>{t('redemptionCodes.countHint')}</DialogDescription>}
          </DialogHeader>

          {generated ? (
            <div className="space-y-3">
              <div className="max-h-64 overflow-auto rounded-lg border bg-muted/40 p-3">
                {generated.map((c) => (
                  <div key={c.id} className="flex items-center justify-between py-1">
                    <code className="font-mono text-xs">{c.code}</code>
                    <Button variant="ghost" size="sm" className="h-6" onClick={() => { copyText(c.code); toast.success(t('common.copied')) }}>
                      <Copy className="h-3 w-3" />
                    </Button>
                  </div>
                ))}
              </div>
              <Button variant="outline" size="sm" className="w-full gap-1.5" onClick={copyAllCodes}>
                <Copy className="h-3.5 w-3.5" />{t('common.copy')}
              </Button>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>{t('redemptionCodes.name')}</Label>
                <Input
                  placeholder={t('redemptionCodes.namePlaceholder')}
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t('redemptionCodes.quota')} <span className="text-destructive">*</span></Label>
                  <Input
                    type="number"
                    placeholder={t('redemptionCodes.quotaPlaceholder')}
                    value={form.quota}
                    onChange={(e) => setForm({ ...form, quota: e.target.value })}
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('redemptionCodes.count')}</Label>
                  <Input
                    type="number"
                    min={1}
                    max={100}
                    value={form.count}
                    onChange={(e) => setForm({ ...form, count: e.target.value })}
                  />
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t('redemptionCodes.expiry')}</Label>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant={form.neverExpires ? 'default' : 'outline'}
                    size="sm"
                    onClick={() => setForm({ ...form, neverExpires: true, expires_at: '' })}
                  >
                    {t('redemptionCodes.neverExpires')}
                  </Button>
                  <Input
                    type="datetime-local"
                    disabled={form.neverExpires}
                    value={form.expires_at ? form.expires_at.slice(0, 16) : ''}
                    onChange={(e) => setForm({ ...form, neverExpires: false, expires_at: e.target.value ? new Date(e.target.value).toISOString() : '' })}
                  />
                </div>
              </div>
            </div>
          )}

          <DialogFooter>
            {generated ? (
              <>
                <Button variant="outline" onClick={() => { setGenerated(null); setCreateOpen(false) }}>{t('common.close')}</Button>
                <Button onClick={() => { setGenerated(null) }}>{t('redemptionCodes.create')}</Button>
              </>
            ) : (
              <>
                <Button variant="outline" onClick={() => setCreateOpen(false)}>{t('common.cancel')}</Button>
                <Button
                  disabled={createMutation.isPending || !form.quota || parseInt(form.quota) <= 0}
                  onClick={() => createMutation.mutate()}
                >
                  {t('redemptionCodes.create')}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
