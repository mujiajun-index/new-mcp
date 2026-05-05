import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getUserLogs, getUserLogStats } from '@/features/logs/api'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Activity, CheckCircle, XCircle, Clock, Zap, Search, RotateCw, ChevronLeft, ChevronRight } from 'lucide-react'
import type { LogFilter } from '@/types'

export function UserLogsPage() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pageSize = 20
  const [filter, setFilter] = useState<LogFilter>({})

  const { data: stats, isLoading: statsLoading } = useQuery({
    queryKey: ['user-log-stats', filter],
    queryFn: () => getUserLogStats(filter),
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['user-logs', page, filter],
    queryFn: () => getUserLogs({ ...filter, page, page_size: pageSize }),
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
    setPage(1)
  }

  const formatTime = (dateStr: string) => {
    if (!dateStr) return '-'
    const d = new Date(dateStr)
    return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(1)}s`
  }

  const statCards = [
    { label: t('logs.totalCalls'), value: stats?.data?.total_calls ?? 0, icon: Activity, color: 'text-sky-500', bg: 'bg-sky-500/10' },
    { label: t('logs.successCalls'), value: stats?.data?.success_calls ?? 0, icon: CheckCircle, color: 'text-emerald-500', bg: 'bg-emerald-500/10' },
    { label: t('logs.failedCalls'), value: stats?.data?.failed_calls ?? 0, icon: XCircle, color: 'text-red-500', bg: 'bg-red-500/10' },
    { label: t('logs.avgDuration'), value: stats?.data?.avg_duration_ms ? formatDuration(stats.data.avg_duration_ms) : '0ms', icon: Clock, color: 'text-violet-500', bg: 'bg-violet-500/10' },
    { label: t('logs.todayCalls'), value: stats?.data?.calls_today ?? 0, icon: Zap, color: 'text-amber-500', bg: 'bg-amber-500/10' },
  ]

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-[1400px]">
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

        <Button variant="outline" size="sm" onClick={resetFilters} className="h-9">
          <RotateCw className="mr-1.5 h-3.5 w-3.5" />
          {t('logs.reset')}
        </Button>
      </div>

      {/* Table */}
      <div className="rounded-xl border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[60px]">ID</TableHead>
              <TableHead>{t('logs.toolName')}</TableHead>
              <TableHead>{t('logs.groupName')}</TableHead>
              <TableHead>{t('logs.serviceName')}</TableHead>
              <TableHead>{t('logs.status')}</TableHead>
              <TableHead>{t('logs.duration')}</TableHead>
              <TableHead>IP</TableHead>
              <TableHead>{t('logs.time')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={8} className="h-32 text-center text-muted-foreground">
                  {t('common.loading')}
                </TableCell>
              </TableRow>
            ) : logs.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className="h-32 text-center text-muted-foreground">
                  {t('common.noData')}
                </TableCell>
              </TableRow>
            ) : (
              logs.map((log: any) => (
                <UserLogRow key={log.id} log={log} formatTime={formatTime} formatDuration={formatDuration} />
              ))
            )}
          </TableBody>
        </Table>
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
  )
}

function UserLogRow({ log, formatTime, formatDuration }: { log: any; formatTime: (s: string) => string; formatDuration: (ms: number) => string }) {
  const [expanded, setExpanded] = useState(false)
  const isSuccess = log.response_status === 'success'

  return (
    <>
      <TableRow className="cursor-pointer" onClick={() => setExpanded(!expanded)}>
        <TableCell className="text-xs text-muted-foreground tabular-nums">{log.id}</TableCell>
        <TableCell>
          <code className="text-xs font-mono bg-muted px-1.5 py-0.5 rounded">{log.tool_name}</code>
        </TableCell>
        <TableCell className="text-sm">{log.group_name || '-'}</TableCell>
        <TableCell className="text-sm">{log.service_name || '-'}</TableCell>
        <TableCell>
          <Badge variant={isSuccess ? 'success' : 'destructive'}>
            {isSuccess ? '成功' : '失败'}
          </Badge>
        </TableCell>
        <TableCell className="text-sm tabular-nums">{formatDuration(log.duration_ms)}</TableCell>
        <TableCell className="text-xs text-muted-foreground font-mono">{log.client_ip}</TableCell>
        <TableCell className="text-xs text-muted-foreground tabular-nums">{formatTime(log.created_at)}</TableCell>
      </TableRow>
      {expanded && (
        <TableRow>
          <TableCell colSpan={8} className="bg-muted/30 px-6 py-3">
            <div className="space-y-2 text-sm">
              {log.error_message && (
                <div>
                  <span className="text-muted-foreground">{t('logs.errorMessage')}：</span>
                  <span className="text-red-500">{log.error_message}</span>
                </div>
              )}
              {log.api_key_name && (
                <div>
                  <span className="text-muted-foreground">API Key：</span>
                  <span className="text-xs">{log.api_key_name}</span>
                </div>
              )}
            </div>
          </TableCell>
        </TableRow>
      )}
    </>
  )
}
