import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { getWalletOverview, getWalletUsageStats, redeemCode, getInviteOverview, transferAffQuota } from '../api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { formatQuotaCurrency, currencySymbol } from '@/lib/billing'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'
import {
  Wallet as WalletIcon, TrendingUp, History, Gift, Activity, Zap, Coins, ArrowRight,
  UserPlus, Copy, Check, ArrowDownToLine,
} from 'lucide-react'
import type { WalletOverview, WalletUsageStats, InviteOverview } from '@/types'

export function WalletPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { config } = useSystemConfigStore()
  const [redeemInput, setRedeemInput] = useState('')
  const [transferInput, setTransferInput] = useState('')
  const [copiedKey, setCopiedKey] = useState('')

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

  const { data: inviteData } = useQuery({
    queryKey: ['invite-overview'],
    queryFn: getInviteOverview,
  })
  const invite: InviteOverview | undefined = inviteData?.data

  const redeemMutation = useMutation({
    mutationFn: () => redeemCode({ code: redeemInput.trim() }),
    onSuccess: (res) => {
      // 直接展示兑换码存储的充值面值(货币),不做额度换算——用户无需感知额度概念。
      const amount = res?.data?.amount ?? 0
      toast.success(t('wallet.redeemSuccess', { amount: `${currencySymbol(config.displayCurrency)}${amount.toFixed(2)}` }))
      setRedeemInput('')
      queryClient.invalidateQueries({ queryKey: ['wallet-overview'] })
      queryClient.invalidateQueries({ queryKey: ['wallet-usage-stats'] })
      // 充值后刷新统一日志,使新的充值记录可见。
      queryClient.invalidateQueries({ queryKey: ['user-logs'] })
      queryClient.invalidateQueries({ queryKey: ['user-log-stats'] })
    },
  })

  // 邀请奖励转入钱包:输入以货币单位计(quota / quotaPerUnit),提交时换算为 quota。
  const minTransferCurrency = config.quotaPerUnit > 0 ? 1 : 0 // 最小 1 货币单位
  const transferQuota = Math.round((Number(transferInput) || 0) * config.quotaPerUnit)
  const maxTransferCurrency =
    invite && config.quotaPerUnit > 0 ? invite.aff_quota / config.quotaPerUnit : 0
  const canTransfer =
    !!invite &&
    invite.aff_quota > 0 &&
    transferQuota >= config.quotaPerUnit &&
    transferQuota <= invite.aff_quota

  const transferMutation = useMutation({
    mutationFn: () => transferAffQuota({ quota: transferQuota }),
    onSuccess: (res) => {
      toast.success(t('wallet.transferSuccess', { quota: res?.data?.quota ?? 0 }))
      setTransferInput('')
      queryClient.invalidateQueries({ queryKey: ['wallet-overview'] })
      queryClient.invalidateQueries({ queryKey: ['invite-overview'] })
      queryClient.invalidateQueries({ queryKey: ['user-logs'] })
    },
  })

  const copyText = async (text: string, key: string) => {
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      setCopiedKey(key)
      toast.success(t('wallet.copied'))
      setTimeout(() => setCopiedKey(''), 1500)
    } catch {
      toast.error(t('wallet.copyFailed'))
    }
  }

  const billingDisabled = !config.billingEnabled

  const overviewCards = [
    { label: t('wallet.balance'), value: overview?.quota ?? 0, icon: WalletIcon, color: 'text-sky-500', bg: 'bg-sky-500/10', isQuota: true },
    { label: t('wallet.used'), value: overview?.used_quota ?? 0, icon: TrendingUp, color: 'text-violet-500', bg: 'bg-violet-500/10', isQuota: true },
    { label: t('wallet.topup'), value: overview?.total_topup ?? 0, icon: Coins, color: 'text-amber-500', bg: 'bg-amber-500/10', isQuota: true },
    { label: t('wallet.requestCount'), value: overview?.request_count ?? 0, icon: Activity, color: 'text-emerald-500', bg: 'bg-emerald-500/10', isQuota: false },
  ]

  const usageCards = [
    { label: t('wallet.consumedToday'), value: stats?.consumed_today ?? 0, icon: Zap, color: 'text-amber-500', bg: 'bg-amber-500/10', isQuota: true },
    { label: t('wallet.consumedWeek'), value: stats?.consumed_week ?? 0, icon: History, color: 'text-sky-500', bg: 'bg-sky-500/10', isQuota: true },
    { label: t('wallet.consumedTotal'), value: stats?.consumed_total ?? 0, icon: TrendingUp, color: 'text-violet-500', bg: 'bg-violet-500/10', isQuota: true },
  ]

  // 额度类数值按货币展示(quota / QuotaPerUnit),次数类保持整数。
  const fmt = (v: number, isQuota?: boolean) =>
    isQuota ? formatQuotaCurrency(v, config.quotaPerUnit, config.displayCurrency) : v.toLocaleString()

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
                {overviewLoading ? '...' : fmt(card.value, card.isQuota)}
                {card.isQuota ? null : (
                  <span className="ml-1 text-xs font-normal text-muted-foreground">{t('wallet.countUnit')}</span>
                )}
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
              {stats ? fmt(card.value, card.isQuota) : '...'}
              {card.isQuota ? null : (
                <span className="ml-1 text-xs font-normal text-muted-foreground">{t('wallet.countUnit')}</span>
              )}
            </p>
          </div>
        ))}
      </div>

      {/* 消费明细已合并到「调用日志」页统一展示 */}

      {/* 邀请奖励 */}
      <div className="rounded-xl border bg-card p-4 sm:p-6 space-y-4">
        <div className="flex items-center gap-2">
          <UserPlus className="h-4 w-4 text-primary" />
          <p className="text-sm font-medium">{t('wallet.inviteTitle')}</p>
        </div>
        <p className="text-xs text-muted-foreground">{t('wallet.inviteHint')}</p>

        {/* 邀请码 + 邀请链接 */}
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <p className="text-xs text-muted-foreground">{t('wallet.inviteCode')}</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 rounded-md bg-muted px-3 py-2 font-mono text-sm tracking-wider">
                {invite?.aff_code || '...'}
              </code>
              <Button
                variant="outline"
                size="icon"
                className="shrink-0"
                onClick={() => copyText(invite?.aff_code || '', 'code')}
                disabled={!invite?.aff_code}
              >
                {copiedKey === 'code' ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
          </div>
          <div className="space-y-1.5">
            <p className="text-xs text-muted-foreground">{t('wallet.inviteLink')}</p>
            <div className="flex items-center gap-2">
              <Input
                readOnly
                value={invite?.invite_url || ''}
                className="font-mono text-xs"
                placeholder="..."
              />
              <Button
                variant="outline"
                size="icon"
                className="shrink-0"
                onClick={() => copyText(invite?.invite_url || '', 'link')}
                disabled={!invite?.invite_url}
              >
                {copiedKey === 'link' ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
          </div>
        </div>

        {/* 邀请统计 */}
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="rounded-lg bg-muted/50 p-3">
            <p className="text-xs text-muted-foreground">{t('wallet.invitedCount')}</p>
            <p className="mt-1 text-xl font-semibold tabular-nums">{invite?.aff_count ?? 0}</p>
          </div>
          <div className="rounded-lg bg-muted/50 p-3">
            <p className="text-xs text-muted-foreground">{t('wallet.inviteRewardBalance')}</p>
            <p className="mt-1 text-xl font-semibold tabular-nums">
              {invite ? fmt(invite.aff_quota, true) : '...'}
            </p>
          </div>
          <div className="rounded-lg bg-muted/50 p-3">
            <p className="text-xs text-muted-foreground">{t('wallet.inviteRewardTotal')}</p>
            <p className="mt-1 text-xl font-semibold tabular-nums">
              {invite ? fmt(invite.aff_history_quota, true) : '...'}
            </p>
          </div>
        </div>

        {/* 转入钱包 */}
        <div className="space-y-2 border-t pt-4">
          <div className="flex items-center gap-2">
            <ArrowDownToLine className="h-4 w-4 text-primary" />
            <p className="text-sm font-medium">{t('wallet.transferToWallet')}</p>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('wallet.transferDesc', { min: minTransferCurrency, max: maxTransferCurrency.toFixed(2) })}
          </p>
          <div className="flex gap-2">
            <Input
              type="number"
              inputMode="decimal"
              placeholder={t('wallet.transferPlaceholder', { min: minTransferCurrency })}
              value={transferInput}
              onChange={(e) => setTransferInput(e.target.value)}
              disabled={!invite || invite.aff_quota <= 0}
              className="flex-1"
            />
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              disabled={!invite || invite.aff_quota <= 0}
              onClick={() => setTransferInput(maxTransferCurrency.toString())}
            >
              {t('wallet.transferAll')}
            </Button>
            <Button
              size="sm"
              className="shrink-0"
              disabled={!canTransfer || transferMutation.isPending}
              onClick={() => transferMutation.mutate()}
            >
              {transferMutation.isPending ? t('common.loading') : t('wallet.transfer')}
            </Button>
          </div>
        </div>
      </div>

      <div className="flex justify-end">
        <Button variant="outline" size="sm" onClick={() => navigate({ to: '/logs', search: { type: 2 } })}>
          {t('wallet.viewDetails')}
          <ArrowRight className="ml-1.5 h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  )
}
