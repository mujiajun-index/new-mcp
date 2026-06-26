import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getCameras, deleteCamera, enableCamera, disableCamera } from '../api'
import type { CameraListItem } from '../api'
import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { Plus, Trash2, Video, Eye, Loader2, CirclePower, Phone, MoreHorizontal } from 'lucide-react'
import { toast } from 'sonner'
import { useState } from 'react'

export function CameraListPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [deletingId, setDeletingId] = useState<number | null>(null)

  // Video page navigation: carry id + current session token, opens on any device (phone/tablet/desktop)
  const streamToken = localStorage.getItem('newmcp-token') || ''
  const streamBase = (import.meta.env.BASE_URL ?? '/').replace(/\/$/, '')
  const openLive = (cameraId: number) => {
    const href = `${streamBase}/camera-live/${cameraId}?token=${encodeURIComponent(streamToken)}`
    window.open(href, '_blank', 'noopener,noreferrer')
  }

  const { data, isLoading } = useQuery({
    queryKey: ['cameras'],
    queryFn: () => getCameras(),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteCamera,
    onSuccess: () => {
      toast.success(t('cameras.detail.deleted'))
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
      setDeletingId(null)
    },
  })

  const toggleMutation = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: 'enable' | 'disable' }) => {
      return action === 'enable' ? enableCamera(id) : disableCamera(id)
    },
    onSuccess: (_data, variables) => {
      toast.success(variables.action === 'enable' ? t('cameras.statusEnabled') : t('cameras.statusDisabled'))
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
    },
  })

  const cameras: CameraListItem[] = data?.data || []

  const enabledBadge = (camera: CameraListItem) => (
    <span
      className={`inline-flex items-center gap-1.5 text-sm ${
        camera.auto_register ? 'text-emerald-600 dark:text-emerald-400' : 'text-zinc-500'
      }`}
    >
      <span className={`h-2 w-2 rounded-full ${camera.auto_register ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
      {camera.auto_register ? t('cameras.statusEnabled') : t('cameras.statusDisabled')}
    </span>
  )

  const streamingValue = (camera: CameraListItem) => (
    <span className="inline-flex items-center gap-1.5">
      <span className={`h-2 w-2 rounded-full ${camera.streaming ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
      <span className="text-sm">{camera.streaming ? t('cameras.streamStreaming') : t('cameras.streamIdle')}</span>
    </span>
  )

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('cameras.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('cameras.subtitle')}</p>
        </div>
        <Link to="/cameras/create">
          <Button className="gap-2"><Plus className="h-4 w-4" />{t('cameras.create.title')}</Button>
        </Link>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
        ) : cameras.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Video className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{t('cameras.noCameras')}</p>
            <p className="mt-1 text-xs text-muted-foreground/60">{t('cameras.noCamerasHint')}</p>
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {cameras.map((camera) => (
              <MobileListCard
                key={camera.id}
                title={camera.name}
                badge={enabledBadge(camera)}
                meta={[
                  { label: t('cameras.visionConfig'), value: camera.vision_config_name || '-' },
                  { label: t('cameras.streamStatus'), value: streamingValue(camera) },
                ]}
                actions={
                  <>
                    <Link to="/cameras/$id" params={{ id: String(camera.id) }}>
                      <Button variant="ghost" size="sm" className="gap-1">
                        <Eye className="h-3.5 w-3.5" />{t('cameras.detail.title')}
                      </Button>
                    </Link>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="gap-1"
                      disabled={!camera.auto_register}
                      onClick={() => openLive(camera.id)}
                      title={camera.auto_register ? t('cameras.openVideoTabTitle') : t('cameras.enableFirstTitle')}
                    >
                      <Phone className="h-3.5 w-3.5" />{t('cameras.video')}
                    </Button>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8">
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          disabled={toggleMutation.isPending}
                          onClick={() =>
                            toggleMutation.mutate({
                              id: camera.id,
                              action: camera.auto_register ? 'disable' : 'enable',
                            })
                          }
                        >
                          <CirclePower className="mr-2 h-4 w-4" />
                          {camera.auto_register ? t('cameras.disable') : t('cameras.enable')}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          className="text-destructive focus:text-destructive"
                          onClick={() => setDeletingId(camera.id)}
                        >
                          <Trash2 className="mr-2 h-4 w-4" />
                          {t('common.delete')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                    <AlertDialog open={deletingId === camera.id} onOpenChange={(open) => !open && setDeletingId(null)}>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>{t('cameras.deleteConfirmTitle')}</AlertDialogTitle>
                          <AlertDialogDescription>
                            {t('cameras.deleteConfirmDesc', { name: camera.name })}
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
                          <AlertDialogAction
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                            onClick={() => deleteMutation.mutate(camera.id)}
                          >
                            {t('common.delete')}
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
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
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('cameras.table.name')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('cameras.table.visionConfig')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('cameras.table.status')}</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">{t('cameras.table.streamStatus')}</th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">{t('cameras.table.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {cameras.map((camera) => (
                  <tr key={camera.id} className="border-b last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-3 font-medium">{camera.name}</td>
                    <td className="px-4 py-3 text-muted-foreground">{camera.vision_config_name || '-'}</td>
                    <td className="px-4 py-3">{enabledBadge(camera)}</td>
                    <td className="px-4 py-3">{streamingValue(camera)}</td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Link to="/cameras/$id" params={{ id: String(camera.id) }}>
                          <Button variant="ghost" size="sm" className="gap-1">
                            <Eye className="h-3.5 w-3.5" />{t('cameras.detail.title')}
                          </Button>
                        </Link>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="gap-1"
                          disabled={!camera.auto_register}
                          onClick={() => openLive(camera.id)}
                          title={camera.auto_register ? t('cameras.openVideoTabTitle') : t('cameras.enableFirstTitle')}
                        >
                          <Phone className="h-3.5 w-3.5" />{t('cameras.video')}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="gap-1"
                          onClick={() => toggleMutation.mutate({
                            id: camera.id,
                            action: camera.auto_register ? 'disable' : 'enable',
                          })}
                          disabled={toggleMutation.isPending}
                        >
                          {toggleMutation.isPending ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <CirclePower className="h-3.5 w-3.5" />
                          )}
                          {camera.auto_register ? t('cameras.disable') : t('cameras.enable')}
                        </Button>
                        <AlertDialog open={deletingId === camera.id} onOpenChange={(open) => !open && setDeletingId(null)}>
                          <AlertDialogTrigger asChild>
                            <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setDeletingId(camera.id)}>
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>{t('cameras.deleteConfirmTitle')}</AlertDialogTitle>
                              <AlertDialogDescription>
                                {t('cameras.deleteConfirmDesc', { name: camera.name })}
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
                              <AlertDialogAction
                                className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                                onClick={() => deleteMutation.mutate(camera.id)}
                              >
                                {t('common.delete')}
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}