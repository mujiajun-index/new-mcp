import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  adminListMarketplace, adminCreateMarketplace, adminDeleteMarketplace,
  adminBatchPricing, adminCloneMarketplace, adminListServices,
} from '../api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel, isExplicitlyPriced } from '@/lib/billing'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { toast } from 'sonner'
import {
  Plus, Copy, Trash2, CheckSquare, Square, Tag, AlertTriangle, Store,
} from 'lucide-react'

export function AdminMarketplacePage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { config } = useSystemConfigStore()

  const [page, setPage] = useState(1)
  const pageSize = 20
  const [selected, setSelected] = useState<Set<number>>(new Set())

  const [createOpen, setCreateOpen] = useState(false)
  const [cloneOpen, setCloneOpen] = useState(false)
  const [batchOpen, setBatchOpen] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-marketplace', page],
    queryFn: () => adminListMarketplace({ page, page_size: pageSize }),
  })
  const items: any[] = data?.data ?? []
  const pagination = data?.pagination
  const totalPages = pagination?.total_pages ?? 1

  const toggleSelect = (id: number) =>
    setSelected((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  const toggleSelectAll = () => {
    if (selected.size === items.length && items.length > 0) setSelected(new Set())
    else setSelected(new Set(items.map((i) => i.id)))
  }

  const deleteMutation = useMutation({
    mutationFn: adminDeleteMarketplace,
    onSuccess: () => {
      toast.success(t('common.delete'))
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  const createMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => adminCreateMarketplace(body),
    onSuccess: () => {
      toast.success(t('common.success'))
      setCreateOpen(false)
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  const cloneMutation = useMutation({
    mutationFn: (body: any) => adminCloneMarketplace(body),
    onSuccess: () => {
      toast.success(t('common.success'))
      setCloneOpen(false)
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  const batchMutation = useMutation({
    mutationFn: (items: { id: number; billing_type: string; price_per_call?: number }[]) =>
      adminBatchPricing({ items }),
    onSuccess: () => {
      toast.success(t('common.success'))
      setBatchOpen(false)
      setSelected(new Set())
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  const notSelfUse = !config.selfUseModeEnabled

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.adminMarketplace')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('marketplace.pricing')}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" className="gap-2" onClick={() => setCloneOpen(true)}>
            <Copy className="h-4 w-4" />{t('marketplace.clone')}
          </Button>
          <Button className="gap-2" onClick={() => setCreateOpen(true)}>
            <Plus className="h-4 w-4" />{t('common.create')}
          </Button>
        </div>
      </div>

      {notSelfUse && (
        <div className="flex items-start gap-2 rounded-xl border border-amber-500/30 bg-amber-500/5 p-4 text-xs text-amber-700 dark:text-amber-300">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{t('pricing.commercialNote')}</span>
        </div>
      )}

      {/* Batch bar */}
      {selected.size > 0 && (
        <div className="flex items-center gap-3 rounded-xl border bg-card p-3">
          <span className="text-sm text-muted-foreground">{t('apiKeys.selected', { count: selected.size })}</span>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setBatchOpen(true)}>
            <Tag className="h-3.5 w-3.5" />{t('marketplace.batchPricing')}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setSelected(new Set())}>{t('apiKeys.clearSelection')}</Button>
        </div>
      )}

      {/* Table */}
      <div className="rounded-xl border bg-card overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Store className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('pricing.empty')}</p>
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/50">
                <TableHead className="w-10 px-4">
                  <button onClick={toggleSelectAll} className="text-muted-foreground hover:text-foreground">
                    {selected.size === items.length && items.length > 0
                      ? <CheckSquare className="h-4 w-4" />
                      : <Square className="h-4 w-4" />}
                  </button>
                </TableHead>
                <TableHead className="px-4">{t('pricing.colService')}</TableHead>
                <TableHead className="px-4">{t('pricing.colCategory')}</TableHead>
                <TableHead className="px-4">{t('marketplace.billingType')}</TableHead>
                <TableHead className="px-4">{t('pricing.colPrice')}</TableHead>
                <TableHead className="px-4">{t('common.status')}</TableHead>
                <TableHead className="px-4 text-right">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => {
                const priced = isExplicitlyPriced(item.billing_type, item.price_per_call)
                return (
                  <TableRow key={item.id} data-state={selected.has(item.id) ? 'selected' : undefined}>
                    <TableCell className="px-4">
                      <button onClick={() => toggleSelect(item.id)} className="text-muted-foreground hover:text-foreground">
                        {selected.has(item.id) ? <CheckSquare className="h-4 w-4 text-primary" /> : <Square className="h-4 w-4" />}
                      </button>
                    </TableCell>
                    <TableCell className="px-4 font-medium">{item.display_name || item.name}</TableCell>
                    <TableCell className="px-4 text-xs text-muted-foreground">
                      {item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}
                    </TableCell>
                    <TableCell className="px-4 text-xs">
                      <Badge variant={item.billing_type === 'free' ? 'secondary' : 'outline'}>
                        {item.billing_type === 'free' ? t('marketplace.billingFree') : t('marketplace.billingPerCall')}
                      </Badge>
                    </TableCell>
                    <TableCell className="px-4">
                      <span className={`text-sm font-medium ${priced ? 'text-primary' : 'text-amber-600'}`}>
                        {priceLabel(item.billing_type, item.price_per_call, config.displayCurrency)}
                      </span>
                    </TableCell>
                    <TableCell className="px-4">
                      <Badge variant={item.status === 1 ? 'outline' : 'secondary'}
                        className={item.status === 1 ? 'text-emerald-600 border-emerald-300' : ''}>
                        {item.status === 1 ? t('common.enabled') : t('common.disabled')}
                      </Badge>
                    </TableCell>
                    <TableCell className="px-4 text-right">
                      <Button variant="ghost" size="sm" className="text-destructive" title={t('common.delete')}
                        onClick={() => { if (confirm(t('services.deleteConfirm', { name: item.display_name || item.name }))) deleteMutation.mutate(item.id) }}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        )}
      </div>

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

      <CreateDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onConfirm={(body) => createMutation.mutate(body)}
        pending={createMutation.isPending}
        notSelfUse={notSelfUse}
      />
      <CloneDialog
        open={cloneOpen}
        onOpenChange={setCloneOpen}
        onConfirm={(body) => cloneMutation.mutate(body)}
        pending={cloneMutation.isPending}
        notSelfUse={notSelfUse}
      />
      <BatchDialog
        open={batchOpen}
        onOpenChange={setBatchOpen}
        selectedIds={[...selected]}
        items={items}
        onConfirm={(batchItems) => batchMutation.mutate(batchItems)}
        pending={batchMutation.isPending}
      />
    </div>
  )
}

// --- Create dialog ---
function CreateDialog({
  open, onOpenChange, onConfirm, pending, notSelfUse,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  onConfirm: (body: Record<string, unknown>) => void
  pending: boolean
  notSelfUse: boolean
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState({
    name: '', display_name: '', description: '', category: 'instant',
    transport_type: 'streamable-http', billing_type: 'per_call', price_per_call: '0',
  })

  const submit = () => {
    const billingType = form.billing_type
    const price = parseFloat(form.price_per_call) || 0
    if (notSelfUse && billingType !== 'free' && price <= 0) {
      toast.error(t('pricing.commercialNote'))
      return
    }
    onConfirm({
      name: form.name,
      display_name: form.display_name || undefined,
      description: form.description || undefined,
      category: form.category,
      transport_type: form.transport_type,
      billing_type: billingType,
      price_per_call: billingType === 'free' ? 0 : price,
      status: 1,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('common.create')}</DialogTitle>
          <DialogDescription>{t('marketplace.platformHostedDesc')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t('services.serviceIdentifier')} <span className="text-destructive">*</span></Label>
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t('services.displayName')}</Label>
              <Input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
            </div>
          </div>
          <div className="space-y-2">
            <Label>{t('services.description')}</Label>
            <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t('pricing.colCategory')}</Label>
              <Select value={form.category} onValueChange={(v) => setForm({ ...form, category: v })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="instant">{t('marketplace.ready')}</SelectItem>
                  <SelectItem value="source">{t('marketplace.source')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t('services.transportType')}</Label>
              <Select value={form.transport_type} onValueChange={(v) => setForm({ ...form, transport_type: v })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {['stdio', 'sse', 'streamable-http', 'websocket', 'passive-ws'].map((tp) => (
                    <SelectItem key={tp} value={tp}>{tp}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4 border-t pt-4">
            <div className="space-y-2">
              <Label>{t('marketplace.billingType')}</Label>
              <Select value={form.billing_type} onValueChange={(v) => setForm({ ...form, billing_type: v, price_per_call: v === 'free' ? '0' : form.price_per_call })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="per_call">{t('marketplace.billingPerCall')}</SelectItem>
                  <SelectItem value="free">{t('marketplace.billingFree')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t('marketplace.pricePerCall')}</Label>
              <Input type="number" step="0.0001" disabled={form.billing_type === 'free'}
                value={form.price_per_call} onChange={(e) => setForm({ ...form, price_per_call: e.target.value })} />
            </div>
          </div>
          {notSelfUse && (
            <p className="text-xs text-amber-600">{t('pricing.commercialNote')}</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={pending || !form.name.trim()} onClick={submit}>{t('common.create')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// --- Clone dialog ---
function CloneDialog({
  open, onOpenChange, onConfirm, pending, notSelfUse,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  onConfirm: (body: any) => void
  pending: boolean
  notSelfUse: boolean
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState({
    from_service_id: '', name: '', display_name: '', description: '',
    billing_type: 'per_call', price_per_call: '0',
  })
  const { data: servicesData } = useQuery({
    queryKey: ['admin-services'],
    queryFn: adminListServices,
    enabled: open,
  })
  const services: any[] = servicesData?.data ?? []

  const submit = () => {
    const billingType = form.billing_type
    const price = parseFloat(form.price_per_call) || 0
    if (notSelfUse && billingType !== 'free' && price <= 0) {
      toast.error(t('pricing.commercialNote'))
      return
    }
    onConfirm({
      from_service_id: parseInt(form.from_service_id),
      name: form.name,
      display_name: form.display_name || undefined,
      description: form.description || undefined,
      billing_type: billingType,
      price_per_call: billingType === 'free' ? 0 : price,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('marketplace.clone')}</DialogTitle>
          <DialogDescription>{t('marketplace.platformHostedDesc')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t('marketplace.cloneFrom', { id: '' })} <span className="text-destructive">*</span></Label>
            <Select value={form.from_service_id} onValueChange={(v) => setForm({ ...form, from_service_id: v })}>
              <SelectTrigger><SelectValue placeholder={t('services.service')} /></SelectTrigger>
              <SelectContent>
                {services.map((s) => (
                  <SelectItem key={s.id} value={String(s.id)}>{s.display_name || s.name} (#{s.id})</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t('services.serviceIdentifier')} <span className="text-destructive">*</span></Label>
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>{t('services.displayName')}</Label>
              <Input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4 border-t pt-4">
            <div className="space-y-2">
              <Label>{t('marketplace.billingType')}</Label>
              <Select value={form.billing_type} onValueChange={(v) => setForm({ ...form, billing_type: v, price_per_call: v === 'free' ? '0' : form.price_per_call })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="per_call">{t('marketplace.billingPerCall')}</SelectItem>
                  <SelectItem value="free">{t('marketplace.billingFree')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t('marketplace.pricePerCall')}</Label>
              <Input type="number" step="0.0001" disabled={form.billing_type === 'free'}
                value={form.price_per_call} onChange={(e) => setForm({ ...form, price_per_call: e.target.value })} />
            </div>
          </div>
          <p className="flex items-start gap-2 rounded-lg bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-300">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            {t('marketplace.credentialReplaceHint')}
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={pending || !form.from_service_id || !form.name.trim()} onClick={submit}>
            {t('marketplace.clone')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// --- Batch pricing dialog ---
function BatchDialog({
  open, onOpenChange, selectedIds, items, onConfirm, pending,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  selectedIds: number[]
  items: any[]
  onConfirm: (batchItems: { id: number; billing_type: string; price_per_call?: number }[]) => void
  pending: boolean
}) {
  const { t } = useTranslation()
  const [billingType, setBillingType] = useState('per_call')
  const [price, setPrice] = useState('0')

  const selectedItems = items.filter((i) => selectedIds.includes(i.id))

  const submit = () => {
    const p = parseFloat(price) || 0
    onConfirm(
      selectedItems.map((i) => ({
        id: i.id,
        billing_type: billingType,
        price_per_call: billingType === 'free' ? undefined : p,
      }))
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('marketplace.batchPricingTitle')}</DialogTitle>
          <DialogDescription>{t('apiKeys.selected', { count: selectedItems.length })}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>{t('marketplace.billingType')}</Label>
              <Select value={billingType} onValueChange={(v) => { setBillingType(v); if (v === 'free') setPrice('0') }}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="per_call">{t('marketplace.billingPerCall')}</SelectItem>
                  <SelectItem value="free">{t('marketplace.billingFree')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>{t('marketplace.pricePerCall')}</Label>
              <Input type="number" step="0.0001" disabled={billingType === 'free'} value={price} onChange={(e) => setPrice(e.target.value)} />
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={pending || selectedItems.length === 0} onClick={submit}>{t('common.save')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
