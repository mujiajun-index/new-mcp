import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getServices, deleteService, updateService, testService } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { LocalPager } from '@/components/local-pager'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { useAuthStore } from '@/stores/auth-store'
import { isAdminRole } from '@/lib/roles'
import type { TransportType, ServiceListItem } from '@/types'
import {
  Plus, Search, Server, Trash2, Zap, Loader2,
  Wifi, Terminal, Globe, Radio, Plug, MoreHorizontal, LayoutGrid,
} from 'lucide-react'
import { toast } from 'sonner'

const transportIcons: Record<string, React.ComponentType<{ className?: string }>> = {
  'stdio': Terminal,
  'sse': Globe,
  'streamable-http': Wifi,
  'websocket': Radio,
  'passive-ws': Plug,
  'virtual': Zap,
}

// 类型筛选维度对齐总览视图:marketplace 非传输类型而是 source 维度(市场引用行);
// stdio 选项与总览同口径,仅管理员可见(普通用户的 stdio 服务归平台进程托管,
// 总览接口对其排除 stdio),普通用户含市场安装的 stdio 引用行也不提供该筛选项。
type TypeFilter = 'all' | TransportType | 'marketplace'
// 启用状态筛选(对应后端 status 位),与总览一致的三态下拉;管理页默认「全部」
// (总览默认已启用是运维视角,管理页常要找禁用服务重新启用)
type EnabledFilter = 'all' | 'enabled' | 'disabled'
// 类型筛选可选项(顺序即下拉展示顺序,含虚拟服务)
const TYPE_FILTERS: TransportType[] = ['stdio', 'sse', 'streamable-http', 'websocket', 'passive-ws', 'virtual']

function useTransportLabel() {
  const { t } = useTranslation()
  return (type: string) => {
    if (type === 'virtual') return t('services.transport_virtual')
    return t(`services.transports.${type}`, { defaultValue: type })
  }
}

function StatusBadge({ status }: { status: number }) {
  const { t } = useTranslation()
  if (status === 1) return <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">{t('services.statusBadgeEnabled')}</span>
  return <span className="inline-flex items-center gap-1 rounded-full bg-zinc-500/10 px-2 py-0.5 text-xs font-medium text-zinc-500">{t('services.statusBadgeDisabled')}</span>
}

function HealthBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  if (status === 'healthy') return <span className="inline-flex h-2 w-2 rounded-full bg-emerald-500" title={t('services.healthHealthy')} />
  if (status === 'unhealthy') return <span className="inline-flex h-2 w-2 rounded-full bg-red-500" title={t('services.healthUnhealthy')} />
  return <span className="inline-flex h-2 w-2 rounded-full bg-zinc-300 dark:bg-zinc-600" title={t('services.healthUnknown')} />
}

function HealthLabel({ status }: { status: string }) {
  const { t } = useTranslation()
  if (status === 'healthy') return <>{t('services.healthHealthy')}</>
  if (status === 'unhealthy') return <>{t('services.healthUnhealthy')}</>
  return <>{t('services.healthUnknown')}</>
}

