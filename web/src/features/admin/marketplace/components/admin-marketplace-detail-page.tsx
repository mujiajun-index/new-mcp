import { useState } from 'react'
import { useParams, useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { adminGetMarketplace, adminUpdateMarketplace } from '../api'
import { adminListMarketplaceGroups, adminListMarketplaceTags } from '@/features/admin/marketplace-categories/api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel } from '@/lib/billing'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from 'sonner'
import { ArrowLeft, Save } from 'lucide-react'
import type { MarketplaceDetail } from '@/types'

// AdminMarketplaceDetailPage 市场项详情 + 编辑(§11)。上半只读概览,下半编辑表单(调 adminUpdateMarketplace)。
export function AdminMarketplaceDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams({ from: '/_authenticated/admin/marketplace/$id' })
  const queryClient = useQueryClient()
  const { config } = useSystemConfigStore()
  const notSelfUse = !config.selfUseModeEnabled

  const { data, isLoading } = useQuery({
    queryKey: ['admin-marketplace-detail', id],
    queryFn: () => adminGetMarketplace(Number(id)),
  })
  const item: MarketplaceDetail | undefined = data?.data

  const { data: groupsData } = useQuery({
    queryKey: ['admin-marketplace-groups'],
    queryFn: () => adminListMarketplaceGroups(),
  })
  const groups: any[] = groupsData?.data ?? []
  const { data: tagsData } = useQuery({
    queryKey: ['admin-marketplace-tags'],
    queryFn: () => adminListMarketplaceTags({ page: 1, page_size: 200 }),
  })
  const tagsLib: any[] = tagsData?.data ?? []

  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: number; body: Record<string, unknown> }) => adminUpdateMarketplace(id, body),
    onSuccess: () => {
      toast.success(t('common.success'))
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace-detail', id] })
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  if (isLoading || !item) {
    return <div className="p-8 text-sm text-muted-foreground">{t('common.loading')}</div>
  }

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <Button variant="ghost" size="sm" className="gap-1.5 w-fit" onClick={() => navigate({ to: '/admin/marketplace' })}>
        <ArrowLeft className="h-4 w-4" />{t('marketplace.backToMarketplace')}
      </Button>

      {/* 概览(只读) */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="text-xl font-semibold">{item.display_name || item.name}</h2>
        <p className="mt-1 text-sm text-muted-foreground">{item.name} · v{item.version}</p>
        <div className="mt-3 flex flex-wrap gap-2 text-xs">
          <Badge variant="outline">{item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}</Badge>
          {item.group_name && <Badge variant="secondary">{item.group_name}</Badge>}
          {item.tags?.map((tag) => <Badge key={tag} variant="outline" className="font-normal">{tag}</Badge>)}
          <Badge variant={item.billing_type === 'free' ? 'secondary' : 'outline'}>
            {priceLabel(item.billing_type, item.price_per_call, config.displayCurrency)}
          </Badge>
          <Badge variant={item.status === 1 ? 'outline' : 'secondary'}
            className={item.status === 1 ? 'text-emerald-600 border-emerald-300' : ''}>
            {item.status === 1 ? t('common.enabled') : t('common.disabled')}
          </Badge>
        </div>
        <div className="mt-3 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div><span className="text-muted-foreground">{t('marketplace.installs')}</span>: {item.install_count}</div>
          <div><span className="text-muted-foreground">{t('marketplace.rating')}</span>: {item.rating_count > 0 ? item.rating_avg.toFixed(1) : '-'}</div>
          <div><span className="text-muted-foreground">{t('services.transportType')}</span>: {item.transport_type}</div>
          <div><span className="text-muted-foreground">{t('common.createdAt')}</span>: {item.created_at.slice(0, 10)}</div>
        </div>
        {item.description && <p className="mt-3 text-sm text-muted-foreground">{item.description}</p>}
      </div>

      {/* 编辑表单 */}
      <EditForm item={item} groups={groups} tagsLib={tagsLib} notSelfUse={notSelfUse}
        onSave={(body) => updateMutation.mutate({ id: Number(id), body })} pending={updateMutation.isPending} />
    </div>
  )
}

