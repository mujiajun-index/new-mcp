import { useMemo, useState } from 'react'
import { Link, Navigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getServiceOverview } from '../api'
import { useAuthStore } from '@/stores/auth-store'
import { isAdminRole } from '@/lib/roles'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { ServicesOverviewData, ServicesOverviewItem, TransportType } from '@/types'
import {
  Activity, ArrowLeft, Clock, HeartPulse, Layers, MemoryStick,
  RefreshCw, Search, Server, Wrench,
} from 'lucide-react'

// formatBytes: 进程资源指标的展示格式化,与详情页口径一致(B/KB/MB/GB)
function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}

type StatusFilter = 'all' | 'running' | 'stopped'
type TypeFilter = 'all' | TransportType
// 类型筛选可选项(顺序即下拉展示顺序)
const TYPE_FILTERS: TransportType[] = ['stdio', 'sse', 'streamable-http', 'websocket', 'passive-ws', 'virtual']

// 传输类型显示名(类型筛选下拉用,与卡片徽章同口径)
function transportTypeLabel(t: ReturnType<typeof useTranslation>['t'], type: TransportType): string {
  if (type === 'virtual') return t('services.transport_virtual')
  return t(`services.transports.${type}`, { defaultValue: type })
}

function ServiceCard({ s, hostTotal }: { s: ServicesOverviewItem; hostTotal: number }) {
  const { t } = useTranslation()
  const isStdio = s.transport_type === 'stdio'
  const showProcess = s.running && isStdio
  const cpu = showProcess && s.cpu_percent != null ? s.cpu_percent : 0
  const memPct = showProcess && s.memory_rss_bytes && hostTotal > 0
    ? (s.memory_rss_bytes / hostTotal) * 100
    : 0
  const transportLabel = transportTypeLabel(t, s.transport_type)

  // 状态圆点:禁用恒灰;stdio 看进程实测;远程/虚拟看最近健康检测结果(健康/异常/未知)
  let dotCls = 'bg-zinc-300 dark:bg-zinc-600'
  let dotTitle: string
  if (s.status !== 1) {
    dotTitle = t('services.statusBadgeDisabled')
  } else if (isStdio) {
    if (s.running) {
      dotCls = 'bg-emerald-500'
      dotTitle = t('services.overview.filterRunning')
    } else {
      dotTitle = t('services.overview.filterStopped')
    }
  } else if (s.running) {
    dotCls = 'bg-emerald-500'
    dotTitle = t('services.healthHealthy')
  } else if (s.health_status === 'unhealthy') {
    dotCls = 'bg-red-500'
    dotTitle = t('services.healthUnhealthy')
  } else {
    dotTitle = t('services.healthUnknown')
  }

  return (
    <Link to="/services/$id" params={{ id: String(s.id) }}>
      <div className="space-y-4 rounded-xl border bg-card p-5 transition-all duration-200 hover:border-ring/20 hover:shadow-md hover:shadow-black/[0.03]">
        <div className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <span
              className={`h-2 w-2 shrink-0 rounded-full ${dotCls}`}
              title={dotTitle}
            />
            <span className="truncate text-sm font-medium">{s.display_name || s.name}</span>
          </div>
          <Badge variant="secondary" className="shrink-0 text-[10px]">{transportLabel}</Badge>
        </div>

        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <Wrench className="h-3.5 w-3.5" />
            {t('services.toolsCount')} {s.tools_count}
          </span>
          <span className="inline-flex items-center gap-1">
            <Clock className="h-3.5 w-3.5" />
            {s.created_at.slice(0, 10)}
          </span>
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">{t('services.overview.cpuRow')}</span>
            <span className="tabular-nums">
              {showProcess && s.cpu_percent != null ? `${s.cpu_percent.toFixed(1)}%` : '—'}
            </span>
          </div>
          <Progress value={cpu} className="h-1 bg-sky-500/15 [&_[data-slot=progress-indicator]]:bg-sky-500" />
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">{t('services.overview.memRow')}</span>
            <span className="tabular-nums">
              {showProcess && s.memory_rss_bytes != null
                ? hostTotal > 0
                  ? `${formatBytes(s.memory_rss_bytes)} / ${formatBytes(hostTotal)}`
                  : formatBytes(s.memory_rss_bytes)
                : '—'}
            </span>
          </div>
          <Progress value={memPct} className="h-1 bg-emerald-500/15 [&_[data-slot=progress-indicator]]:bg-emerald-500" />
        </div>
      </div>
    </Link>
  )
}

