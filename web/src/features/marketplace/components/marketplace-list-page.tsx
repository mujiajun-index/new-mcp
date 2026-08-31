import { useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getMarketplaceItems, getMarketplaceGroups } from '../api'
import { useTagColors, tagStyle } from '../tag-colors'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel } from '@/lib/billing'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import type { MarketplaceListItem } from '@/types'
import { Search, Store, Download, Star, Zap, Code2, FolderTree, X } from 'lucide-react'

export function MarketplaceListPage() {
  const { t } = useTranslation()
  const { config } = useSystemConfigStore()
  const [keyword, setKeyword] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [category, setCategory] = useState<'' | 'instant' | 'source'>('')
  const [groupId, setGroupId] = useState<number | ''>('')
  const [tag, setTag] = useState('')
  const [groupsOpen, setGroupsOpen] = useState(false)
  const [page, setPage] = useState(1)
  const pageSize = 12

  const { data: groupsData } = useQuery({ queryKey: ['marketplace-groups'], queryFn: getMarketplaceGroups })
  const groups: any[] = groupsData?.data ?? []
  // 当前生效的分组筛选对象(统计条取消 chip 展示用;groupId 为 '' 时无)
  const activeGroup = typeof groupId === 'number' ? groups.find((g) => g.id === groupId) : undefined
  const tagColors = useTagColors()

  const { data, isLoading } = useQuery({
    queryKey: ['marketplace', keyword, category, groupId, tag, page],
    queryFn: () => getMarketplaceItems({
      keyword: keyword || undefined,
      category: category || undefined,
      group_id: groupId || undefined,
      tag: tag || undefined,
      page,
      page_size: pageSize,
    }),
  })

  const items: MarketplaceListItem[] = data?.data || []
  const pagination = data?.pagination
  const totalPages = pagination?.total_pages ?? 1

  return (
    <div className="flex gap-6 p-4 sm:p-6 lg:p-8">
      {/* 左侧分组筛选(桌面端,由顶部「分组」按钮控制显示) */}
      {groupsOpen && (
        <aside className="hidden w-56 shrink-0 lg:block">
          <div className="sticky top-4 space-y-1">
            <p className="mb-2 px-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t('categories.groups')}
            </p>
            <button
              onClick={() => { setGroupId(''); setPage(1) }}
              className={`flex w-full items-center rounded-lg px-3 py-2 text-sm transition-colors ${groupId === '' ? 'bg-primary/10 font-medium text-primary' : 'text-sidebar-foreground/70 hover:bg-muted'}`}
            >
              {t('marketplace.filterAll')}
            </button>
            {groups.map((g) => (
              <button key={g.id} onClick={() => { setGroupId(g.id); setPage(1) }}
                className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors ${groupId === g.id ? 'bg-primary/10 font-medium text-primary' : 'text-sidebar-foreground/70 hover:bg-muted'}`}>
                {g.icon_url ? <img src={g.icon_url} alt="" className="h-4 w-4" /> : <FolderTree className="h-3.5 w-3.5 shrink-0" />}
                <span className="truncate">{g.display_name || g.name}</span>
                {/* 分组下已上架服务数(与按组筛选口径一致) */}
                <span className="ml-auto shrink-0 text-xs tabular-nums text-muted-foreground">{g.item_count}</span>
              </button>
            ))}
          </div>
        </aside>
      )}

      {/* 右侧主内容 */}
      <div className="min-w-0 flex-1 space-y-6">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('marketplace.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('marketplace.subtitle')}</p>
        </div>

        {/* Filters */}
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <form onSubmit={(e) => { e.preventDefault(); setKeyword(searchInput); setPage(1) }} className="relative max-w-sm flex-1">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input placeholder={t('marketplace.searchPlaceholder')} value={searchInput} onChange={(e) => setSearchInput(e.target.value)} className="pl-9" />
          </form>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant={groupsOpen ? 'default' : 'outline'} size="sm" className="gap-1.5" onClick={() => setGroupsOpen((v) => !v)}>
              <FolderTree className="h-3.5 w-3.5" />{t('categories.groups')}
            </Button>
          </div>
          {/* 服务类型筛选靠右;无「全部」按钮,点击选中、再点取消 */}
          <div className="flex flex-wrap items-center gap-2 sm:ml-auto">
            <span className="text-xs text-muted-foreground">{t('marketplace.categoryLabel')}</span>
            <Button variant={category === 'instant' ? 'default' : 'outline'} size="sm" className="gap-1.5"
              onClick={() => { setCategory(category === 'instant' ? '' : 'instant'); setPage(1) }}>
              <Zap className="h-3.5 w-3.5" />{t('marketplace.filterReady')}
            </Button>
            <Button variant={category === 'source' ? 'default' : 'outline'} size="sm" className="gap-1.5"
              onClick={() => { setCategory(category === 'source' ? '' : 'source'); setPage(1) }}>
              <Code2 className="h-3.5 w-3.5" />{t('marketplace.filterSource')}
            </Button>
          </div>
        </div>

        {/* 分组筛选:移动端/平板为横向滚动条,桌面端(≥lg)用左侧边栏 */}
        {groupsOpen && (
          <div className="flex gap-2 overflow-x-auto pb-1 lg:hidden">
            <Button variant={groupId === '' ? 'default' : 'outline'} size="sm" className="shrink-0" onClick={() => { setGroupId(''); setPage(1) }}>
              {t('marketplace.filterAll')}
            </Button>
            {groups.map((g) => (
              <Button key={g.id} variant={groupId === g.id ? 'default' : 'outline'} size="sm" className="shrink-0 gap-1.5"
                onClick={() => { setGroupId(g.id); setPage(1) }}>
                {g.icon_url ? <img src={g.icon_url} alt="" className="h-3.5 w-3.5" /> : <FolderTree className="h-3.5 w-3.5" />}
                {g.display_name || g.name}
                <span className="text-[10px] tabular-nums opacity-60">{g.item_count}</span>
              </Button>
            ))}
          </div>
        )}

        {/* 结果统计条:总数 + 当前生效筛选 chip(点 chip 取消对应查询) */}
        {!isLoading && pagination && (
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm text-muted-foreground">{t('marketplace.foundResults', { count: pagination.total })}</p>
            {activeGroup && (
              <button type="button" onClick={() => { setGroupId(''); setPage(1) }}
                className="flex cursor-pointer items-center gap-1.5 rounded-full border bg-muted/40 px-2.5 py-1 text-xs font-medium transition-colors hover:bg-muted">
                {activeGroup.display_name || activeGroup.name}
                <X className="h-3 w-3 text-muted-foreground" />
              </button>
            )}
            {tag && (
              <button type="button" onClick={() => { setTag(''); setPage(1) }}
                style={tagColors[tag] ? tagStyle(tagColors[tag]) : undefined}
                className={`flex cursor-pointer items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${
                  tagColors[tag] ? 'transition-opacity hover:opacity-75' : 'border bg-muted/40 transition-colors hover:bg-muted'
                }`}>
                {tag}
                <X className="h-3 w-3 opacity-60" />
              </button>
            )}
            {keyword && (
              <button type="button" onClick={() => { setKeyword(''); setSearchInput(''); setPage(1) }}
                className="flex cursor-pointer items-center gap-1.5 rounded-full border bg-muted/40 px-2.5 py-1 text-xs font-medium transition-colors hover:bg-muted">
                {t('marketplace.searchChip', { keyword })}
                <X className="h-3 w-3 text-muted-foreground" />
              </button>
            )}
          </div>
        )}

        {/* Grid */}
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Store className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('marketplace.noServices')}</p>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {items.map((item) => {
              const groupNames = item.group_names ?? []
              const tagNames = item.tags ?? []
              const firstTag = tagNames[0] ?? '' // >2 汇总分支用,长度分支保证非空
              // 分组筛选 chip:行内(≤2 个)与弹层(>2 个)共用。chip 只携带名字,由分组
              // 列表反查 id;绑定仅指向启用分组,正常必命中,查不到则不动作。close 供弹层项点击后收起
              const groupChip = (name: string, close?: () => void) => {
                const g = groups.find((gr) => gr.name === name)
                return (
                  <button key={name} type="button"
                    onClick={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      close?.()
                      if (g) { setGroupId(g.id); setPage(1) }
                    }}
                    className="cursor-pointer rounded bg-blue-500/10 px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-500/25 dark:text-blue-400">
                    {name}
                  </button>
                )
              }
              // 标签筛选 chip:点击筛选,再点当前标签取消;沿用字典配色
              const tagChip = (tagName: string, close?: () => void) => (
                <button key={tagName} type="button"
                  onClick={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    close?.()
                    setTag(tag === tagName ? '' : tagName)
                    setPage(1)
                  }}
                  style={tagStyle(tagColors[tagName])}
                  className={`cursor-pointer rounded px-1.5 py-0.5 text-[10px] font-medium transition-opacity hover:opacity-75 ${tagColors[tagName] ? '' : 'bg-muted text-muted-foreground'} ${tag === tagName ? 'ring-1 ring-primary/60' : ''}`}>
                  {tagName}
                </button>
              )
              return (
                <Link key={item.id} to="/marketplace/$id" params={{ id: String(item.id) }}>
                  <div className="group flex h-full flex-col rounded-xl border bg-card p-5 transition-all hover:border-primary/20 hover:shadow-md hover:shadow-black/[0.03]">
                    <div className="flex items-start gap-3">
                      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                        {item.icon_url ? <img src={item.icon_url} alt="" className="h-6 w-6" /> : <span className="text-lg font-bold">{(item.display_name || item.name).charAt(0)}</span>}
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <h3 className="truncate font-semibold transition-colors group-hover:text-primary">{item.display_name || item.name}</h3>
                          {/* 部署形态标识(即用型/源码型)紧跟名称,版本号随后 */}
                          <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${item.category === 'instant' ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'}`}>
                            {item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}
                          </span>
                          {item.version && <span className="shrink-0 text-[10px] text-muted-foreground">v{item.version}</span>}
                        </div>
                        {/* 分组/标签 ≤2 个逐个展示;>2 个收敛为「首个等N个」汇总 chip,悬停/点击弹出全部 */}
                        <div className="mt-0.5 flex flex-wrap items-center gap-2">
                          {groupNames.length > 2 ? (
                            <OverflowChip
                              summary={t('marketplace.moreGroups', { first: groupNames[0], count: groupNames.length, more: groupNames.length - 1 })}
                              summaryClassName="rounded bg-blue-500/10 px-1.5 py-0.5 text-[10px] font-medium text-blue-600 dark:text-blue-400">
                              {(close) => groupNames.map((name) => groupChip(name, close))}
                            </OverflowChip>
                          ) : (
                            groupNames.map((name) => groupChip(name))
                          )}
                          {tagNames.length > 2 ? (
                            <OverflowChip
                              summary={t('marketplace.moreTags', { first: firstTag, count: tagNames.length, more: tagNames.length - 1 })}
                              summaryClassName={`rounded px-1.5 py-0.5 text-[10px] font-medium ${tagColors[firstTag] ? '' : 'bg-muted text-muted-foreground'}`}
                              summaryStyle={tagColors[firstTag] ? tagStyle(tagColors[firstTag]) : undefined}>
                              {(close) => tagNames.map((tagName) => tagChip(tagName, close))}
                            </OverflowChip>
                          ) : (
                            tagNames.map((tagName) => tagChip(tagName))
                          )}
                        </div>
                      </div>
                    </div>
                    {item.description && <p className="mt-3 line-clamp-2 flex-1 text-sm text-muted-foreground">{item.description}</p>}
                    <div className="mt-3 flex items-center justify-between gap-2">
                      <span className="text-sm font-semibold text-primary">{priceLabel(item.billing_type, item.price_per_call, config.displayCurrency)}</span>
                      <div className="flex items-center gap-3 text-xs text-muted-foreground">
                        <span className="flex items-center gap-1"><Download className="h-3 w-3" />{item.install_count}</span>
                        {item.rating_count > 0 && <span className="flex items-center gap-1"><Star className="h-3 w-3" />{item.rating_avg.toFixed(1)}</span>}
                      </div>
                    </div>
                  </div>
                </Link>
              )
            })}
          </div>
        )}

        {/* Pagination */}
        {pagination && (
          <div className="flex items-center justify-between">
            <p className="text-sm text-muted-foreground">{t('common.total')} {pagination.total} {t('common.items')}</p>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>‹</Button>
              <span className="text-sm tabular-nums">{page} / {totalPages}</span>
              <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>›</Button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// OverflowChip 卡片分组/标签过多时的「首个等N个」汇总 chip:悬停或点击弹出全部,
// 弹层内每项仍可点击筛选(通过 children 渲染回调拿到 close)。交互对齐健康度色带
// 的自绘悬停气泡;弹层与 chip 之间不留 margin 缝隙(缝隙不属于容器,鼠标穿过即触发
// mouseleave 提前收起——上一版 bug),留白改用弹层自身 pt;另加 150ms 延迟关闭,
// 斜向移入弹层也稳,重新进入容器即取消。
function OverflowChip({ summary, summaryClassName, summaryStyle, children }: {
  summary: string
  summaryClassName: string
  summaryStyle?: CSSProperties
  children: (close: () => void) => ReactNode
}) {
  const [open, setOpen] = useState(false)
  const closeTimer = useRef<number | null>(null)
  const cancelClose = () => {
    if (closeTimer.current != null) {
      window.clearTimeout(closeTimer.current)
      closeTimer.current = null
    }
  }
  return (
    <div
      className="relative"
      onMouseEnter={() => { cancelClose(); setOpen(true) }}
      onMouseLeave={() => {
        cancelClose()
        closeTimer.current = window.setTimeout(() => { closeTimer.current = null; setOpen(false) }, 150)
      }}>
      <button type="button" style={summaryStyle}
        onClick={(e) => { e.preventDefault(); e.stopPropagation(); setOpen((v) => !v) }}
        className={`cursor-pointer transition-opacity hover:opacity-75 ${summaryClassName}`}>
        {summary}
      </button>
      {open && (
        <div
          onClick={(e) => { e.preventDefault(); e.stopPropagation() }}
          className="absolute left-0 top-full z-50 flex w-max max-w-[380px] flex-wrap gap-1.5 rounded-lg border bg-card p-2 pt-3 shadow-md">
          {/* 指向 chip 的小箭头:旋转方块的上半角,边框/底色与弹层一致,盖住弹层顶边框形成折角 */}
          <span className="pointer-events-none absolute -top-[5px] left-3 h-2 w-2 rotate-45 rounded-[2px] border-l border-t bg-card" />
          {children(() => { cancelClose(); setOpen(false) })}
        </div>
      )}
    </div>
  )
}
