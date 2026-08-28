import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import dayjs from 'dayjs'
import type { HealthBucket } from '@/types'

// 桶成功率 → 颜色:0 红 → 0.5 黄 → 1 绿线性插值(色值与 CLIProxyAPI 参考色带一致)
const RATE_STOPS = [
  { r: 239, g: 68, b: 68 }, // #ef4444
  { r: 250, g: 204, b: 21 }, // #facc15
  { r: 34, g: 197, b: 94 }, // #22c55e
] as const

function rateToColor(rate: number): string {
  const t = Math.max(0, Math.min(1, rate))
  const seg = t < 0.5 ? 0 : 1
  const local = seg === 0 ? t * 2 : (t - 0.5) * 2
  const from = RATE_STOPS[seg]
  const to = seg === 0 ? RATE_STOPS[1] : RATE_STOPS[2]
  return `rgb(${Math.round(from.r + (to.r - from.r) * local)}, ${Math.round(from.g + (to.g - from.g) * local)}, ${Math.round(from.b + (to.b - from.b) * local)})`
}

const BUCKET_COUNT = 20

// 与 apiKeys 一致的相对时间展示;非法/缺失返回 null(调用方自行省略)
function timeAgo(t: ReturnType<typeof useTranslation>['t'], unix?: number): string | null {
  if (!unix) return null
  const diff = Date.now() - unix * 1000
  if (diff < 60_000) return t('services.overview.justNow')
  if (diff < 3_600_000) return t('services.overview.minutesAgo', { count: Math.floor(diff / 60_000) })
  if (diff < 86_400_000) return t('services.overview.hoursAgo', { count: Math.floor(diff / 3_600_000) })
  return t('services.overview.daysAgo', { count: Math.floor(diff / 86_400_000) })
}

// 非 stdio 状态徽章(对齐参考:点+文字 pill;绿=健康/琥珀=异常/灰=未知,红色只
// 出现在色带插值里)。后端 health_status 实时推导(连接>窗口成败>未知)。
const STATUS_PILL = {
  healthy: { box: 'border-emerald-500/35 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400', dot: 'bg-emerald-500' },
  unhealthy: { box: 'border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400', dot: 'bg-amber-500' },
  unknown: { box: 'border-border bg-muted text-muted-foreground', dot: 'bg-zinc-400 dark:bg-zinc-600' },
} as const

export function StatusPill({ kind, label }: { kind: keyof typeof STATUS_PILL; label: string }) {
  const v = STATUS_PILL[kind]
  return (
    <span className={`inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1.5 text-[10px] font-semibold leading-none my-0.5 ${v.box}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${v.dot}`} />
      {label}
    </span>
  )
}

// HealthBarData 色带面板所需的最小数据形状:总览的 ServicesOverviewItem 与市场
// 管理页的条目健康(后端字段同名)都结构兼容,可直接传入。
export interface HealthBarData {
  health_buckets?: HealthBucket[]
  last_error_message?: string
  last_error_at?: number
  last_call_at?: number
}

