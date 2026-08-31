import { useQuery, useMutation } from '@tanstack/react-query'
import { useNavigate, useParams, useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { getMarketplaceItem, addToMyServices } from '../api'
import { useTagColors, tagStyle } from '../tag-colors'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel } from '@/lib/billing'
import { Button } from '@/components/ui/button'
import { SectionCard } from '@/components/section-card'
import { ToolItem } from '@/components/tool-params'
import { ResourceItemCard, PromptItemCard } from '@/components/mcp-items'
import { toast } from 'sonner'
import { ArrowLeft, Download, Star, Zap, ExternalLink, Plus } from 'lucide-react'
import type { McpTool, McpResource, McpResourceTemplate, McpPrompt, MarketplaceEntryPrice } from '@/types'

export function MarketplaceDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const router = useRouter()
  const { id } = useParams({ strict: false }) as { id: string }
  const { config } = useSystemConfigStore()

  const { data, isLoading } = useQuery({
    queryKey: ['marketplace-item', id],
    queryFn: () => getMarketplaceItem(Number(id)),
  })
  const tagColors = useTagColors()

  const installMutation = useMutation({
    mutationFn: () => addToMyServices(Number(id)),
    onSuccess: (res) => {
      toast.success(t('marketplace.addSuccess', { name: res.data?.name }))
      navigate({ to: '/services' })
    },
  })

  const item = data?.data
  const priceText = priceLabel(item?.billing_type ?? '', item?.price_per_call ?? 0, config.displayCurrency)

  // 快照数据(与服务详情页同一展示形态;旧市场项无快照时为空)
  const tools: McpTool[] = item?.tools_snapshot || []
  const resources: McpResource[] = item?.resources_snapshot?.resources || []
  const templates: McpResourceTemplate[] = item?.resources_snapshot?.templates || []
  const prompts: McpPrompt[] = item?.prompts_snapshot || []

  // 条目级价格:命中 free/per_call → 高亮徽标;未命中或显式继承(inherit)→ 弱化显示
  // 回退价(工具/显式继承条目=服务统一价,资源/提示缺省=免费)
  const entryMap = new Map(
    ((item?.entry_prices || []) as MarketplaceEntryPrice[]).map((p) => [`${p.kind}:${p.name}`, p] as const),
  )
  const entryPriceBadge = (kind: 'tool' | 'resource' | 'prompt', name: string, fallback: string) => {
    const e = entryMap.get(`${kind}:${name}`)
    if (e && e.billing_type !== 'inherit') {
      return (
        <span className={`shrink-0 rounded px-1.5 py-0.5 text-xs font-medium ${
          e.billing_type === 'free' ? 'bg-muted text-muted-foreground' : 'bg-primary/10 text-primary'
        }`}>
          {priceLabel(e.billing_type, e.price_per_call, config.displayCurrency)}
        </span>
      )
    }
    return (
      <span className="shrink-0 text-xs text-muted-foreground" title={t('marketplace.servicePriceHint')}>
        {e ? priceText : fallback}
      </span>
    )
  }

  if (isLoading) return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('common.loading')}</div>
  if (!item) return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('marketplace.notFound')}</div>

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <Button variant="ghost" size="sm" className="gap-1.5" onClick={() => {
        // 真回退:从服务详情等入口进详情回到来路;新标签直开无历史时兜底回广场
        if (router.history.canGoBack()) router.history.back()
        else navigate({ to: '/marketplace' })
      }}>
        <ArrowLeft className="h-4 w-4" />{t('common.back')}
      </Button>

      {/* Header */}
      <div className="flex items-start gap-4">
        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 text-primary shrink-0">
          {item.icon_url ? (
            <img src={item.icon_url} alt="" className="h-8 w-8" />
          ) : (
            <span className="text-2xl font-bold">{(item.display_name || item.name).charAt(0)}</span>
          )}
        </div>
        <div className="min-w-0 flex-1">
          {/* 名称行与广场卡片同布局:部署形态标识、版本号紧跟名称 */}
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-xl font-semibold">{item.display_name || item.name}</h1>
            <span className={`shrink-0 rounded px-1.5 py-0.5 text-xs font-medium ${
              item.category === 'instant' ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
            }`}>
              {item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}
            </span>
            {item.version && <span className="shrink-0 text-xs text-muted-foreground">v{item.version}</span>}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1">
            {item.group_names?.map((name: string) => (
              <span key={name} className="rounded bg-blue-500/10 px-2 py-0.5 text-xs font-medium text-blue-600 dark:text-blue-400">{name}</span>
            ))}
            {/* 标签与元信息同排(与列表卡片同序:分组 → 标签 → 元信息);原先沉在工具/资源/提示列表之后 */}
            {item.tags?.map((tag: string) => (
              <span key={tag} style={tagStyle(tagColors[tag])}
                className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${tagColors[tag] ? '' : 'bg-muted text-muted-foreground'}`}>
                {tag}
              </span>
            ))}
            <span className="flex items-center gap-1 text-xs text-muted-foreground">
              <Download className="h-3 w-3" />{t('marketplace.installs', { count: item.install_count })}
            </span>
            {item.rating_count > 0 && (
              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                <Star className="h-3 w-3" />{item.rating_avg.toFixed(1)} ({item.rating_count})
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Description */}
      {item.description && (
        <div className="rounded-xl border bg-card p-5">
          <p className="text-sm leading-relaxed">{item.description}</p>
        </div>
      )}

      {/* Install action */}
      <div className="rounded-xl border bg-card p-5">
        {item.category === 'instant' ? (
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <p className="text-lg font-semibold text-primary">{priceText}</p>
                <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary">
                  {t('marketplace.platformHosted')}
                </span>
              </div>
              <p className="text-xs text-muted-foreground">{t('marketplace.platformHostedDesc')}</p>
            </div>
            <Button className="gap-2" onClick={() => installMutation.mutate()} disabled={installMutation.isPending}>
              {installMutation.isPending ? <Zap className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
              {installMutation.isPending ? t('marketplace.installing') : t('marketplace.addToMyServices')}
            </Button>
          </div>
        ) : (
          <div className="space-y-4">
            <div>
              <p className="text-sm font-medium">{t('marketplace.deployGuide')}</p>
              <p className="text-xs text-muted-foreground">{t('marketplace.deployGuideDesc')}</p>
            </div>
            {item.repo_url && (
              <a href={item.repo_url} target="_blank" rel="noopener noreferrer">
                <Button variant="outline" className="gap-2">
                  <ExternalLink className="h-4 w-4" />{t('marketplace.viewSourceRepo')}
                </Button>
              </a>
            )}
            {item.install_guide && (
              <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto whitespace-pre-wrap">{item.install_guide}</pre>
            )}
            {item.required_env && item.required_env.length > 0 && (
              <div>
                <p className="text-xs font-medium mb-1.5">{t('marketplace.envVarsRequired')}</p>
                <div className="flex flex-wrap gap-1.5">
                  {item.required_env.map((env: string) => (
                    <span key={env} className="rounded bg-muted px-2 py-0.5 text-xs font-mono">{env}</span>
                  ))}
                </div>
              </div>
            )}
            <Button variant="outline" onClick={() => navigate({ to: '/services/create' })}>
              {t('marketplace.goRegister')}
            </Button>
          </div>
        )}
      </div>

      {/* Tools snapshot(条目右侧显示各自价格:条目价高亮,未设则弱化显示服务统一价) */}
      <SectionCard title={t('marketplace.toolsProvided', { count: tools.length })}>
        {tools.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noTools')}</p>
        ) : (
          <div className="space-y-2">
            {tools.map((tool) => (
              <ToolItem key={tool.name} name={tool.name} description={tool.description} schema={tool.inputSchema}
                action={entryPriceBadge('tool', tool.name, priceText)} />
            ))}
          </div>
        )}
      </SectionCard>

      {/* Resources snapshot(资源缺省免费,可显式继承服务价;模板不可定价) */}
      <SectionCard title={t('marketplace.resourcesProvided', { count: resources.length + templates.length })}>
        {resources.length === 0 && templates.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noResources')}</p>
        ) : (
          <div className="space-y-2">
            {resources.map((r) => (
              <ResourceItemCard key={r.uri} name={r.name} uri={r.uri} description={r.description} mimeType={r.mimeType}
                action={entryPriceBadge('resource', r.uri, t('billing.free'))} />
            ))}
            {templates.map((tpl) => (
              <ResourceItemCard key={tpl.uriTemplate} name={tpl.name} uri={tpl.uriTemplate} description={tpl.description} mimeType={tpl.mimeType} isTemplate />
            ))}
          </div>
        )}
      </SectionCard>

      {/* Prompts snapshot(提示缺省免费,可显式继承服务价) */}
      <SectionCard title={t('marketplace.promptsProvided', { count: prompts.length })}>
        {prompts.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">{t('services.noPrompts')}</p>
        ) : (
          <div className="space-y-2">
            {prompts.map((p) => (
              <PromptItemCard key={p.name} name={p.name} description={p.description} args={p.arguments}
                action={entryPriceBadge('prompt', p.name, t('billing.free'))} />
            ))}
          </div>
        )}
      </SectionCard>
    </div>
  )
}
