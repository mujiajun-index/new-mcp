import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'
import { Server, FolderTree, Cloud, Zap, Plus, ArrowRight } from 'lucide-react'

export function DashboardPage() {
  const { t } = useTranslation()
  const { auth } = useAuthStore()

  const stats = [
    { label: t('dashboard.totalServices'), value: '0', icon: Server, color: 'text-sky-500', bg: 'bg-sky-500/10' },
    { label: t('dashboard.totalGroups'), value: '0', icon: FolderTree, color: 'text-violet-500', bg: 'bg-violet-500/10' },
    { label: t('dashboard.totalConnections'), value: '0', icon: Cloud, color: 'text-emerald-500', bg: 'bg-emerald-500/10' },
    { label: t('dashboard.todayCalls'), value: '0', icon: Zap, color: 'text-amber-500', bg: 'bg-amber-500/10' },
  ]

  const quickActions = [
    { label: t('dashboard.addService'), href: '/services/create', icon: Server },
    { label: t('dashboard.addGroup'), href: '/groups/create', icon: FolderTree },
    { label: t('dashboard.addConnection'), href: '/connections/create', icon: Cloud },
  ]

  return (
    <div className="p-6 lg:p-8 space-y-8 max-w-6xl">
      {/* Welcome */}
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {t('dashboard.title')}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          欢迎回来，{auth.user?.username}
        </p>
      </div>

      {/* Stats grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {stats.map((stat, i) => (
          <div
            key={i}
            className="group relative overflow-hidden rounded-xl border bg-card p-5 transition-all duration-200 hover:shadow-md hover:shadow-black/[0.03]"
          >
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
          <h2 className="mb-4 text-base font-semibold">{t('dashboard.healthStatus')}</h2>
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <Server className="h-10 w-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">{t('common.noData')}</p>
            <p className="text-xs text-muted-foreground/60 mt-1">注册服务后将在此显示健康状态</p>
          </div>
        </div>

        {/* Recent Logs */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="mb-4 text-base font-semibold">{t('dashboard.recentLogs')}</h2>
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <Zap className="h-10 w-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">{t('common.noData')}</p>
            <p className="text-xs text-muted-foreground/60 mt-1">调用 MCP 服务后将在此显示日志</p>
          </div>
        </div>
      </div>
    </div>
  )
}
