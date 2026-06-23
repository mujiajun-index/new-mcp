import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { getUserLogs, getUserLogStats } from '@/features/logs/api'
import { useAuthStore } from '@/stores/auth-store'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tooltip, TooltipContent, TooltipTrigger, TooltipProvider } from '@/components/ui/tooltip'
import { CompactDateTimeRangePicker } from '@/components/ui/date-time-range-picker'
import { Activity, CheckCircle, XCircle, Clock, Zap, Search, RotateCw, ChevronLeft, ChevronRight } from 'lucide-react'
import type { LogFilter } from '@/types'

export function UserLogsPage() {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const isAdmin = auth.user?.role === 'admin'
  const isMobile = useIsMobile()
  const [page, setPage] = useState(1)
  const pageSize = 20
  const [filter, setFilter] = useState<LogFilter>({})
  const [dateRange, setDateRange] = useState<{ start?: Date; end?: Date }>({})

  const apiFilter = useMemo(() => {
    const f: LogFilter = { ...filter }
    if (dateRange.start) f.start_date = dayjs(dateRange.start).format('YYYY-MM-DD HH:mm:ss')
    if (dateRange.end) f.end_date = dayjs(dateRange.end).format('YYYY-MM-DD HH:mm:ss')
    return f
  }, [filter, dateRange])

  const { data: stats, isLoading: statsLoading } = useQuery({
    queryKey: ['user-log-stats', apiFilter],
    queryFn: () => getUserLogStats(apiFilter),
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['user-logs', page, apiFilter],
    queryFn: () => getUserLogs({ ...apiFilter, page, page_size: pageSize }),
  })

  const logs = data?.data ?? []
  const pagination = data?.pagination
  const totalPages = pagination?.total_pages ?? 1

  const updateFilter = (key: keyof LogFilter, value: string) => {
    setFilter(prev => ({ ...prev, [key]: value || undefined }))
    setPage(1)
  }

  const resetFilters = () => {
    setFilter({})
    setDateRange({})
    setPage(1)
  }

  const handleDateRangeChange = (range: { start?: Date; end?: Date }) => {
    setDateRange(range)
    setPage(1)
  }

  const formatTime = (dateStr: string) => {
    if (!dateStr) return '-'
    const d = new Date(dateStr)
    return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(2)}s`
  }

  const statCards = [
    { label: t('logs.totalCalls'), value: stats?.data?.total_calls ?? 0, icon: Activity, color: 'text-sky-500', bg: 'bg-sky-500/10' },
    { label: t('logs.successCalls'), value: stats?.data?.success_calls ?? 0, icon: CheckCircle, color: 'text-emerald-500', bg: 'bg-emerald-500/10' },
    { label: t('logs.failedCalls'), value: stats?.data?.failed_calls ?? 0, icon: XCircle, color: 'text-red-500', bg: 'bg-red-500/10' },
    { label: t('logs.avgDuration'), value: stats?.data?.avg_duration_ms ? formatDuration(stats.data.avg_duration_ms) : '0ms', icon: Clock, color: 'text-violet-500', bg: 'bg-violet-500/10' },
    { label: t('logs.todayCalls'), value: stats?.data?.calls_today ?? 0, icon: Zap, color: 'text-amber-500', bg: 'bg-amber-500/10' },
  ]

  return (
    <TooltipProvider delayDuration={200}>
      <div className="space-y-6 p-4 sm:p-6 lg:p-8">
        {/* Header */}
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('logs.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('logs.subtitle')}</p>
        </div>

        {/* Stats */}
        <div className="grid gap-4 sm:grid-cols-3 lg:grid-cols-5">
          {statCards.map((card, i) => (
            <div key={i} className="rounded-xl border bg-card p-4">
              <div className="flex items-center justify-between">
                <p className="text-xs text-muted-foreground">{card.label}</p>
                <div className={`flex h-8 w-8 items-center justify-center rounded-lg ${card.bg}`}>
                  <card.icon className={`h-4 w-4 ${card.color}`} />
                </div>
              </div>
              <p className="mt-2 text-2xl font-semibold tracking-tight tabular-nums">
                {statsLoading ? '...' : card.value}
              </p>
            </div>
          ))}
        </div>

        {/* Filters */}
        <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card p-4">
          <div className="relative flex-1 min-w-[200px] max-w-xs">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('logs.searchPlaceholder')}
              value={filter.keyword ?? ''}
              onChange={e => updateFilter('keyword', e.target.value)}
              className="pl-9 h-9"
            />
          </div>

          <Select value={filter.status ?? 'all'} onValueChange={v => updateFilter('status', v === 'all' ? '' : v)}>
            <SelectTrigger className="w-[130px] h-9">
              <SelectValue placeholder={t('logs.status')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('logs.allStatus')}</SelectItem>
              <SelectItem value="success">{t('logs.success')}</SelectItem>
              <SelectItem value="error">{t('logs.error')}</SelectItem>
            </SelectContent>
          </Select>

          <CompactDateTimeRangePicker
            start={dateRange.start}
            end={dateRange.end}
            onChange={handleDateRangeChange}
          />

          <Input
            placeholder={t('logs.toolName')}
            value={filter.tool_name ?? ''}
            onChange={e => updateFilter('tool_name', e.target.value)}
            className="w-[150px] h-9"
          />

          <Input
            placeholder={t('logs.groupName')}
            value={filter.group_name ?? ''}
            onChange={e => updateFilter('group_name', e.target.value)}
            className="w-[150px] h-9"
          />

          {isAdmin && (
            <>
              <Input
                placeholder={t('logs.serviceName')}
                value={filter.service_name ?? ''}
                onChange={e => updateFilter('service_name', e.target.value)}
                className="w-[150px] h-9"
              />
              <Input
                placeholder={t('logs.username')}
                value={filter.username ?? ''}
                onChange={e => updateFilter('username', e.target.value)}
                className="w-[130px] h-9"
              />
            </>
          )}

          <Button variant="outline" size="sm" onClick={resetFilters} className="h-9">
            <RotateCw className="mr-1.5 h-3.5 w-3.5" />
            {t('logs.reset')}
          </Button>
        </div>

        {/* Table */}
        <div className="rounded-xl border bg-card">
          {isLoading ? (
            <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
              {t('common.loading')}
            </div>
          ) : logs.length === 0 ? (
            <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
              {t('common.noData')}
            </div>
          ) : isMobile ? (
            <div className="divide-y">
              {logs.map((log: any) => {
                const isSuccess = log.response_status === 'success'
                const errorMsg = log.error_message || ''
                return (
                  <MobileListCard
                    key={log.id}
                    title={
                      <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                        {log.tool_name}
                      </code>
                    }
                    badge={
                      <Badge variant={isSuccess ? 'success' : 'destructive'}>
                        {isSuccess ? t('logs.success') : t('logs.error')}
                      </Badge>
                    }
                    meta={[
                      { label: t('logs.duration'), value: <span className="tabular-nums">{formatDuration(log.duration_ms)}</span> },
                      { label: t('logs.groupName'), value: log.group_name || '-' },
                      { label: 'IP', value: <span className="font-mono">{log.client_ip}</span> },
                      { label: t('logs.time'), value: formatTime(log.created_at) },
                      ...(isAdmin ? [
                        { label: t('logs.username'), value: log.username || '-' },
                        { label: t('logs.apiKeyName'), value: log.api_key_name || '-' },
                        { label: t('logs.serviceName'), value: log.service_name || '-' },
                      ] : []),
                    ]}
                    note={
                      errorMsg ? (
                        <span className="line-clamp-2 text-red-500">{errorMsg}</span>
                      ) : undefined
                    }
                  />
                )
              })}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[50px]">ID</TableHead>
                  {isAdmin && <TableHead>{t('logs.username')}</TableHead>}
                  {isAdmin && <TableHead>{t('logs.apiKeyName')}</TableHead>}
                  <TableHead>{t('logs.toolName')}</TableHead>
                  <TableHead>{t('logs.groupName')}</TableHead>
                  {isAdmin && <TableHead>{t('logs.serviceName')}</TableHead>}
                  <TableHead>{t('logs.status')}</TableHead>
                  <TableHead>{t('logs.duration')}</TableHead>
                  <TableHead>{t('logs.errorMessage')}</TableHead>
                  <TableHead>IP</TableHead>
                  <TableHead>{t('logs.time')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {logs.map((log: any) => (
                  <LogRow key={log.id} log={log} isAdmin={isAdmin} formatTime={formatTime} formatDuration={formatDuration} />
                ))}
              </TableBody>
            </Table>
          )}
        </div>

        {/* Pagination */}
        {pagination && (
          <div className="flex items-center justify-between">
            <p className="text-sm text-muted-foreground">
              {t('logs.total')} {pagination.total} {t('logs.records')}
            </p>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={page <= 1 || isFetching}
                onClick={() => setPage(p => p - 1)}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <span className="text-sm tabular-nums">{page} / {totalPages}</span>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= totalPages || isFetching}
                onClick={() => setPage(p => p + 1)}
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}
      </div>
    </TooltipProvider>
  )
}

function LogRow({ log, isAdmin, formatTime, formatDuration }: {
  log: any
  isAdmin: boolean
  formatTime: (s: string) => string
  formatDuration: (ms: number) => string
}) {
  const { t } = useTranslation()
  const isSuccess = log.response_status === 'success'
  const errorMsg = log.error_message || ''

  return (
    <TableRow>
      <TableCell className="text-xs text-muted-foreground tabular-nums">{log.id}</TableCell>
      {isAdmin && <TableCell className="text-sm">{log.username || '-'}</TableCell>}
      {isAdmin && <TableCell className="text-sm">{log.api_key_name || '-'}</TableCell>}
      <TableCell>
        <code className="text-xs font-mono bg-muted px-1.5 py-0.5 rounded">{log.tool_name}</code>
      </TableCell>
      <TableCell className="text-sm">{log.group_name || '-'}</TableCell>
      {isAdmin && <TableCell className="text-sm">{log.service_name || '-'}</TableCell>}
      <TableCell>
        <Badge variant={isSuccess ? 'success' : 'destructive'}>
          {isSuccess ? t('logs.success') : t('logs.error')}
        </Badge>
      </TableCell>
      <TableCell className="text-sm tabular-nums">{formatDuration(log.duration_ms)}</TableCell>
      <TableCell className="max-w-[200px]">
        {errorMsg ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-xs text-red-500 truncate block cursor-default">
                {errorMsg.length > 30 ? errorMsg.slice(0, 30) + '...' : errorMsg}
              </span>
            </TooltipTrigger>
            <TooltipContent side="top" className="max-w-[400px] whitespace-pre-wrap break-all">
              {errorMsg}
            </TooltipContent>
          </Tooltip>
        ) : (
          <span className="text-xs text-muted-foreground">-</span>
        )}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground font-mono">{log.client_ip}</TableCell>
      <TableCell className="text-xs text-muted-foreground tabular-nums">{formatTime(log.created_at)}</TableCell>
    </TableRow>
  )
}
