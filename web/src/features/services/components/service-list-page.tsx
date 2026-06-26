import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getServices, deleteService, updateService, testService } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import type { TransportType, ServiceListItem } from '@/types'
import {
  Plus, Search, Server, Trash2, Zap, Loader2,
  Wifi, Terminal, Globe, Radio, Plug, MoreHorizontal,
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

function healthLabel(status: string) {
  const { t } = useTranslation()
  if (status === 'healthy') return t('services.healthHealthy')
  if (status === 'unhealthy') return t('services.healthUnhealthy')
  return t('services.healthUnknown')
}

export function ServiceListPage() {
  const { t } = useTranslation()
  const transportLabel = useTransportLabel()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [keyword, setKeyword] = useState('')
  const [transportFilter, setTransportFilter] = useState<string>('')
  const [searchInput, setSearchInput] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['services', keyword, transportFilter],
    queryFn: () => getServices({ keyword: keyword || undefined, transport_type: (transportFilter || undefined) as TransportType | undefined }),
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['services'] })
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

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    setKeyword(searchInput)
  }

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.services')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('services.subtitle')}</p>
        </div>
        <Link to="/services/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            {t('services.registerNew')}
          </Button>
        </Link>
      </div>

      {/* Filters */}
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
        <div className="flex flex-wrap gap-2">
          <Button
            variant={transportFilter === '' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setTransportFilter('')}
          >
            {t('services.filterAll')}
          </Button>
          {['stdio', 'sse', 'streamable-http', 'websocket', 'passive-ws'].map((tp) => (
            <Button
              key={tp}
              variant={transportFilter === tp ? 'default' : 'outline'}
              size="sm"
              onClick={() => setTransportFilter(tp)}
            >
              {transportLabel(tp)}
            </Button>
          ))}
        </div>
      </div>

      {/* List */}
      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
        ) : services.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Server className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('services.noServices')}</p>
            <p className="mt-1 text-xs text-muted-foreground/60">{t('services.noServicesHint')}</p>
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
                      <Link to="/services/$id" params={{ id: String(s.id) }} className="font-medium transition-colors hover:text-primary">
                        {s.display_name || s.name}
                      </Link>
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
                      onClick={() => toggleMutation.mutate({ id: s.id, status: s.status === 1 ? 0 : 1 })}
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
                        </span>
                      ),
                    },
                    { label: t('services.toolsCount'), value: <span className="tabular-nums">{s.tools_count}</span> },
                    {
                      label: t('services.healthHealthy'),
                      value: (
                        <span className="inline-flex items-center gap-1.5">
                          <HealthBadge status={s.health_status} />
                          {healthLabel(s.health_status)}
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
                        <Link to="/services/$id" params={{ id: String(s.id) }} className="font-medium hover:text-primary transition-colors">
                          {s.display_name || s.name}
                        </Link>
                        {s.description && (
                          <p className="mt-0.5 text-xs text-muted-foreground line-clamp-1">{s.description}</p>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className="inline-flex items-center gap-1.5 text-xs">
                          <Icon className="h-3.5 w-3.5" />
                          {transportLabel(s.transport_type)}
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
                          onClick={() => toggleMutation.mutate({ id: s.id, status: s.status === 1 ? 0 : 1 })}
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
    </div>
  )
}