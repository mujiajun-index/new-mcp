import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getMarketplaceItems, getMarketplaceGroups } from '../api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel } from '@/lib/billing'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import type { MarketplaceListItem } from '@/types'
import { Search, Store, Download, Star, Zap, Code2, FolderTree } from 'lucide-react'

export function MarketplaceListPage() {
  const { t } = useTranslation()
  const { config } = useSystemConfigStore()
  const [keyword, setKeyword] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [category, setCategory] = useState<'' | 'instant' | 'source'>('')
  const [groupId, setGroupId] = useState<number | ''>('')
  const [groupsOpen, setGroupsOpen] = useState(false)

  const { data: groupsData } = useQuery({ queryKey: ['marketplace-groups'], queryFn: getMarketplaceGroups })
  const groups: any[] = groupsData?.data ?? []

  const { data, isLoading } = useQuery({
    queryKey: ['marketplace', keyword, category, groupId],
    queryFn: () => getMarketplaceItems({
      keyword: keyword || undefined,
      category: category || undefined,
      group_id: groupId || undefined,
    }),
  })

  const items: MarketplaceListItem[] = data?.data || []

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
              onClick={() => setGroupId('')}
              className={`flex w-full items-center rounded-lg px-3 py-2 text-sm transition-colors ${groupId === '' ? 'bg-primary/10 font-medium text-primary' : 'text-sidebar-foreground/70 hover:bg-muted'}`}
            >
              {t('marketplace.filterAll')}
            </button>
            {groups.map((g) => (
              <button key={g.id} onClick={() => setGroupId(g.id)}
                className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors ${groupId === g.id ? 'bg-primary/10 font-medium text-primary' : 'text-sidebar-foreground/70 hover:bg-muted'}`}>
                {g.icon_url ? <img src={g.icon_url} alt="" className="h-4 w-4" /> : <FolderTree className="h-3.5 w-3.5 shrink-0" />}
                <span className="truncate">{g.display_name || g.name}</span>
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
          <form onSubmit={(e) => { e.preventDefault(); setKeyword(searchInput) }} className="relative max-w-sm flex-1">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input placeholder={t('marketplace.searchPlaceholder')} value={searchInput} onChange={(e) => setSearchInput(e.target.value)} className="pl-9" />
          </form>
          <Button variant={groupsOpen ? 'default' : 'outline'} size="sm" className="gap-1.5 shrink-0" onClick={() => setGroupsOpen((v) => !v)}>
            <FolderTree className="h-3.5 w-3.5" />{t('categories.groups')}
          </Button>
          <div className="flex flex-wrap gap-2">
            <Button variant={category === '' ? 'default' : 'outline'} size="sm" onClick={() => setCategory('')}>{t('marketplace.filterAll')}</Button>
            <Button variant={category === 'instant' ? 'default' : 'outline'} size="sm" className="gap-1.5" onClick={() => setCategory('instant')}>
              <Zap className="h-3.5 w-3.5" />{t('marketplace.filterReady')}
            </Button>
            <Button variant={category === 'source' ? 'default' : 'outline'} size="sm" className="gap-1.5" onClick={() => setCategory('source')}>
              <Code2 className="h-3.5 w-3.5" />{t('marketplace.filterSource')}
            </Button>
        </div>
        </div>

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
            {items.map((item) => (
              <Link key={item.id} to="/marketplace/$id" params={{ id: String(item.id) }}>
                <div className="group flex h-full flex-col rounded-xl border bg-card p-5 transition-all hover:border-primary/20 hover:shadow-md hover:shadow-black/[0.03]">
                  <div className="flex items-start gap-3">
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                      {item.icon_url ? <img src={item.icon_url} alt="" className="h-6 w-6" /> : <span className="text-lg font-bold">{(item.display_name || item.name).charAt(0)}</span>}
                    </div>
                    <div className="min-w-0 flex-1">
                      <h3 className="truncate font-semibold transition-colors group-hover:text-primary">{item.display_name || item.name}</h3>
                      <div className="mt-0.5 flex flex-wrap items-center gap-2">
                        {item.group_name && <span className="rounded bg-blue-500/10 px-1.5 py-0.5 text-[10px] font-medium text-blue-600 dark:text-blue-400">{item.group_name}</span>}
                        <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${item.category === 'instant' ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'}`}>
                          {item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}
                        </span>
                        {item.version && <span className="text-[10px] text-muted-foreground">v{item.version}</span>}
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
                  {item.tags?.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {item.tags.slice(0, 2).map((tag) => <span key={tag} className="rounded bg-muted px-1.5 py-0.5 text-[10px]">{tag}</span>)}
                    </div>
                  )}
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
