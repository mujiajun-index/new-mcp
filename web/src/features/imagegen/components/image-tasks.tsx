import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  Copy,
  Download,
  ImagePlus,
  Loader2,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  ContentBlock,
  copyText,
} from '@/features/services/components/test-result-panel'
import {
  deleteImageTask,
  getImageTask,
  imageDownloadURL,
  listImageTasks,
} from '../api'
import type { ImageTask } from '../api'

function TaskStatus({ task }: { task: ImageTask }) {
  const { t } = useTranslation()
  return (
    <Badge
      className="whitespace-nowrap"
      variant={
        task.status === 'completed'
          ? 'success'
          : ['failed', 'canceled', 'interrupted'].includes(task.status)
            ? 'destructive'
            : 'secondary'
      }
    >
      {task.status === 'running' && (
        <Loader2 className="mr-1 h-3 w-3 animate-spin" />
      )}
      {t(`imagegen.taskStatus.${task.status}`)}
    </Badge>
  )
}

function ImageTaskDetail({ id }: { id: string }) {
  const { t } = useTranslation()
  const { data, isError } = useQuery({
    queryKey: ['image-task', id],
    queryFn: () => getImageTask(id),
    refetchInterval: (query) =>
      query.state.data?.data.status === 'running' ? 5000 : false,
  })
  if (isError)
    return (
      <p className="text-sm text-destructive">{t('imagegen.taskLoadFailed')}</p>
    )
  if (!data) return <Loader2 className="h-5 w-5 animate-spin" />
  const task = data.data
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-2">
        <span className="min-w-0 break-all font-mono text-sm">
          {task.model}
        </span>
        <TaskStatus task={task} />
        <Badge variant="outline">
          {t(`imagegen.operations.${task.operation}`)}
        </Badge>
      </div>
      <dl className="grid grid-cols-1 gap-3 text-xs text-muted-foreground sm:grid-cols-2">
        <div>
          <dt>{t('imagegen.taskConfig')}</dt>
          <dd className="mt-1 break-words text-foreground">
            {task.config_name || t('imagegen.draftConfig')}
          </dd>
        </div>
        <div>
          <dt>{t('imagegen.taskSource')}</dt>
          <dd className="mt-1 text-foreground">
            {t(`imagegen.taskSources.${task.source}`)}
          </dd>
        </div>
        <div>
          <dt>{t('common.createdAt')}</dt>
          <dd className="mt-1 text-foreground">
            {new Date(task.created_at).toLocaleString()}
          </dd>
        </div>
        <div>
          <dt>{t('imagegen.taskDuration')}</dt>
          <dd className="mt-1 text-foreground">
            {task.finished_at
              ? `${(task.duration_ms / 1000).toFixed(1)}s`
              : '—'}
          </dd>
        </div>
        {task.operation === 'edit' && (
          <div>
            <dt>{t('imagegen.referenceCount')}</dt>
            <dd className="mt-1 text-foreground">{task.reference_count}</dd>
          </div>
        )}
      </dl>
      <div className="space-y-2">
        <h3 className="text-sm font-medium">{t('imagegen.prompt')}</h3>
        <p className="max-h-60 overflow-y-auto whitespace-pre-wrap break-words rounded-lg bg-muted/50 p-3 text-sm [overflow-wrap:anywhere]">
          {task.prompt || '—'}
        </p>
      </div>
      {task.error && (
        <p className="whitespace-pre-wrap break-words rounded-lg bg-destructive/10 p-3 text-xs text-destructive">
          {task.error}
        </p>
      )}
      {task.warnings
        .filter((warning) => !task.error?.includes(warning))
        .map((warning, i) => (
          <p
            key={i}
            className="break-words rounded-lg bg-muted p-3 text-xs text-muted-foreground"
          >
            {warning}
          </p>
        ))}
      {task.text && (
        <p className="whitespace-pre-wrap break-words text-sm">{task.text}</p>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        {task.images.map((image, i) => (
          <div key={i} className="min-w-0 space-y-2 rounded-lg border p-3">
            {image.url ? (
              <ContentBlock
                block={{
                  type: 'resource_link',
                  uri: image.url,
                  name: `generated-image-${i + 1}`,
                  mimeType: image.mime,
                }}
                index={i}
                downloadUri={imageDownloadURL(image.url)}
              />
            ) : (
              <div className="flex h-32 items-center justify-center gap-2 rounded-md bg-muted text-sm text-muted-foreground">
                <ImagePlus className="h-5 w-5" />
                {t('imagegen.imageExpired')}
              </div>
            )}
            <p className="text-xs text-muted-foreground">
              {image.width} × {image.height} · {image.mime} ·{' '}
              {(image.size / 1024 / 1024).toFixed(2)} MB
            </p>
            <p className="text-xs text-muted-foreground">
              {image.status === 'expired'
                ? t('imagegen.imageExpired')
                : image.expires_at
                  ? t('imagegen.imageExpires', {
                      date: new Date(image.expires_at).toLocaleString(),
                    })
                  : t('imagegen.permanent')}
            </p>
            {image.revised_prompt && (
              <p className="whitespace-pre-wrap break-words text-xs text-muted-foreground">
                {t('imagegen.revisedPrompt')}: {image.revised_prompt}
              </p>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

export function ImageTasks({
  configID,
  pending = false,
}: {
  configID?: number
  pending?: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const { data, isLoading, isError, isFetching, refetch } = useQuery({
    queryKey: ['image-tasks', configID, page],
    queryFn: () => listImageTasks(configID, page),
    refetchInterval: (query) =>
      pending ||
      query.state.data?.data.items.some((task) => task.status === 'running')
        ? 5000
        : false,
  })
  const remove = useMutation({
    mutationFn: deleteImageTask,
    onSuccess: (_, id) => {
      toast.success(t('imagegen.taskDeleted'))
      if (selectedID === id) setSelectedID(null)
      queryClient.removeQueries({ queryKey: ['image-task', id] })
      queryClient.invalidateQueries({ queryKey: ['image-tasks'] })
      if (page > 1 && data?.data.items.length === 1) setPage(page - 1)
    },
    onError: (err) => toast.error(err.message),
  })
  const total = data?.data.total ?? 0
  const pages = Math.max(1, Math.ceil(total / (data?.data.page_size ?? 20)))
  return (
    <section className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{t('imagegen.history')}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('imagegen.historyHint')}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={isFetching}
          onClick={() => refetch()}
          className="shrink-0 gap-2"
        >
          <RefreshCw
            className={`h-3.5 w-3.5 ${isFetching ? 'animate-spin' : ''}`}
          />
          {t('common.refresh')}
        </Button>
      </div>
      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-5 w-5 animate-spin" />
            {t('common.loading')}
          </div>
        ) : isError ? (
          <p className="p-8 text-center text-sm text-destructive">
            {t('imagegen.taskLoadFailed')}
          </p>
        ) : !data?.data.items.length ? (
          <p className="p-12 text-center text-sm text-muted-foreground">
            {t('imagegen.noTasks')}
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table
              className="w-full text-sm"
              aria-label={t('imagegen.history')}
            >
              <thead>
                <tr className="border-b bg-muted/50">
                  {[
                    'imagegen.task',
                    'imagegen.capabilities',
                    'imagegen.prompt',
                    'common.status',
                    'imagegen.taskImages',
                    'common.createdAt',
                    'imagegen.taskDuration',
                  ].map((label) => (
                    <th
                      key={label}
                      scope="col"
                      className="whitespace-nowrap px-4 py-3 text-left font-medium text-muted-foreground"
                    >
                      {t(label)}
                    </th>
                  ))}
                  <th
                    scope="col"
                    className="px-4 py-3 text-right font-medium text-muted-foreground"
                  >
                    {t('common.actions')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.data.items.map((task) => {
                  const ready = task.images.filter((image) => image.url)
                  return (
                    <tr
                      key={task.task_id}
                      className="border-b transition-colors last:border-0 hover:bg-muted/30"
                    >
                      <td className="px-4 py-3">
                        <div className="min-w-40 max-w-52 space-y-1">
                          <span className="block break-all font-mono text-xs">
                            {task.model}
                          </span>
                          {configID === undefined && (
                            <p
                              className="truncate text-xs text-muted-foreground"
                              title={task.config_name}
                            >
                              {task.config_name || t('imagegen.draftConfig')}
                            </p>
                          )}
                          <div className="flex items-center gap-1">
                            <code
                              className="truncate text-[11px] text-muted-foreground"
                              title={task.task_id}
                            >
                              {task.task_id}
                            </code>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-6 w-6 shrink-0"
                              aria-label={t('imagegen.copyTask')}
                              onClick={() =>
                                copyText(task.task_id)
                                  .then(() => toast.success(t('common.copied')))
                                  .catch(() =>
                                    toast.error(t('common.copyFailed'))
                                  )
                              }
                            >
                              <Copy className="h-3 w-3" />
                            </Button>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs">
                        <span className="whitespace-nowrap">
                          {t(`imagegen.operations.${task.operation}`)}
                        </span>
                        <p className="mt-1 text-muted-foreground">
                          {t(`imagegen.taskSources.${task.source}`)}
                        </p>
                      </td>
                      <td className="px-4 py-3">
                        <p
                          className="line-clamp-2 min-w-32 max-w-60 whitespace-pre-wrap break-words text-xs"
                          title={task.prompt}
                        >
                          {task.prompt}
                        </p>
                      </td>
                      <td className="px-4 py-3">
                        <TaskStatus task={task} />
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">
                        {task.images.length
                          ? t('imagegen.imageCount', {
                              ready: ready.length,
                              total: task.images.length,
                            })
                          : '—'}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-xs text-muted-foreground">
                        {new Date(task.created_at).toLocaleString()}
                      </td>
                      <td className="px-4 py-3 text-xs text-muted-foreground">
                        {task.finished_at
                          ? `${(task.duration_ms / 1000).toFixed(1)}s`
                          : '—'}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1 whitespace-nowrap">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setSelectedID(task.task_id)}
                          >
                            {t('imagegen.detail')}
                          </Button>
                          {ready[0]?.url && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8"
                              asChild
                            >
                              <a
                                href={imageDownloadURL(ready[0].url)}
                                download
                                aria-label={t('imagegen.downloadImage')}
                                title={t('imagegen.downloadImage')}
                              >
                                <Download className="h-4 w-4" />
                              </a>
                            </Button>
                          )}
                          {task.status !== 'running' && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-destructive hover:text-destructive"
                              disabled={remove.isPending}
                              aria-label={t('imagegen.deleteTask')}
                              title={t('imagegen.deleteTask')}
                              onClick={() => {
                                if (confirm(t('imagegen.deleteTaskConfirm')))
                                  remove.mutate(task.task_id)
                              }}
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {total > 0 && (
        <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
          <span>{t('imagegen.taskPagination', { page, pages, total })}</span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              {t('imagegen.previous')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= pages}
              onClick={() => setPage(page + 1)}
            >
              {t('imagegen.next')}
            </Button>
          </div>
        </div>
      )}
      <Dialog
        open={selectedID !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedID(null)
        }}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t('imagegen.taskDetail')}</DialogTitle>
            <DialogDescription className="break-all">
              {selectedID}
            </DialogDescription>
          </DialogHeader>
          {selectedID && <ImageTaskDetail key={selectedID} id={selectedID} />}
        </DialogContent>
      </Dialog>
    </section>
  )
}
