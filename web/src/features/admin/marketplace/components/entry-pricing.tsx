import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { MarketplaceDetail, MarketplaceEntryKind, MarketplaceEntryPrice } from '@/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

// 条目级定价(§5.2):市场管理详情页对工具/资源/提示逐条设价。
// 模式:inherit=回退(工具→服务统一价,资源/提示→免费)/ free=免费 / custom=自定义按次价。
// 保存为全量替换(PUT /admin/marketplace/:id/entry-prices),不在载荷中的条目回退。

export type EntryPriceMode = 'inherit' | 'free' | 'custom'

export interface EntryDraftValue {
  mode: EntryPriceMode
  price: string // custom 模式下的单价输入(字符串暂存,提交时解析)
}

function entryKey(kind: MarketplaceEntryKind, name: string) {
  return `${kind}:${name}`
}

function defaultMode(kind: MarketplaceEntryKind): EntryPriceMode {
  return kind === 'tool' ? 'inherit' : 'free'
}

// buildServerDraft 服务端当前态:entry_prices 命中 → free/custom(回填价格);
// 未命中 → 工具 inherit / 资源提示 free。
function buildServerDraft(item?: MarketplaceDetail): Record<string, EntryDraftValue> {
  const out: Record<string, EntryDraftValue> = {}
  if (!item) return out
  const set = (kind: MarketplaceEntryKind, name: string) => {
    const saved = (item.entry_prices || []).find((p) => p.kind === kind && p.name === name)
    out[entryKey(kind, name)] = saved
      ? saved.billing_type === 'free'
        ? { mode: 'free', price: '0' }
        : { mode: 'custom', price: String(saved.price_per_call) }
      : { mode: defaultMode(kind), price: '' }
  }
  ;(item.tools_snapshot || []).forEach((tool) => set('tool', tool.name))
  ;(item.resources_snapshot?.resources || []).forEach((r) => set('resource', r.uri))
  ;(item.prompts_snapshot || []).forEach((p) => set('prompt', p.name))
  return out
}

const sameValue = (a?: EntryDraftValue, b?: EntryDraftValue): boolean => {
  if (!a || !b) return a === b
  if (a.mode !== b.mode) return false
  if (a.mode !== 'custom') return true
  return (parseFloat(a.price) || 0) === (parseFloat(b.price) || 0)
}

// useEntryPricingDraft 条目定价草稿:本地覆盖只记被改过的条目,未改条目始终随服务端
// 数据走(保存成功/快照刷新后无需手工重置);dirtyCount=覆盖与服务端不一致的条目数。
export function useEntryPricingDraft(item?: MarketplaceDetail) {
  const [overrides, setOverrides] = useState<Record<string, EntryDraftValue>>({})
  const serverDraft = useMemo(() => buildServerDraft(item), [item])

  const getEntry = (kind: MarketplaceEntryKind, name: string): EntryDraftValue =>
    overrides[entryKey(kind, name)] ?? serverDraft[entryKey(kind, name)] ?? { mode: defaultMode(kind), price: '' }

  const setEntry = (kind: MarketplaceEntryKind, name: string, v: EntryDraftValue) =>
    setOverrides((prev) => ({ ...prev, [entryKey(kind, name)]: v }))

  const reset = () => setOverrides({})

  const dirtyCount = useMemo(
    () => Object.entries(overrides).filter(([key, v]) => !sameValue(serverDraft[key], v)).length,
    [overrides, serverDraft],
  )

  // buildPayload 导出完整条目价列表(非 inherit 项)。custom 价 <=0 时返回 null,调用方提示并阻断提交。
  const buildPayload = (): MarketplaceEntryPrice[] | null => {
    if (!item) return []
    const entries: Array<[MarketplaceEntryKind, string]> = [
      ...(item.tools_snapshot || []).map((tool) => ['tool', tool.name] as [MarketplaceEntryKind, string]),
      ...(item.resources_snapshot?.resources || []).map((r) => ['resource', r.uri] as [MarketplaceEntryKind, string]),
      ...(item.prompts_snapshot || []).map((p) => ['prompt', p.name] as [MarketplaceEntryKind, string]),
    ]
    const out: MarketplaceEntryPrice[] = []
    for (const [kind, name] of entries) {
      const v = getEntry(kind, name)
      if (v.mode === 'inherit') continue
      if (v.mode === 'free') {
        out.push({ kind, name, billing_type: 'free', price_per_call: 0 })
        continue
      }
      const price = parseFloat(v.price) || 0
      if (price <= 0) return null
      out.push({ kind, name, billing_type: 'per_call', price_per_call: price })
    }
    return out
  }

  return { getEntry, setEntry, reset, dirtyCount, buildPayload }
}

// EntryPriceControl 单条目定价控件(名称行右侧):模式小下拉 + custom 时单价输入。
// 工具三态(含"继承服务价"),资源/提示两态(默认免费,无继承态)。
export function EntryPriceControl({ kind, value, onChange }: {
  kind: MarketplaceEntryKind
  value: EntryDraftValue
  onChange: (v: EntryDraftValue) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-1.5">
      <Select
        value={value.mode}
        onValueChange={(mode) =>
          onChange({ mode: mode as EntryPriceMode, price: mode === 'free' ? '0' : value.price })}
      >
        <SelectTrigger className="h-7 w-28 text-xs" aria-label={t('marketplace.entryPricingTitle')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {kind === 'tool' && <SelectItem value="inherit">{t('marketplace.entryModeInherit')}</SelectItem>}
          <SelectItem value="free">{t('billing.free')}</SelectItem>
          <SelectItem value="custom">{t('marketplace.entryModeCustom')}</SelectItem>
        </SelectContent>
      </Select>
      {value.mode === 'custom' && (
        <Input
          type="number"
          min="0"
          step="0.0001"
          className="h-7 w-24 text-xs"
          value={value.price}
          onChange={(e) => onChange({ ...value, price: e.target.value })}
        />
      )}
    </div>
  )
}

// EntryPricingBar 底部保存条(仿分组批量保存):有待保存条目时出现。
export function EntryPricingBar({ dirtyCount, pending, onCancel, onSave }: {
  dirtyCount: number
  pending: boolean
  onCancel: () => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  if (dirtyCount === 0) return null
  return (
    <div className="sticky bottom-4 z-10 flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-card p-3 shadow-lg">
      <span className="text-xs text-muted-foreground">{t('marketplace.entryPending', { count: dirtyCount })}</span>
      <div className="flex gap-2">
        <Button variant="outline" size="sm" disabled={pending} onClick={onCancel}>
          {t('common.cancel')}
        </Button>
        <Button size="sm" disabled={pending} onClick={onSave}>
          {t('marketplace.entrySave')}
        </Button>
      </div>
    </div>
  )
}
