import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getWalletOverview, getWalletBilling, getWalletUsageStats, redeemCode } from '../api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { useIsMobile } from '@/hooks/use-mobile'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { billingStatusKey, billingStatusClass, priceScopeKey, priceLabel } from '@/lib/billing'
import { toast } from 'sonner'
import {
  Wallet as WalletIcon, TrendingUp, History, Gift, Activity,
  ChevronLeft, ChevronRight, Zap, Coins,
} from 'lucide-react'
import type { WalletOverview, WalletUsageStats } from '@/types'

const formatTime = (s: string) => {
  if (!s) return '-'
  return new Date(s).toLocaleString('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  })
}

export function WalletPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const { config } = useSystemConfigStore()
  const [page, setPage] = useState(1)
  const pageSize = 15
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

  const { data: billingData, isLoading: billingLoading, isFetching } = useQuery({
    queryKey: ['wallet-billing', page],
    queryFn: () => getWalletBilling({ page, page_size: pageSize }),
  })
  const billing = billingData?.data ?? []
  const pagination = billingData?.pagination
  const totalPages = pagination?.total_pages ?? 1

  const redeemMutation = useMutation({
    mutationFn: () => redeemCode({ code: redeemInput.trim() }),
    onSuccess: (res) => {
      toast.success(t('wallet.redeemSuccess', { quota: res?.data?.quota ?? 0 }))
      setRedeemInput('')
      queryClient.invalidateQueries({ queryKey: ['wallet-overview'] })
      queryClient.invalidateQueries({ queryKey: ['wallet-usage-stats'] })
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

      {/* Billing details */}
      <div>
        <h2 className="mb-1 text-lg font-semibold">{t('wallet.billingTitle')}</h2>
        <p className="mb-3 text-xs text-muted-foreground">{t('wallet.billingDesc')}</p>
        <div className="rounded-xl border bg-card">
          {billingLoading ? (
            <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
          ) : billing.length === 0 ? (
            <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('wallet.noBilling')}</div>
          ) : isMobile ? (
            <div className="divide-y">
              {billing.map((item: any) => (
                <MobileListCard
                  key={item.id}
                  title={
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{item.tool_name}</code>
                  }
                  badge={
                    <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${billingStatusClass(item.billing_status)}`}>
                      {t(billingStatusKey(item.billing_status))}
                    </span>
                  }
                  meta={[
                    { label: t('wallet.consumed'), value: <span className="tabular-nums">{item.quota_consumed}</span> },
                    { label: t('wallet.unitPrice'), value: priceLabel(item.billing_type, item.unit_price, config.displayCurrency) },
                    { label: t('wallet.service'), value: item.service_name || '-' },
                    { label: t('wallet.time'), value: formatTime(item.created_at) },
                  ]}
                />
              ))}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('wallet.tool')}</TableHead>
                  <TableHead>{t('wallet.service')}</TableHead>
                  <TableHead>{t('wallet.status')}</TableHead>
                  <TableHead>{t('wallet.unitPrice')}</TableHead>
                  <TableHead>{t('wallet.consumed')}</TableHead>
                  <TableHead>{t('wallet.scope')}</TableHead>
                  <TableHead>{t('wallet.time')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {billing.map((item: any) => (
                  <TableRow key={item.id}>
                    <TableCell>
                      <code className="text-xs font-mono bg-muted px-1.5 py-0.5 rounded">{item.tool_name}</code>
                    </TableCell>
                    <TableCell className="text-sm">{item.service_name || '-'}</TableCell>
                    <TableCell>
                      <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${billingStatusClass(item.billing_status)}`}>
                        {t(billingStatusKey(item.billing_status))}
                      </span>
                    </TableCell>
                    <TableCell className="text-sm tabular-nums">
                      {priceLabel(item.billing_type, item.unit_price, config.displayCurrency)}
                    </TableCell>
                    <TableCell className="text-sm tabular-nums">{item.quota_consumed}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {item.price_scope ? t(priceScopeKey(item.price_scope)) : '-'}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground tabular-nums">{formatTime(item.created_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      </div>

      {/* Pagination */}
      {pagination && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">
            {t('wallet.total')} {pagination.total} {t('wallet.records')}
          </p>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1 || isFetching} onClick={() => setPage((p) => p - 1)}>
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <span className="text-sm tabular-nums">{page} / {totalPages}</span>
            <Button variant="outline" size="sm" disabled={page >= totalPages || isFetching} onClick={() => setPage((p) => p + 1)}>
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
