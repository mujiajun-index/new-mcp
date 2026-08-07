import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { getWalletOverview, getWalletUsageStats, redeemCode } from '../api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'
import {
  Wallet as WalletIcon, TrendingUp, History, Gift, Activity, Zap, Coins, ArrowRight,
} from 'lucide-react'
import type { WalletOverview, WalletUsageStats } from '@/types'

export function WalletPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { config } = useSystemConfigStore()
  const [redeemInput, setRedeemInput] = useState('')

  const { data: overviewData, isLoading: overviewLoading } = useQuery({
    queryKey: ['wallet-overview'],
    queryFn: getWalletOverview,
  })
  const overview: WalletOverview | undefined = overviewData?.data

  const { data: statsData } = useQuery({
    queryKey: ['wallet-usage-stats'],
    queryFn: getWalletUsageStats,
  })
  const stats: WalletUsageStats | undefined = statsData?.data

  const redeemMutation = useMutation({
    mutationFn: () => redeemCode({ code: redeemInput.trim() }),
    onSuccess: (res) => {
      toast.success(t('wallet.redeemSuccess', { quota: res?.data?.quota ?? 0 }))
      setRedeemInput('')
      queryClient.invalidateQueries({ queryKey: ['wallet-overview'] })
      queryClient.invalidateQueries({ queryKey: ['wallet-usage-stats'] })
      // 充值后刷新统一日志,使新的充值记录可见。
      queryClient.invalidateQueries({ queryKey: ['user-logs'] })
      queryClient.invalidateQueries({ queryKey: ['user-log-stats'] })
    },
  })

  const billingDisabled = !config.billingEnabled

  const overviewCards = [
    { label: t('wallet.balance'), value: overview?.quota ?? 0, icon: WalletIcon, color: 'text-sky-500', bg: 'bg-sky-500/10' },
    { label: t('wallet.used'), value: overview?.used_quota ?? 0, icon: TrendingUp, color: 'text-violet-500', bg: 'bg-violet-500/10' },
    { label: t('wallet.topup'), value: overview?.total_topup ?? 0, icon: Coins, color: 'text-amber-500', bg: 'bg-amber-500/10' },
    { label: t('wallet.requestCount'), value: overview?.request_count ?? 0, icon: Activity, color: 'text-emerald-500', bg: 'bg-emerald-500/10' },
  ]

  const usageCards = [
    { label: t('wallet.consumedToday'), value: stats?.consumed_today ?? 0, icon: Zap, color: 'text-amber-500', bg: 'bg-amber-500/10' },
    { label: t('wallet.consumedWeek'), value: stats?.consumed_week ?? 0, icon: History, color: 'text-sky-500', bg: 'bg-sky-500/10' },
    { label: t('wallet.consumedTotal'), value: stats?.consumed_total ?? 0, icon: TrendingUp, color: 'text-violet-500', bg: 'bg-violet-500/10' },
  ]

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('wallet.title')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('wallet.subtitle')}</p>
      </div>

      {billingDisabled && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-4 text-sm text-amber-700 dark:text-amber-300">
          {t('billing.disabled')} — {t('wallet.billingDesc')}
        </div>
      )}

      {/* Overview + redemption */}
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="grid gap-4 sm:grid-cols-2 lg:col-span-2">
          {overviewCards.map((card, i) => (
            <div key={i} className="rounded-xl border bg-card p-4">
              <div className="flex items-center justify-between">
                <p className="text-xs text-muted-foreground">{card.label}</p>
                <div className={`flex h-8 w-8 items-center justify-center rounded-lg ${card.bg}`}>
                  <card.icon className={`h-4 w-4 ${card.color}`} />
                </div>
              </div>
              <p className="mt-2 text-2xl font-semibold tabular-nums">
                {overviewLoading ? '...' : card.value}
                <span className="ml-1 text-xs font-normal text-muted-foreground">{t('wallet.quotaUnit')}</span>
              </p>
            </div>
          ))}
        </div>

        {/* Group + redemption card */}
        <div className="space-y-4">
          <div className="rounded-xl border bg-card p-4">
            <p className="text-xs text-muted-foreground">{t('wallet.group')}</p>
            <p className="mt-1 text-lg font-semibold">{overview?.group || '-'}</p>
          </div>
          <div className="rounded-xl border bg-card p-4 space-y-3">
            <div className="flex items-center gap-2">
              <Gift className="h-4 w-4 text-primary" />
              <p className="text-sm font-medium">{t('wallet.redeemTitle')}</p>
            </div>
            <p className="text-xs text-muted-foreground">{t('wallet.redeemDesc')}</p>
            {config.redemptionEnabled ? (
              <div className="flex gap-2">
                <Input
                  placeholder={t('wallet.redeemPlaceholder')}
                  value={redeemInput}
                  onChange={(e) => setRedeemInput(e.target.value)}
                  className="font-mono text-sm"
                />
                <Button
                  size="sm"
                  disabled={!redeemInput.trim() || redeemMutation.isPending}
                  onClick={() => redeemMutation.mutate()}
                >
                  {redeemMutation.isPending ? t('wallet.redeeming') : t('wallet.redeem')}
                </Button>
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">{t('wallet.redeemClosed')}</p>
            )}
          </div>
        </div>
      </div>

      {/* Usage stats */}
      <div className="grid gap-4 sm:grid-cols-3">
        {usageCards.map((card, i) => (
          <div key={i} className="rounded-xl border bg-card p-4">
            <div className="flex items-center justify-between">
              <p className="text-xs text-muted-foreground">{card.label}</p>
              <div className={`flex h-7 w-7 items-center justify-center rounded-lg ${card.bg}`}>
                <card.icon className={`h-3.5 w-3.5 ${card.color}`} />
              </div>
            </div>
            <p className="mt-2 text-xl font-semibold tabular-nums">
              {stats ? card.value : '...'}
              <span className="ml-1 text-xs font-normal text-muted-foreground">{t('wallet.quotaUnit')}</span>
            </p>
          </div>
        ))}
      </div>

      {/* 消费明细已合并到「调用日志」页统一展示 */}
      <div className="flex justify-end">
        <Button variant="outline" size="sm" onClick={() => navigate({ to: '/logs', search: { type: 2 } })}>
          {t('wallet.viewDetails')}
          <ArrowRight className="ml-1.5 h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  )
}
