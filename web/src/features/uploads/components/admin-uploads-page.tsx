import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { adminGetUploads, adminDeleteUpload, adminBatchDeleteUploads, type UploadListItem } from '../api'
import { formatBytes, formatRelative, formatDateTime } from '../utils'
import { BackendBadge, Thumb, CopyUrlButton, ImagePreviewDialog } from './shared'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Image as ImageIcon, Trash2, Loader2, RefreshCw, Search } from 'lucide-react'
import { toast } from 'sonner'

export function AdminUploadsPage() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [page, setPage] = useState(1)
  const [userInput, setUserInput] = useState('')
  const [userId, setUserId] = useState<number | undefined>(undefined)
  const [previewIndex, setPreviewIndex] = useState<number | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const pageSize = 15
  const locale = i18n.language?.startsWith('zh') ? 'zh-CN' : 'en-US'

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['admin-uploads', userId, page],
    queryFn: () => adminGetUploads({ user_id: userId, page, page_size: pageSize }),
  })

  const deleteMutation = useMutation({
    mutationFn: adminDeleteUpload,
    onSuccess: () => {
      toast.success(t('uploads.deleteSuccess'))
      queryClient.invalidateQueries({ queryKey: ['admin-uploads'] })
    },
  })

  const batchDeleteMutation = useMutation({
    mutationFn: (ids: number[]) => adminBatchDeleteUploads(ids),
    onSuccess: (data) => {
      if (data.failed > 0) {
        toast.warning(t('uploads.batchDeletePartial', { deleted: data.deleted, failed: data.failed }))
      } else {
        toast.success(t('uploads.batchDeleteSuccess', { count: data.deleted }))
      }
      setSelected(new Set())
      queryClient.invalidateQueries({ queryKey: ['admin-uploads'] })
    },
  })

  const items: UploadListItem[] = data?.data || []
  const pagination = data?.pagination
  const totalPages = pagination?.total_pages ?? 1
  const total = pagination?.total ?? 0

  // Reset selection when the view changes (page / user filter), so stale ids from
  // a previous page don't linger.
  useEffect(() => {
    setSelected(new Set())
  }, [page, userId])

  const allSelected = items.length > 0 && items.every((i) => selected.has(i.id))
  const toggleItem = (id: number, checked: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  const toggleAll = () =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (allSelected) items.forEach((i) => next.delete(i.id))
      else items.forEach((i) => next.add(i.id))
      return next
    })

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    const n = parseInt(userInput, 10)
    setUserId(Number.isNaN(n) || n <= 0 ? undefined : n)
    setPage(1)
  }

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('uploads.adminTitle')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('uploads.adminSubtitle')}</p>
        </div>
        <div className="flex items-center gap-2">
          {selected.size > 0 && (
            <Button
              variant="destructive"
              size="sm"
              className="gap-1.5"
              disabled={batchDeleteMutation.isPending}
              onClick={() => {
                if (confirm(t('uploads.batchDeleteConfirm', { count: selected.size }))) {
                  batchDeleteMutation.mutate(Array.from(selected))
                }
              }}
            >
              <Trash2 className="h-4 w-4" />
              {t('uploads.batchDelete')} ({selected.size})
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={() => queryClient.invalidateQueries({ queryKey: ['admin-uploads'] })}
          >
            <RefreshCw className={isFetching ? 'h-4 w-4 animate-spin' : 'h-4 w-4'} />
            {t('common.refresh')}
          </Button>
        </div>
      </div>

      {/* User filter */}
      <form onSubmit={handleSearch} className="flex items-center gap-2">
        <div className="relative max-w-xs flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="number"
            min="1"
            placeholder={t('uploads.filterByUser')}
            value={userInput}
            onChange={(e) => setUserInput(e.target.value)}
            className="pl-9"
          />
        </div>
        <Button type="submit" variant="outline" size="sm">
          {t('common.search')}
        </Button>
        {userId !== undefined && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              setUserInput('')
              setUserId(undefined)
              setPage(1)
            }}
          >
            {t('common.clear')}
          </Button>
        )}
      </form>

      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t('common.loading')}
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <ImageIcon className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('uploads.empty')}</p>
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {items.map((item, idx) => (
              <MobileListCard
                key={item.id}
                title={
                  <div className="flex items-center gap-2.5">
                    <Checkbox
                      checked={selected.has(item.id)}
                      onCheckedChange={(c) => toggleItem(item.id, c === true)}
                      aria-label={t('common.select')}
                    />
                    <Thumb src={item.url} alt={item.key} onClick={() => setPreviewIndex(idx)} />
                    <span className="font-mono text-xs text-muted-foreground">{item.mime || '-'}</span>
                  </div>
                }
                badge={<BackendBadge backend={item.backend} />}
                meta={[
                  { label: t('uploads.user'), value: `#${item.user_id}` },
                  { label: t('uploads.size'), value: formatBytes(item.size) },
                  { label: t('uploads.created'), value: formatRelative(item.created_at, t) },
                ]}
                actions={
                  <>
                    <CopyUrlButton url={item.url} t={t} />
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive"
                      disabled={deleteMutation.isPending}
                      onClick={() => {
                        if (confirm(t('uploads.adminDeleteConfirm'))) deleteMutation.mutate(item.id)
                      }}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </>
                }
              />
            ))}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-2 py-3">
                    <div className="flex items-center">
                      <Checkbox
                        checked={allSelected}
                        onCheckedChange={() => toggleAll()}
                        aria-label={t('uploads.selectAll')}
                      />
                    </div>
                  </th>
                  <th className="px-2 py-3 text-left font-medium text-muted-foreground whitespace-nowrap">{t('uploads.preview')}</th>
                  <th className="px-3 py-3 text-left font-medium text-muted-foreground whitespace-nowrap">{t('uploads.user')}</th>
                  <th className="px-3 py-3 text-left font-medium text-muted-foreground whitespace-nowrap">{t('uploads.type')}</th>
                  <th className="px-3 py-3 text-left font-medium text-muted-foreground whitespace-nowrap">{t('uploads.size')}</th>
                  <th className="px-3 py-3 text-left font-medium text-muted-foreground whitespace-nowrap">{t('uploads.backend')}</th>
                  <th className="px-3 py-3 text-left font-medium text-muted-foreground whitespace-nowrap">{t('uploads.storageKey')}</th>
                  <th className="px-3 py-3 text-left font-medium text-muted-foreground whitespace-nowrap">{t('uploads.created')}</th>
                  <th className="px-3 py-3 text-left font-medium text-muted-foreground whitespace-nowrap">{t('uploads.expires')}</th>
                  <th className="px-3 py-3 text-right font-medium text-muted-foreground whitespace-nowrap">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item, idx) => (
                  <tr key={item.id} className="border-b last:border-0 hover:bg-muted/30 transition-colors">
                    <td className="px-2 py-3">
                      <div className="flex items-center">
                        <Checkbox
                          checked={selected.has(item.id)}
                          onCheckedChange={(c) => toggleItem(item.id, c === true)}
                          aria-label={t('common.select')}
                        />
                      </div>
                    </td>
                    <td className="px-2 py-3">
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span className="inline-flex">
                              <Thumb src={item.url} alt={item.key} onClick={() => setPreviewIndex(idx)} />
                            </span>
                          </TooltipTrigger>
                          <TooltipContent className="font-mono text-xs">
                            <div>{t('uploads.shortId')}: {item.short_id}</div>
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </td>
                    <td className="px-3 py-3 whitespace-nowrap">
                      <span className="font-mono text-xs">#{item.user_id}</span>
                    </td>
                    <td className="max-w-[120px] px-3 py-3">
                      <span className="block truncate font-mono text-xs text-muted-foreground" title={item.mime || '-'}>
                        {item.mime || '-'}
                      </span>
                    </td>
                    <td className="px-3 py-3 tabular-nums text-muted-foreground whitespace-nowrap">{formatBytes(item.size)}</td>
                    <td className="px-3 py-3">
                      <BackendBadge backend={item.backend} />
                    </td>
                    <td className="max-w-[200px] px-3 py-3">
                      <span className="block truncate font-mono text-xs text-muted-foreground" title={item.key}>
                        {item.key}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-xs text-muted-foreground whitespace-nowrap">
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span>{formatRelative(item.created_at, t)}</span>
                          </TooltipTrigger>
                          <TooltipContent>{formatDateTime(item.created_at, locale)}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </td>
                    <td className="px-3 py-3 text-xs text-muted-foreground whitespace-nowrap">
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span>{formatRelative(item.expires_at, t)}</span>
                          </TooltipTrigger>
                          <TooltipContent>{formatDateTime(item.expires_at, locale)}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </td>
                    <td className="px-3 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <CopyUrlButton url={item.url} t={t} iconOnly />
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          disabled={deleteMutation.isPending}
                          onClick={() => {
                            if (confirm(t('uploads.adminDeleteConfirm'))) deleteMutation.mutate(item.id)
                          }}
                          title={t('common.delete')}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {pagination && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">
            {t('common.total')} {total} {t('common.items')}
          </p>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
              ‹
            </Button>
            <span className="text-sm tabular-nums">
              {page} / {totalPages}
            </span>
            <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
              ›
            </Button>
          </div>
        </div>
      )}

      <ImagePreviewDialog
        items={items}
        index={previewIndex ?? 0}
        open={previewIndex !== null}
        onIndexChange={setPreviewIndex}
        onOpenChange={(o) => {
          if (!o) setPreviewIndex(null)
        }}
      />
    </div>
  )
}
