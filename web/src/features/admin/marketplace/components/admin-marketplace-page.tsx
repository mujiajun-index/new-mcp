import { useEffect, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  adminListMarketplace, adminUpdateMarketplace, adminDeleteMarketplace,
  adminBatchPricing, adminBatchSetGroupsTags, adminCloneMarketplace, adminListCloneSources,
  adminGetMarketplaceHealth,
} from '../api'
import { adminListMarketplaceGroups, adminListMarketplaceTags } from '@/features/admin/marketplace-categories/api'
import { ServiceHealthBar } from '@/features/services/components/service-health-bar'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel, isExplicitlyPriced, PRICE_MAX, PRICE_MIN } from '@/lib/billing'
import type { BatchGroupsTagsReq, MarketplaceItemHealth } from '@/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { toast } from 'sonner'
import {
  Copy, Trash2, CheckSquare, Square, Tag, Tags, FolderTree, Check, AlertTriangle, Store, Eye, Search,
} from 'lucide-react'

export function AdminMarketplacePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { config } = useSystemConfigStore()
  const isMobile = useIsMobile()

  const [page, setPage] = useState(1)
  const pageSize = 20
  const [selected, setSelected] = useState<Set<number>>(new Set())

  // 筛选:搜索词(提交生效)+状态/类型/分组/标签;默认只看已启用,状态 0=全部
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState(1)
  const [category, setCategory] = useState('')
  const [groupId, setGroupId] = useState<number | ''>('')
  const [tag, setTag] = useState('')

  const [cloneOpen, setCloneOpen] = useState(false)
  const [batchOpen, setBatchOpen] = useState(false)
  const [groupsOpen, setGroupsOpen] = useState(false)
  const [tagsOpen, setTagsOpen] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-marketplace', page, keyword, status, category, groupId, tag],
    queryFn: () => adminListMarketplace({
      page,
      page_size: pageSize,
      status: status || undefined,
      keyword: keyword || undefined,
      category: category || undefined,
      group_id: groupId || undefined,
      tag: tag || undefined,
    }),
  })
  // 平台级健康:全部条目同条目下全部用户引用行的调用聚合;30s 轮询对齐后端缓存
  const { data: healthData } = useQuery({
    queryKey: ['admin-marketplace-health'],
    queryFn: adminGetMarketplaceHealth,
    refetchInterval: 30_000,
  })
  const healthById: Record<string, MarketplaceItemHealth> = healthData?.data ?? {}
  const items: any[] = data?.data ?? []
  const pagination = data?.pagination
  const totalPages = pagination?.total_pages ?? 1

  // 筛选字典:分组仅启用(禁用分组的绑定已在禁用时同事务摘除);标签取启用字典
  const { data: groupsData } = useQuery({
    queryKey: ['admin-marketplace-groups'],
    queryFn: () => adminListMarketplaceGroups(),
  })
  const groups: any[] = (groupsData?.data ?? []).filter((g: any) => g.status === 1)
  const { data: tagsData } = useQuery({
    queryKey: ['admin-marketplace-tags'],
    queryFn: () => adminListMarketplaceTags({ page: 1, page_size: 200, status: 1 }),
  })
  const tagsLib: any[] = tagsData?.data ?? []

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

  // 行内启用/禁用:复用 adminUpdateMarketplace({status})。失败时(如非自用模式启用未定价项)
  // 由全局响应拦截器 toast 错误信息,开关因数据未变而保持在原位。
  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) => adminUpdateMarketplace(id, { status }),
    onSuccess: () => {
      toast.success(t('common.success'))
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

  // 批量设置分组/标签(替换语义,两个独立入口共用端点):ids 取整个选中集(含跨页),不按当前页过滤
  const classifyMutation = useMutation({
    mutationFn: (body: BatchGroupsTagsReq) => adminBatchSetGroupsTags(body),
    onSuccess: () => {
      toast.success(t('common.success'))
      setGroupsOpen(false)
      setTagsOpen(false)
      setSelected(new Set())
      queryClient.invalidateQueries({ queryKey: ['admin-marketplace'] })
    },
  })

  const notSelfUse = !config.selfUseModeEnabled

  // 列宽权重(px),与调用日志列表同口径:经 <colgroup> 换算成各列占总宽的百分比,
  // 表格 w-full 时所有列等比例同步伸缩;视口窄于权重总和时表格按 minWidth(=总和)
  // 横向滚动,此时每列恰好等于其 px 权重。服务名列 truncate 防长名撑行,健康
  // 色带列(20 格 flex-1)吃伸缩余量。
  const columns = [
    { key: 'check', w: 40 },
    { key: 'service', w: 220 },
    { key: 'category', w: 88 },
    { key: 'sort', w: 64 },
    { key: 'billing', w: 104 },
    { key: 'price', w: 88 },
    { key: 'health', w: 240 },
    { key: 'status', w: 112 },
    { key: 'actions', w: 100 },
  ]
  const columnsTotal = columns.reduce((s, c) => s + c.w, 0)

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.adminMarketplace')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('marketplace.pricing')}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button className="gap-2" onClick={() => setCloneOpen(true)}>
            <Copy className="h-4 w-4" />{t('marketplace.clone')}
          </Button>
        </div>
      </div>

      {notSelfUse && (
        <div className="flex items-start gap-2 rounded-xl border border-amber-500/30 bg-amber-500/5 p-4 text-xs text-amber-700 dark:text-amber-300">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{t('pricing.commercialNote')}</span>
        </div>
      )}

      {/* Filters:搜索(提交生效)+状态/类型/分组/标签下拉(默认已启用) */}
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <form
          onSubmit={(e) => { e.preventDefault(); setKeyword(searchInput); setPage(1) }}
          className="relative w-full max-w-sm flex-1"
        >
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input placeholder={t('marketplace.adminSearchPlaceholder')} value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)} className="pl-9" />
        </form>
        {/* Radix Select 不允许空串 value,「全部」用哨兵 all 表示不过滤 */}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <label className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{t('common.status')}</span>
            <Select value={String(status)} onValueChange={(v) => { setStatus(Number(v)); setPage(1) }}>
              <SelectTrigger className="w-[92px]"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="1">{t('common.enabled')}</SelectItem>
                <SelectItem value="2">{t('common.disabled')}</SelectItem>
                <SelectItem value="0">{t('marketplace.filterAll')}</SelectItem>
              </SelectContent>
            </Select>
          </label>
          <label className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{t('pricing.colCategory')}</span>
            <Select value={category || 'all'} onValueChange={(v) => { setCategory(v === 'all' ? '' : v); setPage(1) }}>
              <SelectTrigger className="w-[92px]"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('marketplace.filterAll')}</SelectItem>
                <SelectItem value="instant">{t('marketplace.filterReady')}</SelectItem>
                <SelectItem value="source">{t('marketplace.filterSource')}</SelectItem>
              </SelectContent>
            </Select>
          </label>
          <label className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{t('categories.groups')}</span>
            <Select value={groupId === '' ? 'all' : String(groupId)}
              onValueChange={(v) => { setGroupId(v === 'all' ? '' : Number(v)); setPage(1) }}>
              <SelectTrigger className="w-[120px]"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('marketplace.filterAll')}</SelectItem>
                {groups.map((g) => (
                  <SelectItem key={g.id} value={String(g.id)}>{g.display_name || g.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </label>
          <label className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{t('categories.tags')}</span>
            <Select value={tag || 'all'} onValueChange={(v) => { setTag(v === 'all' ? '' : v); setPage(1) }}>
              <SelectTrigger className="w-[110px]"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('marketplace.filterAll')}</SelectItem>
                {tagsLib.map((tg) => (
                  <SelectItem key={tg.id} value={tg.name}>{tg.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </label>
        </div>
      </div>

      {/* Batch bar */}
      {selected.size > 0 && (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card p-3">
          <span className="text-sm text-muted-foreground">{t('apiKeys.selected', { count: selected.size })}</span>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setBatchOpen(true)}>
            <Tag className="h-3.5 w-3.5" />{t('marketplace.batchPricing')}
          </Button>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setGroupsOpen(true)}>
            <FolderTree className="h-3.5 w-3.5" />{t('marketplace.batchSetGroups')}
          </Button>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setTagsOpen(true)}>
            <Tags className="h-3.5 w-3.5" />{t('marketplace.batchSetTags')}
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
        ) : isMobile ? (
          <div className="divide-y">
            {items.map((item) => {
              const priced = isExplicitlyPriced(item.billing_type, item.price_per_call)
              // 价格分色:免费绿(对齐市场列表卡)、已定价主题色、未定价琥珀警示
              const priceColor = item.billing_type === 'free'
                ? 'text-emerald-600 dark:text-emerald-400'
                : priced ? 'text-primary' : 'text-amber-600'
              return (
                <MobileListCard
                  key={item.id}
                  className={selected.has(item.id) ? 'bg-muted/40' : undefined}
                  title={
                    <div className="flex items-center gap-2">
                      <button onClick={() => toggleSelect(item.id)} className="shrink-0 text-muted-foreground hover:text-foreground">
                        {selected.has(item.id) ? <CheckSquare className="h-4 w-4 text-primary" /> : <Square className="h-4 w-4" />}
                      </button>
                      <span className="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded bg-primary/10 text-primary">
                        {item.icon_url
                          ? <img src={item.icon_url} alt="" className="h-3.5 w-3.5" />
                          : <span className="text-[10px] font-bold">{(item.display_name || item.name).charAt(0)}</span>}
                      </span>
                      <span className="truncate">{item.display_name || item.name}</span>
                    </div>
                  }
                  badge={
                    <Badge variant={item.billing_type === 'free' ? 'secondary' : 'outline'}>
                      {item.billing_type === 'free' ? t('marketplace.billingFree') : t('marketplace.billingPerCall')}
                    </Badge>
                  }
                  meta={[
                    { label: t('pricing.colCategory'), value: item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source') },
                    { label: t('marketplace.sortOrder'), value: <span className="tabular-nums">{item.sort_order}</span> },
                    {
                      label: t('pricing.colPrice'),
                      value: (
                        <span className={priceColor}>
                          {priceLabel(item.billing_type, item.price_per_call, config.displayCurrency)}
                        </span>
                      ),
                    },
                  ]}
                  note={
                    <ServiceHealthBar
                      s={healthById[String(item.id)] ?? {}}
                      wide
                      disabled={item.status !== 1}
                    />
                  }
                  actions={
                    <>
                      <div className="mr-auto flex items-center gap-2">
                        <Switch
                          checked={item.status === 1}
                          onCheckedChange={(checked) => statusMutation.mutate({ id: item.id, status: checked ? 1 : 2 })}
                          disabled={statusMutation.isPending && statusMutation.variables?.id === item.id}
                        />
                        <span className={`text-xs ${item.status === 1 ? 'text-emerald-600' : 'text-muted-foreground'}`}>
                          {item.status === 1 ? t('common.enabled') : t('common.disabled')}
                        </span>
                      </div>
                      <Button variant="ghost" size="sm" title={t('common.details')}
                        onClick={() => navigate({ to: '/admin/marketplace/$id', params: { id: String(item.id) } })}>
                        <Eye className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" title={t('common.delete')}
                        onClick={() => { if (confirm(t('services.deleteConfirm', { name: item.display_name || item.name }))) deleteMutation.mutate(item.id) }}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </>
                  }
                />
              )
            })}
          </div>
        ) : (
          <Table className="table-fixed" style={{ minWidth: `${columnsTotal}px` }}>
            <colgroup>
              {columns.map((c) => (
                <col key={c.key} style={{ width: `${(c.w / columnsTotal) * 100}%` }} />
              ))}
            </colgroup>
            <TableHeader>
              <TableRow className="bg-muted/50">
                <TableHead className="whitespace-nowrap">
                  <button onClick={toggleSelectAll} className="text-muted-foreground hover:text-foreground">
                    {selected.size === items.length && items.length > 0
                      ? <CheckSquare className="h-4 w-4" />
                      : <Square className="h-4 w-4" />}
                  </button>
                </TableHead>
                <TableHead className="whitespace-nowrap">{t('pricing.colService')}</TableHead>
                <TableHead className="whitespace-nowrap">{t('pricing.colCategory')}</TableHead>
                <TableHead className="whitespace-nowrap">{t('marketplace.sortOrder')}</TableHead>
                <TableHead className="whitespace-nowrap">{t('marketplace.billingType')}</TableHead>
                <TableHead className="whitespace-nowrap">{t('pricing.colPrice')}</TableHead>
                <TableHead className="whitespace-nowrap">{t('marketplace.healthCol')}</TableHead>
                <TableHead className="whitespace-nowrap">{t('common.status')}</TableHead>
                <TableHead className="whitespace-nowrap text-right">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => {
                const priced = isExplicitlyPriced(item.billing_type, item.price_per_call)
                const priceColor = item.billing_type === 'free'
                  ? 'text-emerald-600 dark:text-emerald-400'
                  : priced ? 'text-primary' : 'text-amber-600'
                return (
                  <TableRow key={item.id} data-state={selected.has(item.id) ? 'selected' : undefined}>
                    <TableCell>
                      <button onClick={() => toggleSelect(item.id)} className="text-muted-foreground hover:text-foreground">
                        {selected.has(item.id) ? <CheckSquare className="h-4 w-4 text-primary" /> : <Square className="h-4 w-4" />}
                      </button>
                    </TableCell>
                    <TableCell className="font-medium" title={item.display_name || item.name}>
                      <div className="flex min-w-0 items-center gap-2">
                        {/* 服务图标:无图标时回退首字母,与市场广场同款 */}
                        <span className="flex h-6 w-6 shrink-0 items-center justify-center overflow-hidden rounded-md bg-primary/10 text-primary">
                          {item.icon_url
                            ? <img src={item.icon_url} alt="" className="h-4 w-4" />
                            : <span className="text-xs font-bold">{(item.display_name || item.name).charAt(0)}</span>}
                        </span>
                        <span className="truncate">{item.display_name || item.name}</span>
                      </div>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      {item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs tabular-nums text-muted-foreground">
                      {item.sort_order}
                    </TableCell>
                    <TableCell className="text-xs">
                      <Badge variant={item.billing_type === 'free' ? 'secondary' : 'outline'}>
                        {item.billing_type === 'free' ? t('marketplace.billingFree') : t('marketplace.billingPerCall')}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <span className={`text-sm font-medium ${priceColor}`}>
                        {priceLabel(item.billing_type, item.price_per_call, config.displayCurrency)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <ServiceHealthBar
                        s={healthById[String(item.id)] ?? {}}
                        wide
                        disabled={item.status !== 1}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2 whitespace-nowrap">
                        <Switch
                          checked={item.status === 1}
                          onCheckedChange={(checked) => statusMutation.mutate({ id: item.id, status: checked ? 1 : 2 })}
                          disabled={statusMutation.isPending && statusMutation.variables?.id === item.id}
                        />
                        <span className={`text-xs ${item.status === 1 ? 'text-emerald-600' : 'text-muted-foreground'}`}>
                          {item.status === 1 ? t('common.enabled') : t('common.disabled')}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right">
                      <Button variant="ghost" size="sm" title={t('common.details')}
                        onClick={() => navigate({ to: '/admin/marketplace/$id', params: { id: String(item.id) } })}>
                        <Eye className="h-3.5 w-3.5" />
                      </Button>
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
      <BatchClassifyDialog
        kind="groups"
        open={groupsOpen}
        onOpenChange={setGroupsOpen}
        selectedCount={selected.size}
        groups={groups}
        onConfirm={(body) => classifyMutation.mutate({ ids: [...selected], ...body })}
        pending={classifyMutation.isPending}
      />
      <BatchClassifyDialog
        kind="tags"
        open={tagsOpen}
        onOpenChange={setTagsOpen}
        selectedCount={selected.size}
        tagsLib={tagsLib}
        onConfirm={(body) => classifyMutation.mutate({ ids: [...selected], ...body })}
        pending={classifyMutation.isPending}
      />
    </div>
  )
}

// --- Clone dialog(从自有服务克隆上架:唯一上架入口)---
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
    isolated_process: false,
  })
  const { data: servicesData } = useQuery({
    queryKey: ['marketplace-clone-sources'],
    queryFn: adminListCloneSources,
    enabled: open,
  })
  const services: any[] = servicesData?.data ?? []
  // 进程模式开关仅对 stdio 源有意义(其余传输无平台子进程,后端也会忽略)
  const srcSvc = services.find((s) => String(s.id) === form.from_service_id)
  const srcIsStdio = srcSvc?.transport_type === 'stdio'

  // 每次打开重置表单,避免残留上次的选择
  useEffect(() => {
    if (open) {
      setForm({
        from_service_id: '', name: '', display_name: '', description: '',
        billing_type: 'per_call', price_per_call: '0',
        isolated_process: false,
      })
    }
  }, [open])

  // 选择服务后自动回显标识/名称/描述(一般无需修改,直接提交即可快速上架)
  const selectService = (v: string) => {
    const svc = services.find((s) => String(s.id) === v)
    setForm((prev) => ({
      ...prev,
      from_service_id: v,
      name: svc?.name ?? '',
      display_name: svc?.display_name ?? '',
      description: svc?.description ?? '',
    }))
  }

  const submit = () => {
    const billingType = form.billing_type
    const price = parseFloat(form.price_per_call) || 0
    if (price < 0) {
      toast.error(t('marketplace.priceNegative'))
      return
    }
    if (price > 0 && (price < PRICE_MIN || price > PRICE_MAX)) {
      toast.error(t('marketplace.priceRange'))
      return
    }
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
      isolated_process: srcIsStdio ? form.isolated_process : false,
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
            <Label>{t('services.service')} <span className="text-destructive">*</span></Label>
            <Select value={form.from_service_id} onValueChange={selectService}>
              <SelectTrigger><SelectValue placeholder={t('services.service')} /></SelectTrigger>
              <SelectContent>
                {services.map((s) => (
                  <SelectItem key={s.id} value={String(s.id)}>{s.display_name || s.name} (#{s.id})</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
          <div className="grid grid-cols-1 gap-4 border-t pt-4 sm:grid-cols-2">
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
              <Input type="number" min="0.001" max="999" step="0.001" disabled={form.billing_type === 'free'}
                value={form.price_per_call} onChange={(e) => setForm({ ...form, price_per_call: e.target.value })} />
            </div>
          </div>
          {/* 进程模式(仅 stdio 源显示):默认共享;上架后可在详情页切换(会重启进程) */}
          {srcIsStdio && (
            <div className="space-y-2">
              <Label>{t('marketplace.processMode')}</Label>
              <div className="flex flex-wrap gap-2">
                {([false, true] as const).map((isolated) => (
                  <button
                    key={String(isolated)}
                    type="button"
                    onClick={() => setForm({ ...form, isolated_process: isolated })}
                    className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                      form.isolated_process === isolated ? 'border-primary bg-primary/5' : 'hover:border-primary/30'
                    }`}
                  >
                    {isolated ? t('marketplace.modeIsolated') : t('marketplace.modeShared')}
                  </button>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">
                {form.isolated_process ? t('marketplace.modeIsolatedHint') : t('marketplace.modeSharedHint')}
              </p>
            </div>
          )}
          <p className="flex items-start gap-2 rounded-lg bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-300">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            {t('marketplace.credentialReplaceHint')}
          </p>
        </div>
        <DialogFooter className="mt-4">
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
    if (p < 0) {
      toast.error(t('marketplace.priceNegative'))
      return
    }
    if (p > 0 && (p < PRICE_MIN || p > PRICE_MAX)) {
      toast.error(t('marketplace.priceRange'))
      return
    }
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
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
              <Input type="number" min="0.001" max="999" step="0.001" disabled={billingType === 'free'} value={price} onChange={(e) => setPrice(e.target.value)} />
            </div>
          </div>
        </div>
        <DialogFooter className="mt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={pending || selectedItems.length === 0} onClick={submit}>{t('common.save')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// --- Batch classify dialog(批量设置分组/标签,替换语义:所选内容整体替换,空选=清空)---
// 分组、标签两个独立入口共用本组件(kind 区分),chip 样式同详情页编辑表单。
function BatchClassifyDialog({
  kind, open, onOpenChange, selectedCount, groups, tagsLib, onConfirm, pending,
}: {
  kind: 'groups' | 'tags'
  open: boolean
  onOpenChange: (v: boolean) => void
  selectedCount: number
  groups?: any[]
  tagsLib?: any[]
  onConfirm: (body: { group_ids?: number[]; tags?: string[] }) => void
  pending: boolean
}) {
  const { t } = useTranslation()
  const [selectedGroups, setSelectedGroups] = useState<number[]>([])
  const [selectedTags, setSelectedTags] = useState<string[]>([])

  const toggleGroup = (id: number) =>
    setSelectedGroups((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  const toggleTag = (name: string) =>
    setSelectedTags((prev) => (prev.includes(name) ? prev.filter((x) => x !== name) : [...prev, name]))

  // 每次打开重置,避免残留上次的选择
  useEffect(() => {
    if (open) {
      setSelectedGroups([])
      setSelectedTags([])
    }
  }, [open])

  const isGroups = kind === 'groups'
  const dict: any[] = (isGroups ? groups : tagsLib) ?? []
  const chosenCount = isGroups ? selectedGroups.length : selectedTags.length

  const submit = () => {
    // [] 显式发送=清空;未开启的字段不在此弹窗提交
    if (isGroups) onConfirm({ group_ids: selectedGroups })
    else onConfirm({ tags: selectedTags })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t(isGroups ? 'marketplace.batchSetGroupsTitle' : 'marketplace.batchSetTagsTitle')}</DialogTitle>
          <DialogDescription>{t('apiKeys.selected', { count: selectedCount })}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <p className="flex items-start gap-2 rounded-lg bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-300">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            {t(isGroups ? 'marketplace.batchSetGroupsHint' : 'marketplace.batchSetTagsHint')}
          </p>
          <div className="flex flex-wrap gap-2">
            {dict.length === 0 && (
              <p className="text-xs text-muted-foreground">{t(isGroups ? 'marketplace.noGroupsHint' : 'marketplace.noTagsHint')}</p>
            )}
            {isGroups
              ? groups!.map((g) => {
                  const selected = selectedGroups.includes(g.id)
                  return (
                    <button key={g.id} type="button" onClick={() => toggleGroup(g.id)}
                      className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-all ${
                        selected ? 'border-primary bg-primary text-primary-foreground shadow-sm' : 'bg-muted/40 hover:bg-muted'}`}>
                      {selected && <Check className="h-3 w-3 shrink-0" />}
                      {g.name}
                    </button>
                  )
                })
              : tagsLib!.map((tag) => {
                  const selected = selectedTags.includes(tag.name)
                  return (
                    <button key={tag.id} type="button" onClick={() => toggleTag(tag.name)}
                      className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-all ${
                        selected
                          ? tag.color ? 'shadow-sm' : 'border-primary bg-primary text-primary-foreground'
                          : tag.color ? 'hover:opacity-80' : 'bg-muted/40 hover:bg-muted'}`}
                      style={selected && tag.color
                        ? { color: tag.color, backgroundColor: `${tag.color}33`, borderColor: tag.color }
                        : !selected && tag.color
                          ? { color: tag.color, backgroundColor: `${tag.color}1A`, borderColor: `${tag.color}55` }
                          : undefined}>
                      {selected
                        ? <Check className="h-3 w-3 shrink-0" />
                        : tag.color && <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ backgroundColor: tag.color }} />}
                      {tag.name}
                    </button>
                  )
                })}
          </div>
          {chosenCount === 0 && dict.length > 0 && (
            <p className="flex items-start gap-2 rounded-lg bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-300">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              {t(isGroups ? 'marketplace.batchClearGroupsWarning' : 'marketplace.batchClearTagsWarning')}
            </p>
          )}
        </div>
        <DialogFooter className="mt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={pending || selectedCount === 0} onClick={submit}>{t('common.save')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