export function ServiceOverviewPage() {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const isAdmin = isAdminRole(auth.user?.role)
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [typeFilter, setTypeFilter] = useState<TypeFilter>('stdio')
  const [search, setSearch] = useState('')

  const { data, isFetching, refetch } = useQuery({
    queryKey: ['services-overview'],
    queryFn: getServiceOverview,
    refetchInterval: 5000,
    enabled: isAdmin,
  })

  const ov: ServicesOverviewData | undefined = data?.data
  const summary = ov?.summary
  const services = useMemo(() => ov?.services ?? [], [ov])

  // 筛选与搜索均为前端本地操作(接口返回全量)
  const filtered = useMemo(() => {
    const kw = search.trim().toLowerCase()
    return services.filter((s) => {
      if (typeFilter !== 'all' && s.transport_type !== typeFilter) return false
      if (statusFilter === 'running' && !s.running) return false
      if (statusFilter === 'stopped' && s.running) return false
      if (!kw) return true
      return s.name.toLowerCase().includes(kw) || s.display_name.toLowerCase().includes(kw)
    })
  }, [services, statusFilter, typeFilter, search])

  const hostTotal = summary?.host_memory_total_bytes ?? 0
  const healthRate = summary && summary.total_services > 0
    ? Math.round((summary.healthy_count / summary.total_services) * 100)
    : 0

  // 管理员专属页面:非管理员直接输入 URL 访问时退回服务列表
  if (!isAdmin) {
    return <Navigate to="/services" replace />
  }

  const statCards = [
    { label: t('services.overview.servicesCard'), value: String(summary?.total_services ?? 0), sub: t('services.overview.runningSub', { count: summary?.running_services ?? 0 }), icon: Server, color: 'text-sky-500', bg: 'bg-sky-500/10' },
    { label: t('services.overview.toolsTotal'), value: String(summary?.tools_total ?? 0), icon: Wrench, color: 'text-violet-500', bg: 'bg-violet-500/10' },
    { label: t('services.overview.processes'), value: String(summary?.process_total ?? 0), icon: Layers, color: 'text-emerald-500', bg: 'bg-emerald-500/10' },
    { label: t('services.overview.memoryUsage'), value: formatBytes(summary?.memory_rss_bytes_total ?? 0), sub: `${t('services.overview.hostTotal')}: ${formatBytes(hostTotal)}`, icon: MemoryStick, color: 'text-amber-500', bg: 'bg-amber-500/10' },
    { label: t('services.overview.cpuUsage'), value: `${(summary?.cpu_percent_total ?? 0).toFixed(1)}%`, hint: t('services.overview.cpuHint'), icon: Activity, color: 'text-rose-500', bg: 'bg-rose-500/10' },
    { label: t('services.overview.healthRate'), value: `${healthRate}%`, sub: `${summary?.healthy_count ?? 0} / ${summary?.total_services ?? 0}`, icon: HeartPulse, color: 'text-teal-500', bg: 'bg-teal-500/10' },
  ]

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Link to="/services">
            <Button variant="ghost" size="icon" title={t('services.subtitle')}>
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">{t('services.overview.title')}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{t('services.overview.subtitle')}</p>
          </div>
        </div>
        <Button variant="outline" className="gap-2" onClick={() => refetch()}>
          <RefreshCw className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} />
          {t('services.overview.refresh')}
        </Button>
      </div>

      {/* 资源概述 */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        {statCards.map((stat, i) => (
          <div
            key={i}
            className="rounded-xl border bg-card p-5 transition-all duration-200 hover:shadow-md hover:shadow-black/[0.03]"
            title={stat.hint}
          >
            <div className="flex items-start justify-between">
              <div className="min-w-0 space-y-2">
                <p className="text-sm text-muted-foreground">{stat.label}</p>
                <p className="text-3xl font-semibold tracking-tight">{stat.value}</p>
                {stat.sub && <p className="text-xs text-muted-foreground">{stat.sub}</p>}
              </div>
              <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${stat.bg}`}>
                <stat.icon className={`h-5 w-5 ${stat.color}`} />
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* 服务列表 */}
      <div>
        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h2 className="text-base font-semibold">{t('services.overview.servicesTitle')}</h2>
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative w-full sm:w-56">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={t('services.overview.searchPlaceholder')}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9"
              />
            </div>
            <Select value={typeFilter} onValueChange={(v) => setTypeFilter(v as TypeFilter)}>
              <SelectTrigger className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('services.overview.filterTypeAll')}</SelectItem>
                {TYPE_FILTERS.map((type) => (
                  <SelectItem key={type} value={type}>{transportTypeLabel(t, type)}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={statusFilter} onValueChange={(v) => setStatusFilter(v as StatusFilter)}>
              <SelectTrigger className="w-28">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('services.overview.filterAll')}</SelectItem>
                <SelectItem value="running">{t('services.overview.filterRunning')}</SelectItem>
                <SelectItem value="stopped">{t('services.overview.filterStopped')}</SelectItem>
              </SelectContent>
            </Select>
            {services.length > 0 && (
              <span className="text-xs text-muted-foreground">
                {t('services.overview.count', { shown: filtered.length, total: services.length })}
              </span>
            )}
          </div>
        </div>

        {services.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-xl border bg-card py-12 text-center">
            <Server className="mb-2 h-8 w-8 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('services.overview.emptyHint')}</p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-xl border bg-card py-12 text-center">
            <Server className="mb-2 h-8 w-8 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('services.overview.noMatch')}</p>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            {filtered.map((s) => (
              <ServiceCard key={s.id} s={s} hostTotal={hostTotal} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
