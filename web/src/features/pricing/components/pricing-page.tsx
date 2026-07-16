import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getMarketplaceItems } from '@/features/marketplace/api'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { priceLabel, currencySymbol } from '@/lib/billing'
import { Input } from '@/components/ui/input'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { useIsMobile } from '@/hooks/use-mobile'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { Search, Store, Info } from 'lucide-react'

export function PricingPage() {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const { config } = useSystemConfigStore()
  const [keyword, setKeyword] = useState('')
  const [searchInput, setSearchInput] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['pricing', keyword],
    queryFn: () => getMarketplaceItems({ keyword: keyword || undefined }),
  })

  const items = data?.data ?? []

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('pricing.title')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('pricing.subtitle')}</p>
      </div>

      {/* Status note */}
      <div className="flex items-start gap-2 rounded-xl border bg-card p-4 text-sm">
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        <div className="space-y-1">
          {!config.billingEnabled ? (
            <p className="text-muted-foreground">{t('pricing.billingOff')}</p>
          ) : (
            <>
              <p className="text-muted-foreground">{t('pricing.defaultDesc')}</p>
              <p className="text-muted-foreground">
                {config.selfUseModeEnabled ? t('pricing.selfUseNote') : t('pricing.commercialNote')}
              </p>
            </>
          )}
        </div>
      </div>

      {/* Search */}
      <form
        onSubmit={(e) => { e.preventDefault(); setKeyword(searchInput) }}
        className="relative max-w-sm flex-1"
      >
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder={t('common.search')}
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          className="pl-9"
        />
      </form>

      {/* Price list */}
      <div className="rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Store className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('pricing.empty')}</p>
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {items.map((item: any) => (
              <MobileListCard
                key={item.id}
                title={<span className="font-medium">{item.display_name || item.name}</span>}
                meta={[
                  { label: t('pricing.colPrice'), value: priceLabel(item.billing_type, item.price_per_call, config.displayCurrency) },
                  { label: t('pricing.colCategory'), value: item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source') },
                ]}
              />
            ))}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('pricing.colService')}</TableHead>
                <TableHead>{t('pricing.colCategory')}</TableHead>
                <TableHead>{t('pricing.colPrice')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item: any) => (
                <TableRow key={item.id}>
                  <TableCell className="font-medium">{item.display_name || item.name}</TableCell>
                  <TableCell>
                    <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${
                      item.category === 'instant'
                        ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                        : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
                    }`}>
                      {item.category === 'instant' ? t('marketplace.ready') : t('marketplace.source')}
                    </span>
                  </TableCell>
                  <TableCell className="text-sm font-medium">
                    {priceLabel(item.billing_type, item.price_per_call, config.displayCurrency)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      <p className="text-xs text-muted-foreground">
        {t('billing.perCall')} = {currencySymbol(config.displayCurrency)}?
      </p>
    </div>
  )
}