function EditForm({ item, groups, tagsLib, notSelfUse, onSave, pending }: {
  item: MarketplaceDetail
  groups: any[]
  tagsLib: any[]
  notSelfUse: boolean
  onSave: (body: Record<string, unknown>) => void
  pending: boolean
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState({
    display_name: item.display_name,
    description: item.description,
    icon_url: item.icon_url,
    version: item.version,
    group_id: item.group_id ? String(item.group_id) : '',
    billing_type: item.billing_type,
    price_per_call: String(item.price_per_call),
    status: String(item.status),
  })
  const [selectedTags, setSelectedTags] = useState<string[]>(item.tags ?? [])

  const toggleTag = (name: string) =>
    setSelectedTags((prev) => (prev.includes(name) ? prev.filter((x) => x !== name) : [...prev, name]))

  const submit = () => {
    const billingType = form.billing_type
    const price = parseFloat(form.price_per_call) || 0
    if (price < 0) { toast.error(t('marketplace.priceNegative')); return }
    if (notSelfUse && billingType !== 'free' && price <= 0) { toast.error(t('pricing.commercialNote')); return }
    onSave({
      display_name: form.display_name,
      description: form.description,
      icon_url: form.icon_url,
      version: form.version,
      group_id: form.group_id ? Number(form.group_id) : null,
      tags: selectedTags,
      billing_type: billingType,
      price_per_call: billingType === 'free' ? 0 : price,
      status: Number(form.status),
    })
  }

  return (
    <div className="space-y-4 rounded-xl border bg-card p-5">
      <h3 className="font-semibold">{t('common.edit')}</h3>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>{t('services.displayName')}</Label>
          <Input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label>{t('services.iconUrl')}</Label>
          <Input value={form.icon_url} onChange={(e) => setForm({ ...form, icon_url: e.target.value })} />
        </div>
      </div>
      <div className="space-y-2">
        <Label>{t('services.description')}</Label>
        <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>{t('marketplace.version')}</Label>
          <Input value={form.version} onChange={(e) => setForm({ ...form, version: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label>{t('categories.groups')}</Label>
          <Select value={form.group_id || '__none__'} onValueChange={(v) => setForm({ ...form, group_id: v === '__none__' ? '' : v })}>
            <SelectTrigger><SelectValue placeholder={t('marketplace.noGroup')} /></SelectTrigger>
            <SelectContent>
              <SelectItem value="__none__">{t('marketplace.noGroup')}</SelectItem>
              {groups.map((g) => <SelectItem key={g.id} value={String(g.id)}>{g.name}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="space-y-2">
        <Label>{t('marketplace.tags')}</Label>
        <div className="flex flex-wrap gap-2">
          {tagsLib.length === 0 && <p className="text-xs text-muted-foreground">{t('marketplace.noTagsHint')}</p>}
          {tagsLib.map((tag) => {
            const selected = selectedTags.includes(tag.name)
            return (
              <button key={tag.id} type="button" onClick={() => toggleTag(tag.name)}
                className={`rounded-full border px-3 py-1 text-xs transition-colors ${selected ? 'border-primary bg-primary text-primary-foreground' : 'bg-muted/40 hover:bg-muted'}`}>
                {tag.name}
              </button>
            )
          })}
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
          <Input type="number" min="0" step="0.0001" disabled={form.billing_type === 'free'}
            value={form.price_per_call} onChange={(e) => setForm({ ...form, price_per_call: e.target.value })} />
        </div>
      </div>
      <div className="space-y-2">
        <Label>{t('common.status')}</Label>
        <Select value={form.status} onValueChange={(v) => setForm({ ...form, status: v })}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="1">{t('common.enabled')}</SelectItem>
            <SelectItem value="2">{t('common.disabled')}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="flex justify-end">
        <Button className="gap-2" disabled={pending} onClick={submit}><Save className="h-4 w-4" />{t('common.save')}</Button>
      </div>
    </div>
  )
}