export function ServiceListPage() {
  const { t } = useTranslation()
  const transportLabel = useTransportLabel()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const { auth } = useAuthStore()
  const isAdmin = isAdminRole(auth.user?.role)
  const [keyword, setKeyword] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [typeFilter, setTypeFilter] = useState<TypeFilter>('all')
  const [enabledFilter, setEnabledFilter] = useState<EnabledFilter>('all')
  // 服务端分页:此前未传分页参数,后端默认只回前 20 条且无翻页控件,超出的服务不可见
  const [page, setPage] = useState(1)
  const PAGE_SIZE = 20

  const { data, isLoading } = useQuery({
    queryKey: ['services', keyword, typeFilter, enabledFilter, page],
    queryFn: () => getServices({
      keyword: keyword || undefined,
      transport_type: typeFilter !== 'all' && typeFilter !== 'marketplace' ? typeFilter : undefined,
      source: typeFilter === 'marketplace' ? 'marketplace' : undefined,
      status: enabledFilter === 'all' ? undefined : enabledFilter === 'enabled' ? 1 : 0,
      page,
      page_size: PAGE_SIZE,
    }),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteService,
    onSuccess: () => {
      toast.success(t('services.deleteSuccess'))
      queryClient.invalidateQueries({ queryKey: ['services'] })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) => updateService(id, { status }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['services'] })
      // 禁用会从全部分组移除该服务，分组列表的工具数/成员会变化，需一并刷新
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      if (variables.status === 0) {
        toast.success(t('services.disabledToast'))
      }
    },
  })

  const testMutation = useMutation({
    mutationFn: testService,
    onSuccess: (res) => {
      const result = res.data
      if (result?.connected) {
        toast.success(t('services.connectSuccess', { count: result.tools_count ?? 0, ms: result.latency_ms ?? 0 }))
      } else {
        toast.error(t('services.connectFailed', { error: result?.error || t('common.unknownError') }))
      }
      queryClient.invalidateQueries({ queryKey: ['services'] })
    },
    onError: () => {
      toast.error(t('services.testRequestFailed'))
    },
  })

  const services: ServiceListItem[] = data?.data || []
  const pagination = data?.pagination
  const total = pagination?.total ?? services.length
  const totalPages = Math.max(1, pagination?.total_pages ?? 1)
  // 删除服务使总页数收缩时夹紧当前页,避免停留在空页
  const safePage = Math.min(page, totalPages)
  const hasFilter = Boolean(keyword || typeFilter !== 'all' || enabledFilter !== 'all')

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    setKeyword(searchInput)
    setPage(1)
  }

  // 切换启用/禁用：禁用会从全部分组移除该服务及其工具，需二次确认；启用直接生效。
  // 平台已下架的市场引用行不可启用（后端同样拦截，前端先拦避免无效请求）。
  const handleToggleStatus = (s: ServiceListItem) => {
    if (s.status === 1) {
      if (!confirm(t('services.disableConfirm', { name: s.display_name || s.name }))) return
      toggleMutation.mutate({ id: s.id, status: 0 })
    } else {
      if (s.marketplace_offline) {
        toast.error(t('services.marketplaceOfflineEnableBlocked'))
        return
      }
      toggleMutation.mutate({ id: s.id, status: 1 })
    }
  }

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.services')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('services.subtitle')}</p>
        </div>
        <div className="flex gap-2">
          <Link to="/services/overview">
            <Button variant="outline" className="gap-2">
              <LayoutGrid className="h-4 w-4" />
              {t('services.overview.title')}
            </Button>
          </Link>
          <Link to="/services/create">
            <Button className="gap-2">
              <Plus className="h-4 w-4" />
              {t('services.registerNew')}
            </Button>
          </Link>
        </div>
      </div>

      {/* Filters:筛选维度与交互对齐总览视图(搜索 + 类型下拉 + 启用状态下拉 + 计数) */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <form onSubmit={handleSearch} className="relative max-w-sm flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('services.searchPlaceholder')}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </form>
        <div className="flex flex-wrap items-center gap-2 sm:ml-auto">
          <Select value={typeFilter} onValueChange={(v) => { setTypeFilter(v as TypeFilter); setPage(1) }}>
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('services.filterTypeAll')}</SelectItem>
              {/* 末尾「市场」按 source=marketplace 过滤(非传输类型),同总览;
                  stdio 仅管理员可见,同总览口径 */}
              {[
                ...(isAdmin ? TYPE_FILTERS : TYPE_FILTERS.filter((type) => type !== 'stdio')),
                ...(['marketplace'] as const),
              ].map((type) => (
                <SelectItem key={type} value={type}>
                  {type === 'marketplace' ? t('marketplace.platformHosted') : transportLabel(type)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={enabledFilter} onValueChange={(v) => { setEnabledFilter(v as EnabledFilter); setPage(1) }}>
            <SelectTrigger className="w-28">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('services.filterAll')}</SelectItem>
              <SelectItem value="enabled">{t('services.filterEnabled')}</SelectItem>
              <SelectItem value="disabled">{t('services.filterDisabled')}</SelectItem>
            </SelectContent>
          </Select>
          {total > 0 && (
            <span className="text-xs text-muted-foreground">
              {t('common.total')} {total} {t('common.items')}
            </span>
          )}
        </div>
      </div>

      {/* List */}
      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
        ) : services.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Server className="mb-3 h-10 w-10 text-muted-foreground/30" />
            {/* 区分「还没有服务」与「筛选无结果」:后者提示调整筛选条件 */}
            <p className="text-sm text-muted-foreground">{hasFilter ? t('services.noMatch') : t('services.noServices')}</p>
            {!hasFilter && <p className="mt-1 text-xs text-muted-foreground/60">{t('services.noServicesHint')}</p>}
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {services.map((s) => {
              const Icon = transportIcons[s.transport_type] || Globe
              const isVirtual = s.transport_type === 'virtual'
              return (
                <MobileListCard
                  key={s.id}
                  title={
                    <div className="flex flex-col">
                      <div className="flex items-center gap-2">
                        <Link to="/services/$id" params={{ id: String(s.id) }} className="font-medium transition-colors hover:text-primary">
                          {s.display_name || s.name}
                        </Link>
                        {s.source === 'marketplace' && (
                          <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                            {t('marketplace.platformHosted')}
                          </span>
                        )}
                        {s.marketplace_offline && (
                          <span className="rounded bg-destructive/10 px-1.5 py-0.5 text-[10px] font-medium text-destructive" title={t('services.marketplaceOfflineDesc')}>
                            {t('services.marketplaceOffline')}
                          </span>
                        )}
                      </div>
                      {s.description && (
                        <p className="line-clamp-1 text-xs text-muted-foreground">{s.description}</p>
                      )}
                    </div>
                  }
                  badge={
                    <button
                      type="button"
                      className="cursor-pointer"
                      title={s.status === 1 ? t('services.clickDisable') : t('services.clickEnable')}
                      aria-label={s.status === 1 ? t('services.clickDisable') : t('services.clickEnable')}
                      onClick={() => handleToggleStatus(s)}
                    >
                      <StatusBadge status={s.status} />
                    </button>
                  }
                  meta={[
                    {
                      label: t('services.transport'),
                      value: (
                        <span className="inline-flex items-center gap-1.5">
                          <Icon className="h-3.5 w-3.5" />
                          {transportLabel(s.transport_type)}
                          {s.key_mode && (
                            <span className="rounded bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-400">
                              {t('services.keys.multiKeyShort', { mode: s.key_mode === 'random' ? t('services.keys.modeRandom') : t('services.keys.modePolling') })}
                            </span>
                          )}
                        </span>
                      ),
                    },
                    { label: t('services.toolsCount'), value: <span className="tabular-nums">{s.tools_count}</span> },
                    {
                      label: t('services.healthHealthy'),
                      value: (
                        <span className="inline-flex items-center gap-1.5">
                          <HealthBadge status={s.health_status} />
                          <HealthLabel status={s.health_status} />
                        </span>
                      ),
                    },
                  ]}
                  actions={
                    <>
                      {!isVirtual && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="gap-1"
                          disabled={testMutation.isPending}
                          onClick={() => testMutation.mutate(s.id)}
                        >
                          {testMutation.isPending && testMutation.variables === s.id
                            ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                            : <Zap className="h-3.5 w-3.5" />}
                          {t('services.test')}
                        </Button>
                      )}
                      <Link to="/services/$id" params={{ id: String(s.id) }}>
                        <Button variant="ghost" size="sm">{t('services.detail')}</Button>
                      </Link>
                      {!isVirtual && (
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" className="h-8 w-8">
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem
                              className="text-destructive focus:text-destructive"
                              onClick={() => {
                                if (confirm(t('services.deleteConfirm', { name: s.display_name || s.name }))) {
                                  deleteMutation.mutate(s.id)
                                }
                              }}
                            >
                              <Trash2 className="mr-2 h-4 w-4" />
                              {t('common.delete')}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      )}
                    </>
                  }
                />
              )
            })}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('services.serviceName')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('services.transportType')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('services.healthHealthy')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('services.toolsCount')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('common.status')}</th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {services.map((s) => {
                  const Icon = transportIcons[s.transport_type] || Globe
                  const isVirtual = s.transport_type === 'virtual'
                  return (
                    <tr key={s.id} className="border-b last:border-0 hover:bg-muted/30 transition-colors">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <Link to="/services/$id" params={{ id: String(s.id) }} className="font-medium hover:text-primary transition-colors">
                            {s.display_name || s.name}
                          </Link>
                          {s.source === 'marketplace' && (
                            <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary" title={t('marketplace.platformHostedDesc')}>
                              {t('marketplace.platformHosted')}
                            </span>
                          )}
                          {s.marketplace_offline && (
                            <span className="rounded bg-destructive/10 px-1.5 py-0.5 text-[10px] font-medium text-destructive" title={t('services.marketplaceOfflineDesc')}>
                              {t('services.marketplaceOffline')}
                            </span>
                          )}
                        </div>
                        {s.description && (
                          <p className="mt-0.5 text-xs text-muted-foreground line-clamp-1">{s.description}</p>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className="inline-flex items-center gap-1.5 text-xs">
                          <Icon className="h-3.5 w-3.5" />
                          {transportLabel(s.transport_type)}
                          {s.key_mode && (
                            <span className="rounded bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-400">
                              {t('services.keys.multiKeyShort', { mode: s.key_mode === 'random' ? t('services.keys.modeRandom') : t('services.keys.modePolling') })}
                            </span>
                          )}
                        </span>
                      </td>
                      <td className="px-4 py-3"><HealthBadge status={s.health_status} /></td>
                      <td className="px-4 py-3 tabular-nums">{s.tools_count}</td>
                      <td className="px-4 py-3">
                        <button
                          type="button"
                          className="cursor-pointer"
                          title={s.status === 1 ? t('services.clickDisable') : t('services.clickEnable')}
                          aria-label={s.status === 1 ? t('services.clickDisable') : t('services.clickEnable')}
                          onClick={() => handleToggleStatus(s)}
                        >
                          <StatusBadge status={s.status} />
                        </button>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          {!isVirtual && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="gap-1"
                            disabled={testMutation.isPending}
                            onClick={() => testMutation.mutate(s.id)}
                          >
                            {testMutation.isPending && testMutation.variables === s.id
                              ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                              : <Zap className="h-3.5 w-3.5" />}
                            {t('services.test')}
                          </Button>
                          )}
                          <Link to="/services/$id" params={{ id: String(s.id) }}>
                            <Button variant="ghost" size="sm">{t('services.detail')}</Button>
                          </Link>
                          {!isVirtual && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => {
                              if (confirm(t('services.deleteConfirm', { name: s.display_name || s.name }))) {
                                deleteMutation.mutate(s.id)
                              }
                            }}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 服务端分页:仅一页时不显示 */}
      {total > PAGE_SIZE && (
        <LocalPager total={total} page={safePage} totalPages={totalPages} onPage={setPage} />
      )}
    </div>
  )
}