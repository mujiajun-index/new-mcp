import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Check, ChevronDown, ChevronRight, Settings2 } from 'lucide-react'

// 分组内资源/提示的条目级勾选管理(与工具管理同一交互,独立成组件避免页面继续膨胀)。
// 工具管理面板因带 ToolParams 展示,仍保留在 group-detail-page 内。
export type FilterableItem = {
  service_id: number
  service_name: string
  kind: 'resource' | 'template' | 'prompt'
  key: string
  description?: string
  meta?: string[]
  enabled: boolean
}

export type FilterableUpdate = {
  service_id: number
  kind: FilterableItem['kind']
  key: string
  enabled: boolean
}

type Props = {
  title: string
  manageLabel: string
  searchPlaceholder: string
  items: FilterableItem[]
  saving: boolean
  onBatchSave: (updates: FilterableUpdate[]) => void
}

export function GroupItemFilterSection({ title, manageLabel, searchPlaceholder, items, saving, onBatchSave }: Props) {
  const { t } = useTranslation()
  const [showManager, setShowManager] = useState(false)
  const [states, setStates] = useState<Record<string, boolean>>({})
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState('')

  const enabledCount = items.filter((i) => i.enabled).length

  const itemsByService = useMemo(() => {
    const map = new Map<string, FilterableItem[]>()
    for (const item of items) {
      const list = map.get(item.service_name) || []
      list.push(item)
      map.set(item.service_name, list)
    }
    return map
  }, [items])

  const filteredByService = useMemo(() => {
    if (!search.trim()) return itemsByService
    const q = search.toLowerCase()
    const filtered = new Map<string, FilterableItem[]>()
    for (const [svcName, svcItems] of itemsByService) {
      const matching = svcItems.filter(
        (i) =>
          i.key.toLowerCase().includes(q) ||
          i.description?.toLowerCase().includes(q) ||
          svcName.toLowerCase().includes(q),
      )
      if (matching.length > 0) filtered.set(svcName, matching)
    }
    return filtered
  }, [itemsByService, search])

  const changed = useMemo(() => {
    const updates: FilterableUpdate[] = []
    for (const item of items) {
      const k = itemKey(item)
      if (k in states && states[k] !== item.enabled) {
        updates.push({ service_id: item.service_id, kind: item.kind, key: item.key, enabled: states[k] ?? true })
      }
    }
    return updates
  }, [items, states])

  function itemKey(item: FilterableItem) {
    return `${item.service_id}:${item.kind}:${item.key}`
  }

  function openManager() {
    const s: Record<string, boolean> = {}
    for (const item of items) s[itemKey(item)] = item.enabled
    setStates(s)
    setExpanded(new Set(itemsByService.keys()))
    setSearch('')
    setShowManager(true)
  }

  function toggleItem(item: FilterableItem) {
    const k = itemKey(item)
    setStates((prev) => ({ ...prev, [k]: !prev[k] }))
  }

  function toggleService(serviceItems: FilterableItem[]) {
    const allEnabled = serviceItems.every((i) => states[itemKey(i)] !== false)
    setStates((prev) => {
      const next = { ...prev }
      for (const i of serviceItems) next[itemKey(i)] = !allEnabled
      return next
    })
  }

  function toggleExpanded(name: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  return (
    <div className="rounded-xl border bg-card p-5">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold">{title}</h2>
        {items.length > 0 && (
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => (showManager ? setShowManager(false) : openManager())}>
            <Settings2 className="h-3.5 w-3.5" />
            {showManager ? t('groups.collapse') : manageLabel}
          </Button>
        )}
      </div>

      {items.length === 0 ? null : showManager ? (
        <div className="space-y-3">
          <div className="relative">
            <input
              type="text"
              placeholder={searchPlaceholder}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full rounded-lg border bg-muted/30 px-3 py-2 text-sm outline-none focus:border-primary"
            />
          </div>

          <div className="space-y-2 max-h-[480px] overflow-y-auto">
            {Array.from(filteredByService.entries()).map(([svcName, svcItems]) => {
              const isExpanded = expanded.has(svcName)
              const enabledInSvc = svcItems.filter((i) => states[itemKey(i)] !== false).length
              const allEnabled = enabledInSvc === svcItems.length
              return (
                <div key={svcName} className="rounded-lg border">
                  <div
                    className="w-full flex items-center justify-between px-3 py-2.5 hover:bg-muted/30 transition-colors cursor-pointer"
                    onClick={() => toggleExpanded(svcName)}
                  >
                    <div className="flex items-center gap-2">
                      {isExpanded ? <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" /> : <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />}
                      <span className="text-sm font-medium">{svcName}</span>
                      <span className="text-xs text-muted-foreground">{t('groups.enabledCount', { enabled: enabledInSvc, total: svcItems.length })}</span>
                    </div>
                    <Button variant="ghost" size="sm" className="h-6 px-2 text-xs" onClick={(e) => { e.stopPropagation(); toggleService(svcItems) }}>
                      {allEnabled ? t('groups.disableAll') : t('groups.enableAll')}
                    </Button>
                  </div>

                  {isExpanded && (
                    <div className="border-t">
                      {svcItems.map((item) => {
                        const k = itemKey(item)
                        const isEnabled = states[k] !== false
                        return (
                          <div
                            key={k}
                            className="flex items-center gap-3 px-3 py-2 hover:bg-muted/20 transition-colors cursor-pointer"
                            onClick={() => toggleItem(item)}
                          >
                            <span className={`flex items-center justify-center h-4 w-4 shrink-0 rounded border transition-colors ${
                              isEnabled ? 'bg-primary border-primary text-primary-foreground' : 'border-muted-foreground/30'
                            }`}>
                              {isEnabled && <Check className="h-3 w-3" />}
                            </span>
                            <div className="flex-1 min-w-0">
                              <p className="text-sm font-mono break-all">{item.key}</p>
                              {item.description && <p className="text-xs text-muted-foreground break-all">{item.description}</p>}
                              {item.meta && item.meta.length > 0 && (
                                <div className="mt-0.5 flex flex-wrap gap-1">
                                  {item.meta.map((m) => (
                                    <span key={m} className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{m}</span>
                                  ))}
                                </div>
                              )}
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  )}
                </div>
              )
            })}
          </div>

          <div className="flex items-center justify-between pt-2 border-t">
            <span className="text-xs text-muted-foreground">
              {changed.length > 0 ? t('groups.changesPending', { count: changed.length }) : t('groups.noChanges')}
            </span>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => { setShowManager(false); setStates({}) }}>
                {t('common.cancel')}
              </Button>
              <Button size="sm" disabled={changed.length === 0 || saving} onClick={() => onBatchSave(changed)}>
                {saving ? t('groups.saving') : t('groups.saveChanges', { count: changed.length })}
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <div className="space-y-2">
          {enabledCount === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-6">{t('groups.noEnabledItemsHint')}</p>
          ) : (
            items.filter((i) => i.enabled).map((item) => (
              <div key={itemKey(item)} className="flex items-start gap-3 rounded-lg border p-3">
                <span className="mt-0.5 h-2 w-2 shrink-0 rounded-full bg-emerald-500" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium font-mono break-all">{item.key}</p>
                  <p className="text-xs text-muted-foreground">{t('groups.itemFrom', { name: item.service_name })}</p>
                  {item.description && <p className="mt-1 text-xs text-muted-foreground break-all">{item.description}</p>}
                  {item.meta && item.meta.length > 0 && (
                    <div className="mt-0.5 flex flex-wrap gap-1">
                      {item.meta.map((m) => (
                        <span key={m} className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">{m}</span>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  )
}
