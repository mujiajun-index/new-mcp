import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import { Progress } from '@/components/ui/progress'
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from '@/components/ui/tooltip'
import type { HealthBucket, HealthState, ServicesOverviewItem } from '@/types'

// 档位 → 分数条颜色(指示色 + 底色),与后端 service/health_score.go 档位阈值
// (≥90 healthy / ≥70 ok / ≥50 degraded / <50 critical)对齐;完整静态类名保证
// Tailwind 4 能扫描到
const STATE_BAR_CLS: Record<HealthState, string> = {
  healthy: 'bg-emerald-500/15 [&_[data-slot=progress-indicator]]:bg-emerald-500',
  ok: 'bg-green-500/15 [&_[data-slot=progress-indicator]]:bg-green-500',
  degraded: 'bg-amber-500/15 [&_[data-slot=progress-indicator]]:bg-amber-500',
  critical: 'bg-red-500/15 [&_[data-slot=progress-indicator]]:bg-red-500',
  no_data: 'bg-muted',
}

// 单个小时桶 → 时间条格色:无调用灰,全成功绿,≥80% 黄,<80% 红
function bucketCls(b: HealthBucket): string {
  if (b.total <= 0) return 'bg-zinc-200 dark:bg-zinc-700'
  if (b.success >= b.total) return 'bg-emerald-500'
  if (b.success / b.total >= 0.8) return 'bg-amber-500'
  return 'bg-red-500'
}

// ServiceHealthBar 非 stdio 服务的健康状态条,替代 stdio 卡片的 CPU/内存两行
// (行高等高):行 1 近 1h 健康分(0-100 + 档位文案 + 档位色分数条),行 2 近 24h
// 每小时一格的成功/失败时间条,悬停看该小时明细。数据来自 mcp_call_logs 真实
// 调用聚合,无调用的服务显示灰色「暂无调用」。
export function ServiceHealthBar({ s, disabled }: { s: ServicesOverviewItem; disabled: boolean }) {
  const { t } = useTranslation()
  const hasData = !disabled && s.health_score != null
  const state = (disabled || !s.health_state) ? 'no_data' : s.health_state
  const buckets = s.health_buckets ?? []

  // 最近一次错误(24h 内)悬停分数值可见,不影响布局
  const lastErrorTitle = s.last_error_message
    ? t('services.overview.healthLastError', {
        msg: s.last_error_message,
        time: s.last_error_at ? dayjs(s.last_error_at * 1000).format('MM-DD HH:mm') : '',
      })
    : undefined

  return (
    <>
      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">{t('services.overview.healthScoreRow')}</span>
          <span className="tabular-nums" title={lastErrorTitle}>
            {hasData
              ? `${s.health_score} · ${t(`services.overview.healthState.${state}`)}`
              : '—'}
          </span>
        </div>
        <Progress
          value={hasData ? s.health_score! : 0}
          className={`h-1 ${STATE_BAR_CLS[state]}`}
        />
      </div>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">{t('services.overview.healthStripRow')}</span>
          <span className="text-[10px] text-muted-foreground">
            {t('services.overview.healthStripWindow')}
          </span>
        </div>
        <TooltipProvider delayDuration={100}>
          <div className="flex h-1 gap-px">
            {Array.from({ length: 24 }, (_, i) => {
              const b = buckets[i]
              const start = dayjs((b?.start_unix ?? 0) * 1000)
              const empty = disabled || !b || b.total <= 0
              return (
                <Tooltip key={i}>
                  <TooltipTrigger asChild>
                    <div
                      className={`h-1 flex-1 rounded-[1px] ${empty ? 'bg-zinc-200 dark:bg-zinc-700' : bucketCls(b)}`}
                    />
                  </TooltipTrigger>
                  <TooltipContent>
                    {empty ? (
                      t('services.overview.healthNoDataTooltip')
                    ) : (
                      t('services.overview.healthBucketTooltip', {
                        range: `${start.format('HH:00')}–${start.add(1, 'hour').format('HH:00')}`,
                        success: b.success,
                        total: b.total,
                        avg: b.avg_duration_ms,
                      })
                    )}
                  </TooltipContent>
                </Tooltip>
              )
            })}
          </div>
        </TooltipProvider>
      </div>
    </>
  )
}
