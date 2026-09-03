import { useState, type ComponentProps, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Loader2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useIsMobile } from '@/hooks/use-mobile'
import { cn } from '@/lib/utils'

// 布局参考 reference/new-api 的 logs-filter-toolbar:主筛选区响应式网格 +
// 可折叠高级筛选(带激活数徽标)+ 底部统计/重置/搜索操作行;移动端收敛为
// 固定主筛选 + 底部抽屉承载全部筛选。筛选为草稿态,点击「搜索」才生效。

interface LogsFilterToolbarProps {
  primaryFilters: ReactNode
  advancedFilters?: ReactNode
  mobilePinnedFilters?: ReactNode
  mobileFilters?: ReactNode
  mobileFilterCount?: number
  stats?: ReactNode
  hasActiveFilters: boolean
  hasAdvancedActiveFilters?: boolean
  advancedFilterCount?: number
  searchLoading?: boolean
  onReset: () => void
  onSearch: () => void
  className?: string
}

interface LogsFilterFieldProps {
  children: ReactNode
  wide?: boolean
  className?: string
}

export function LogsFilterField(props: LogsFilterFieldProps) {
  return (
    <div
      className={cn('min-w-0', props.wide && 'sm:col-span-2', props.className)}
    >
      {props.children}
    </div>
  )
}

export function LogsFilterInput(props: ComponentProps<typeof Input>) {
  return (
    <Input
      {...props}
      autoComplete="off"
      className={cn('h-8 min-w-0 text-sm leading-5', props.className)}
    />
  )
}

export function LogsFilterToolbar(props: LogsFilterToolbarProps) {
  const { t } = useTranslation()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false)
  const [mobilePanelCollapsed, setMobilePanelCollapsed] = useState(false)
  const isMobile = useIsMobile()

  const hasAdvancedFilters = props.advancedFilters != null
  const activeAdvancedCount =
    props.advancedFilterCount ?? (props.hasAdvancedActiveFilters ? 1 : 0)
  const activeMobileFilterCount = props.mobileFilterCount ?? activeAdvancedCount

  const handleMobileReset = () => {
    props.onReset()
    setMobileFiltersOpen(false)
  }

  const handleMobileSearch = () => {
    props.onSearch()
    setMobileFiltersOpen(false)
  }

  const advancedToggle = hasAdvancedFilters ? (
    <Button
      type="button"
      variant="ghost"
      onClick={() => setAdvancedOpen(open => !open)}
      aria-expanded={advancedOpen}
      className={cn(
        'gap-1 px-2 text-muted-foreground hover:text-foreground',
        props.hasAdvancedActiveFilters && !advancedOpen && 'text-primary hover:text-primary'
      )}
    >
      {advancedOpen ? t('logs.collapse') : t('logs.expand')}
      {activeAdvancedCount > 0 && (
        <Badge className="ml-0.5 flex size-5 justify-center p-0 text-[10px]">
          {activeAdvancedCount}
        </Badge>
      )}
      <ChevronDown
        className={cn(
          'size-3.5 transition-transform duration-200',
          advancedOpen && 'rotate-180'
        )}
      />
    </Button>
  ) : null

  if (isMobile && props.mobilePinnedFilters != null) {
    return (
      <Sheet open={mobileFiltersOpen} onOpenChange={setMobileFiltersOpen}>
        <div className={cn('rounded-lg border bg-card/50 p-2.5', props.className)}>
          {!mobilePanelCollapsed && (
            <div className="grid gap-2">{props.mobilePinnedFilters}</div>
          )}

          <div className={cn('flex flex-col gap-2', !mobilePanelCollapsed && 'mt-2')}>
            {!mobilePanelCollapsed && props.stats}
            <div className="flex items-center justify-end gap-1.5">
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setMobilePanelCollapsed(collapsed => !collapsed)}
                aria-expanded={!mobilePanelCollapsed}
                aria-label={mobilePanelCollapsed ? t('logs.expand') : t('logs.collapse')}
                className="mr-auto size-7 text-muted-foreground hover:text-foreground"
              >
                <ChevronDown
                  className={cn(
                    'size-3.5 transition-transform duration-200',
                    !mobilePanelCollapsed && 'rotate-180'
                  )}
                />
              </Button>
              <SheetTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  className={cn(
                    'gap-1 px-2 text-muted-foreground hover:text-foreground',
                    activeMobileFilterCount > 0 && 'text-primary hover:text-primary'
                  )}
                >
                  {t('logs.filter')}
                  {activeMobileFilterCount > 0 && (
                    <Badge className="ml-0.5 flex size-5 justify-center p-0 text-[10px]">
                      {activeMobileFilterCount}
                    </Badge>
                  )}
                </Button>
              </SheetTrigger>
              <Button
                type="button"
                onClick={props.onSearch}
                disabled={props.searchLoading}
              >
                {props.searchLoading && <Loader2 className="animate-spin" />}
                {t('common.search')}
              </Button>
            </div>
          </div>
        </div>

        <SheetContent side="bottom" showClose={false} className="max-h-[85dvh] p-0">
          <div className="mx-auto flex h-full w-full max-w-md flex-1 flex-col overflow-hidden">
            <SheetHeader className="border-b border-border/70 px-4 py-3 text-left">
              <SheetTitle>{t('logs.filter')}</SheetTitle>
              <SheetDescription>{t('logs.filterDrawerDesc')}</SheetDescription>
            </SheetHeader>
            <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-4 py-3">
              {props.mobileFilters ?? (
                <>
                  {props.primaryFilters}
                  {props.advancedFilters}
                </>
              )}
            </div>
            <SheetFooter className="grid grid-cols-2 gap-2 border-t border-border/70 px-4 py-3">
              <Button
                type="button"
                variant="outline"
                onClick={handleMobileReset}
                disabled={!props.hasActiveFilters}
              >
                {t('logs.reset')}
              </Button>
              <Button
                type="button"
                onClick={handleMobileSearch}
                disabled={props.searchLoading}
              >
                {props.searchLoading && <Loader2 className="animate-spin" />}
                {t('common.search')}
              </Button>
            </SheetFooter>
          </div>
        </SheetContent>
      </Sheet>
    )
  }

  return (
    <div className={cn('rounded-lg border bg-card/50 p-2.5 sm:p-3', props.className)}>
      <div className="flex flex-wrap items-start gap-2">
        <div className="grid min-w-0 flex-1 grid-cols-1 gap-2 sm:grid-cols-[repeat(auto-fit,minmax(10rem,1fr))]">
          {props.primaryFilters}
        </div>
        {advancedToggle && (
          <div className="flex shrink-0 items-center justify-end">
            {advancedToggle}
          </div>
        )}
      </div>

      {advancedOpen && props.advancedFilters && (
        <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-[repeat(auto-fit,minmax(10rem,1fr))]">
          {props.advancedFilters}
        </div>
      )}

      <div className="mt-2 flex flex-wrap items-center gap-2">
        {props.stats}
        <div className="ml-auto flex flex-wrap items-center justify-end gap-1.5 sm:gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={props.onReset}
            disabled={!props.hasActiveFilters}
          >
            {t('logs.reset')}
          </Button>
          <Button
            type="button"
            onClick={props.onSearch}
            disabled={props.searchLoading}
          >
            {props.searchLoading && <Loader2 className="animate-spin" />}
            {t('common.search')}
          </Button>
        </div>
      </div>
    </div>
  )
}
