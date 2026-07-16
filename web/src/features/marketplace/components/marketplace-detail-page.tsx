import { useQuery, useMutation } from '@tanstack/react-query'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { getMarketplaceItem, addToMyServices } from '../api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel } from '@/lib/billing'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'
import { ArrowLeft, Download, Star, Zap, ExternalLink, Plus } from 'lucide-react'

export function MarketplaceDetailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const { config } = useSystemConfigStore()

  const { data, isLoading } = useQuery({
    queryKey: ['marketplace-item', id],
    queryFn: () => getMarketplaceItem(Number(id)),
  })

  const installMutation = useMutation({
    mutationFn: () => addToMyServices(Number(id)),
    onSuccess: (res) => {
      toast.success(t('marketplace.addSuccess', { name: res.data?.name }))
      navigate({ to: '/services' })
    },
  })

  const item = data?.data
  const priceText = priceLabel(item?.billing_type ?? '', item?.price_per_call ?? 0, config.displayCurrency)

  if (isLoading) return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('common.loading')}</div>
  if (!item) return <div className="flex items-center justify-center py-20 text-muted-foreground">{t('marketplace.notFound')}</div>

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <Button variant="ghost" size="sm" className="gap-1.5" onClick={() => navigate({ to: '/marketplace' })}>
        <ArrowLeft className="h-4 w-4" />{t('marketplace.backToMarketplace')}
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
        <div className="flex-1">
          <h1 className="text-xl font-semibold">{item.display_name || item.name}</h1>
          <div className="mt-1 flex items-center gap-3">
            <span className={`rounded px-2 py-0.5 text-xs font-medium ${
              item.category === 'instant' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-amber-500/10 text-amber-600'
            }`}>
              {item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}
            </span>
            {item.version && <span className="text-xs text-muted-foreground">v{item.version}</span>}
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

      {/* Tools snapshot */}
      {item.tools_snapshot && item.tools_snapshot.length > 0 && (
        <div className="rounded-xl border bg-card p-5">
          <h2 className="mb-3 text-sm font-semibold">{t('marketplace.toolsProvided', { count: item.tools_snapshot.length })}</h2>
          <div className="space-y-2">
            {item.tools_snapshot.map((tool: { name: string; description?: string }) => (
              <div key={tool.name} className="rounded-lg border p-3">
                <p className="text-sm font-medium font-mono">{tool.name}</p>
                {tool.description && <p className="mt-0.5 text-xs text-muted-foreground">{tool.description}</p>}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Tags */}
      {item.tags && item.tags.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {item.tags.map((tag: string) => (
            <span key={tag} className="rounded-full bg-muted px-3 py-1 text-xs">{tag}</span>
          ))}
        </div>
      )}
    </div>
  )
}
