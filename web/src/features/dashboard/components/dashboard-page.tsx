import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Server, FolderTree, Cloud, Zap, Plus, ArrowRight, Activity } from 'lucide-react'
import { getAdminStats } from '@/features/admin/api'
import { getUserLogs } from '@/features/logs/api'
import { getServices } from '@/features/services/api'

export function DashboardPage() {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const isAdmin = auth.user?.role === 'admin'

  const { data: adminStats } = useQuery({
    queryKey: ['admin-stats'],
    queryFn: getAdminStats,
    enabled: isAdmin,
  })

  const { data: servicesData } = useQuery({
    queryKey: ['dashboard-services'],
    queryFn: () => getServices({ page: 1, page_size: 100 }),
  })

  const { data: recentLogs } = useQuery({
    queryKey: ['dashboard-recent-logs'],
    queryFn: () => getUserLogs({ page: 1, page_size: 5 }),
  })

  const stats = adminStats?.data
  const services = servicesData?.data ?? []
  const logs = recentLogs?.data ?? []

  const statCards = [
    { label: t('dashboard.totalServices'), value: stats?.services_count ?? services.length, icon: Server, color: 'text-sky-500', bg: 'bg-sky-500/10' },
    { label: t('dashboard.totalGroups'), value: stats?.groups_count ?? 0, icon: FolderTree, color: 'text-violet-500', bg: 'bg-violet-500/10' },
    { label: t('dashboard.totalConnections'), value: stats?.connections_count ?? 0, icon: Cloud, color: 'text-emerald-500', bg: 'bg-emerald-500/10' },
    { label: t('dashboard.todayCalls'), value: stats?.calls_today ?? 0, icon: Zap, color: 'text-amber-500', bg: 'bg-amber-500/10' },
  ]

  const quickActions = [
    { label: t('dashboard.addService'), href: '/services/create', icon: Server },
    { label: t('dashboard.addGroup'), href: '/groups/create', icon: FolderTree },
    { label: t('dashboard.addConnection'), href: '/connections/create', icon: Cloud },
  ]

  const healthColor = (status: string) => {
    switch (status) {
      case 'healthy': return 'success'
      case 'unhealthy': return 'destructive'
      default: return 'secondary'
    }
  }

  const formatLogTime = (dateStr: string) => {
    if (!dateStr) return ''
    const d = new Date(dateStr)
    return d.toLocaleString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  return (
    <div className="p-6 lg:p-8 space-y-8">
      {/* Welcome */}
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('dashboard.title')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          欢迎回来，{auth.user?.username}
        </p>
      </div>

      {/* Stats grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {statCards.map((stat, i) => (
          <div key={i} className="group relative overflow-hidden rounded-xl border bg-card p-5 transition-all duration-200 hover:shadow-md hover:shadow-black/[0.03]">
            <div className="flex items-start justify-between">
              <div className="space-y-2">
                <p className="text-sm text-muted-foreground">{stat.label}</p>
                <p className="text-3xl font-semibold tracking-tight tabular-nums">{stat.value}</p>
              </div>
              <div className={`flex h-10 w-10 items-center justify-center rounded-xl ${stat.bg}`}>
                <stat.icon className={`h-5 w-5 ${stat.color}`} />
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Quick Actions */}
      <div>
        <h2 className="mb-4 text-base font-semibold">{t('dashboard.quickActions')}</h2>
        <div className="grid gap-3 sm:grid-cols-3">
          {quickActions.map((action, i) => (
            <Link key={i} to={action.href}>
              <div className="group flex items-center gap-4 rounded-xl border bg-card p-4 transition-all duration-200 hover:border-ring/20 hover:shadow-md hover:shadow-black/[0.03]">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-muted">
                  <action.icon className="h-5 w-5 text-muted-foreground group-hover:text-foreground transition-colors" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium">{action.label}</p>
                </div>
                <ArrowRight className="h-4 w-4 text-muted-foreground/50 group-hover:text-foreground group-hover:translate-x-0.5 transition-all" />
              </div>
            </Link>
          ))}
        </div>
      </div>

      {/* Health Status + Recent Logs */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Service Health */}
        <div className="rounded-xl border bg-card p-5">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base font-semibold">{t('dashboard.healthStatus')}</h2>
            <Link to="/services" className="text-xs text-muted-foreground hover:text-foreground transition-colors">
              查看全部 →
            </Link>
          </div>
          {services.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 text-center">
              <Server className="h-8 w-8 text-muted-foreground/30 mb-2" />
              <p className="text-sm text-muted-foreground">{t('common.noData')}</p>
            </div>
          ) : (
            <div className="space-y-2">
              {services.slice(0, 6).map((svc: any) => (
                <Link key={svc.id} to="/services/$id" params={{ id: String(svc.id) }}>
                  <div className="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-muted/50 transition-colors">
                    <div className="flex items-center gap-3 min-w-0">
                      <Server className="h-4 w-4 text-muted-foreground shrink-0" />
                      <span className="text-sm truncate">{svc.display_name || svc.name}</span>
                    </div>
                    <Badge variant={healthColor(svc.health_status) as any} className="text-[10px] shrink-0">
                      {svc.health_status === 'healthy' ? t('dashboard.healthy') :
                       svc.health_status === 'unhealthy' ? t('dashboard.unhealthy') :
                       t('dashboard.offline')}
                    </Badge>
                  </div>
                </Link>
              ))}
            </div>
          )}
        </div>

        {/* Recent Logs */}
        <div className="rounded-xl border bg-card p-5">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-base font-semibold">{t('dashboard.recentLogs')}</h2>
            <Link to="/logs" className="text-xs text-muted-foreground hover:text-foreground transition-colors">
              查看全部 →
            </Link>
          </div>
          {logs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 text-center">
              <Activity className="h-8 w-8 text-muted-foreground/30 mb-2" />
              <p className="text-sm text-muted-foreground">{t('common.noData')}</p>
            </div>
          ) : (
            <div className="space-y-2">
              {logs.map((log: any) => (
                <div key={log.id} className="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-muted/50 transition-colors">
                  <div className="flex items-center gap-3 min-w-0 flex-1">
                    <div className={`h-2 w-2 rounded-full shrink-0 ${log.response_status === 'success' ? 'bg-emerald-500' : 'bg-red-500'}`} />
                    <code className="text-xs font-mono truncate">{log.tool_name}</code>
                  </div>
                  <div className="flex items-center gap-3 shrink-0">
                    <span className="text-xs text-muted-foreground tabular-nums">{log.duration_ms}ms</span>
                    <span className="text-xs text-muted-foreground">{formatLogTime(log.created_at)}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
