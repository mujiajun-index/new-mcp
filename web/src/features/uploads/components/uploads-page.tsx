import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getUploads, deleteUpload, type UploadListItem } from '../api'
import { formatBytes, formatRelative, formatDateTime } from '../utils'
import { BackendBadge, Thumb, CopyUrlButton } from './shared'
import { Button } from '@/components/ui/button'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  Image as ImageIcon,
  Trash2,
  Loader2,
  RefreshCw,
  AlertTriangle,
} from 'lucide-react'
import { toast } from 'sonner'

export function UploadsPage() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [page, setPage] = useState(1)
  const pageSize = 12
  const locale = i18n.language?.startsWith('zh') ? 'zh-CN' : 'en-US'

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['uploads', page],
    queryFn: () => getUploads({ page, page_size: pageSize }),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteUpload,
    onSuccess: () => {
      toast.success(t('uploads.deleteSuccess'))
      queryClient.invalidateQueries({ queryKey: ['uploads'] })
    },
  })

  const items: UploadListItem[] = data?.data || []
  const pagination = data?.pagination
  const totalPages = pagination?.total_pages ?? 1
  const total = pagination?.total ?? 0

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('uploads.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('uploads.subtitle')}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="gap-1.5"
          onClick={() => queryClient.invalidateQueries({ queryKey: ['uploads'] })}
        >
          <RefreshCw className={isFetching ? 'h-4 w-4 animate-spin' : 'h-4 w-4'} />
          {t('common.refresh')}
        </Button>
      </div>

      <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-xs text-amber-700 dark:text-amber-400">
        <div className="flex items-start gap-2">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{t('uploads.retentionHint')}</span>
        </div>
      </div>

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
            <p className="mt-1 text-xs text-muted-foreground/60">{t('uploads.emptyHint')}</p>
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {items.map((item) => (
              <MobileListCard
                key={item.id}
                title={
                  <div className="flex items-center gap-2.5">
                    <Thumb src={item.url} alt={item.key} />
                    <span className="font-mono text-xs text-muted-foreground">{item.mime || '-'}</span>
                  </div>
                }
                badge={<BackendBadge backend={item.backend} />}
                meta={[
                  { label: t('uploads.size'), value: formatBytes(item.size) },
                  {
                    label: t('uploads.created'),
                    value: formatRelative(item.created_at, t),
                  },
                  {
                    label: t('uploads.expires'),
                    value: (
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span>{formatRelative(item.expires_at, t)}</span>
                          </TooltipTrigger>
                          <TooltipContent>{formatDateTime(item.expires_at, locale)}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    ),
                  },
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
                        if (confirm(t('uploads.deleteConfirm'))) deleteMutation.mutate(item.id)
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
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('uploads.preview')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('uploads.type')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('uploads.size')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('uploads.backend')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('uploads.created')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('uploads.expires')}</th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id} className="border-b last:border-0 hover:bg-muted/30 transition-colors">
                    <td className="px-4 py-3">
                      <Thumb src={item.url} alt={item.key} />
                    </td>
                    <td className="px-4 py-3">
                      <span className="font-mono text-xs text-muted-foreground">{item.mime || '-'}</span>
                    </td>
                    <td className="px-4 py-3 tabular-nums text-muted-foreground">{formatBytes(item.size)}</td>
                    <td className="px-4 py-3">
                      <BackendBadge backend={item.backend} />
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span>{formatRelative(item.created_at, t)}</span>
                          </TooltipTrigger>
                          <TooltipContent>{formatDateTime(item.created_at, locale)}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span>{formatRelative(item.expires_at, t)}</span>
                          </TooltipTrigger>
                          <TooltipContent>{formatDateTime(item.expires_at, locale)}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <CopyUrlButton url={item.url} t={t} />
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          disabled={deleteMutation.isPending}
                          onClick={() => {
                            if (confirm(t('uploads.deleteConfirm'))) deleteMutation.mutate(item.id)
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
    </div>
  )
}