// ServiceHealthBar 非 stdio 服务的近期调用色带面板,替代 stdio 卡片的 CPU/内存两行:
// 近 200 分钟 = 20 格 × 10 分钟,每格按该时段成功率着色(红→黄→绿连续插值),
// 悬停看时段明细;行首窗口计数「成功 N · 失败 N」与整窗成功率,空窗时退化为
// 灰带 + 上次调用时间。数据来自 mcp_call_logs 真实调用聚合(被动口径,无探测)。
// wide:平台级口径(市场管理页),标签与提示注明含全部用户的调用。
export function ServiceHealthBar({ s, disabled, wide }: { s: HealthBarData; disabled: boolean; wide?: boolean }) {
  const { t } = useTranslation()
  // 自绘悬停提示:state 记录悬停格下标,气泡绝对定位其上。
  // (Radix Tooltip 在相邻触发器间移动时内容不刷新,色带这种 20 格连扫场景不可用)
  const [hover, setHover] = useState<number | null>(null)
  // 固定 20 格:聚合失败/字段缺失时兜底为全空桶,保持卡片形态稳定
  const buckets: HealthBucket[] =
    !disabled && s.health_buckets && s.health_buckets.length === BUCKET_COUNT
      ? s.health_buckets
      : Array.from({ length: BUCKET_COUNT }, () => ({ start_unix: 0, success: 0, failed: 0 }))
  const successTotal = buckets.reduce((acc, b) => acc + b.success, 0)
  const failTotal = buckets.reduce((acc, b) => acc + b.failed, 0)
  const total = successTotal + failTotal
  const rate = total > 0 ? successTotal / total : null
  const rateCls = rate == null
    ? 'text-muted-foreground'
    : rate >= 0.9 ? 'text-emerald-600 dark:text-emerald-400'
    : rate >= 0.5 ? 'text-amber-600 dark:text-amber-400'
    : 'text-red-600 dark:text-red-400'
  const ago = timeAgo(t, s.last_call_at)
  // 窗口内最近一次错误:悬停计数区可见,不影响布局
  const lastErrorTitle = s.last_error_message
    ? t('services.overview.healthLastError', {
        msg: s.last_error_message,
        time: s.last_error_at ? dayjs(s.last_error_at * 1000).format('MM-DD HH:mm') : '',
      })
    : undefined

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2 text-xs">
        <span className="text-muted-foreground" title={wide ? t('services.overview.recentCallsHintAllUsers') : t('services.overview.recentCallsHint')}>
          {wide ? t('services.overview.recentCallsAllUsers') : t('services.overview.recentCalls')}
        </span>
        <span className="tabular-nums" title={lastErrorTitle}>
          <span className={successTotal > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground'}>
            {t('services.overview.successCount', { count: successTotal })}
          </span>
          <span className={failTotal > 0 ? 'ml-2 text-red-600 dark:text-red-400' : 'ml-2 text-muted-foreground'}>
            {t('services.overview.failCount', { count: failTotal })}
          </span>
        </span>
      </div>
      <div className="flex items-center gap-2">
        <div className="relative min-w-0 flex-1" onMouseLeave={() => setHover(null)}>
          <div className="flex gap-[2px]">
            {buckets.map((b, i) => {
              const bTotal = b.success + b.failed
              const idle = disabled || bTotal === 0
              return (
                <div
                  key={i}
                  className={`h-3.5 min-w-[3px] flex-1 rounded-[2px] transition-shadow ${
                    idle ? 'bg-zinc-300/50 dark:bg-zinc-700/60' : ''
                  } ${hover === i ? 'ring-1 ring-white' : ''}`}
                  style={idle ? undefined : { backgroundColor: rateToColor(b.success / bTotal) }}
                  onMouseEnter={() => setHover(i)}
                />
              )
            })}
          </div>
          {/* 悬停气泡:居中于当前格上方;首/尾格改为左/右对齐,避免探出卡片边缘 */}
          {(() => {
            if (hover == null) return null
            const b = buckets[hover]
            if (!b) return null
            const bTotal = b.success + b.failed
            const start = dayjs(b.start_unix * 1000)
            const range = `${start.format('HH:mm')}–${start.add(10, 'minute').format('HH:mm')}`
            const align = hover === 0
              ? 'left-0'
              : hover === buckets.length - 1 ? 'right-0' : 'left-1/2 -translate-x-1/2'
            return (
              <div className={`pointer-events-none absolute bottom-full z-50 mb-1.5 whitespace-nowrap rounded-md bg-primary px-3 py-1.5 text-xs text-primary-foreground shadow-md animate-fade-in ${align}`}>
                {bTotal === 0
                  ? t('services.overview.healthNoDataTooltip', { range })
                  : t('services.overview.healthBucketTooltip', {
                      range,
                      success: b.success,
                      failed: b.failed,
                    })}
              </div>
            )
          })()}
        </div>
        <span className={`w-9 shrink-0 text-right text-xs font-semibold tabular-nums ${rateCls}`}>
          {rate == null ? '--' : `${Math.round(rate * 100)}%`}
        </span>
      </div>
      {/* 底部时间行:空窗=「暂无调用 · 上次调用X前」;有调用=「上次调用X前」。
          last_call_at 空窗时由后端点查全史,窗口内则取窗口最后一条 */}
      {ago && (
        <p className="text-[11px] text-muted-foreground">
          {total === 0 ? `${t('services.overview.noCalls')} · ` : ''}
          {t('services.overview.lastCall')} {ago}
        </p>
      )}
    </div>
  )
}
