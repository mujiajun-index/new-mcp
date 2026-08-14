import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Link2, Copy, RefreshCw, Loader2, Ban } from 'lucide-react'
import { getCameraStreamKey, generateCameraStreamKey, revokeCameraStreamKey } from '../api'
import type { CameraStreamKey } from '../api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { copyText } from '@/lib/copy'

interface CameraStreamLinkDialogProps {
  cameraId: number
  cameraName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  /** 生成/重生成成功后的回调,参数为最新的密钥信息(列表页用它直接打开推流页) */
  onGenerated?: (info: CameraStreamKey) => void
  /** 生成成功后立即关闭对话框并刷新列表（列表页"视频"按钮的首次生成流程用）；
   *  详情页不传，保留生成后的链接展示/复制/重生成界面 */
  closeOnGenerate?: boolean
}

const TTL_PRESETS = [
  { value: '3600', hours: 1 },
  { value: '86400', hours: 24 },
  { value: '604800', hours: 24 * 7 },
  { value: '2592000', hours: 24 * 30 },
  { value: '0', hours: 0 },
  { value: 'custom', hours: -1 },
] as const

/**
 * 推流链接管理对话框：生成/重新生成/撤销摄像头推流密钥，展示完整短链接供复制。
 * 未生成态选择有效期后生成；已生成态展示链接与到期时间，可重新生成（旧链接立即失效）或撤销。
 */
export function CameraStreamLinkDialog({ cameraId, cameraName, open, onOpenChange, onGenerated, closeOnGenerate }: CameraStreamLinkDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [ttl, setTtl] = useState<string>('0')
  const [customHours, setCustomHours] = useState('')
  const [confirmAction, setConfirmAction] = useState<'regenerate' | 'revoke' | null>(null)

  const { data: info, isLoading } = useQuery({
    queryKey: ['cameras', cameraId, 'stream-key'],
    queryFn: () => getCameraStreamKey(cameraId),
    enabled: open,
  })

  useEffect(() => {
    if (open) {
      setTtl('0')
      setCustomHours('')
      setConfirmAction(null)
    }
  }, [open, cameraId])

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['cameras'] })
  }

  const key = ['cameras', cameraId, 'stream-key'] as const

  const generateMutation = useMutation({
    mutationFn: (expiresIn: number) => generateCameraStreamKey(cameraId, expiresIn),
    onSuccess: (data) => {
      toast.success(t('cameras.streamLink.generated'))
      // 直接用响应更新缓存,弹框内容立即刷新(不依赖 invalidate 触发的 GET refetch)
      queryClient.setQueryData(key, data)
      invalidate()
      onGenerated?.(data)
      if (closeOnGenerate) {
        onOpenChange(false)
      }
    },
    onError: () => toast.error(t('cameras.streamLink.generateFailed')),
  })

  const revokeMutation = useMutation({
    mutationFn: () => revokeCameraStreamKey(cameraId),
    onSuccess: () => {
      toast.success(t('cameras.streamLink.revoked'))
      setConfirmAction(null)
      queryClient.setQueryData(key, { has_key: false, stream_key: '', stream_url: '', expires_at: '' })
      invalidate()
    },
    onError: () => toast.error(t('cameras.streamLink.revokeFailed')),
  })

  const resolveTtlSeconds = (): number | null => {
    if (ttl !== 'custom') return Number(ttl)
    const hours = Number(customHours)
    if (!Number.isFinite(hours) || hours <= 0) {
      toast.error(t('cameras.streamLink.invalidHours'))
      return null
    }
    return Math.round(hours * 3600)
  }

  const handleGenerate = () => {
    const seconds = resolveTtlSeconds()
    if (seconds === null) return
    generateMutation.mutate(seconds)
  }

  const handleRegenerate = () => {
    const seconds = resolveTtlSeconds()
    if (seconds === null) return
    setConfirmAction(null)
    generateMutation.mutate(seconds)
  }

  const formatExpiry = (info: { expires_at: string }) =>
    info.expires_at ? new Date(info.expires_at).toLocaleString() : t('cameras.streamLink.permanent')

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Link2 className="h-4 w-4" />
              {t('cameras.streamLink.title')}
            </DialogTitle>
            <DialogDescription>{t('cameras.streamLink.desc', { name: cameraName })}</DialogDescription>
          </DialogHeader>

          {isLoading ? (
            <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              {t('common.loading')}
            </div>
          ) : info?.has_key ? (
            <div className="space-y-4">
              <div className="space-y-2">
                <p className="text-sm font-medium">{t('cameras.streamLink.linkLabel')}</p>
                <div className="flex items-center gap-2">
                  <code className="min-w-0 flex-1 truncate rounded-md bg-muted px-3 py-2 text-xs">
                    {info.stream_url}
                  </code>
                  <Button variant="outline" size="sm" className="gap-1 shrink-0" onClick={() => copyText(info.stream_url)}>
                    <Copy className="h-3.5 w-3.5" />{t('common.copy')}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t('cameras.streamLink.expiresAt')}: {formatExpiry(info)}
                </p>
              </div>

              <div className="space-y-2 rounded-lg border p-3">
                <p className="text-xs text-muted-foreground">{t('cameras.streamLink.regenHint')}</p>
                <div className="flex items-center gap-2">
                  <TtlSelect ttl={ttl} setTtl={setTtl} customHours={customHours} setCustomHours={setCustomHours} />
                  <Button
                    variant="outline"
                    size="sm"
                    className="gap-1 shrink-0"
                    disabled={generateMutation.isPending}
                    onClick={() => setConfirmAction('regenerate')}
                  >
                    {generateMutation.isPending ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <RefreshCw className="h-3.5 w-3.5" />
                    )}
                    {t('cameras.streamLink.regenerate')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="gap-1 text-destructive shrink-0"
                    disabled={revokeMutation.isPending}
                    onClick={() => setConfirmAction('revoke')}
                  >
                    {revokeMutation.isPending ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Ban className="h-3.5 w-3.5" />
                    )}
                    {t('cameras.streamLink.revoke')}
                  </Button>
                </div>
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">{t('cameras.streamLink.notGenerated')}</p>
              <div className="flex items-center gap-2">
                <TtlSelect ttl={ttl} setTtl={setTtl} customHours={customHours} setCustomHours={setCustomHours} />
                <Button size="sm" className="gap-1 shrink-0" disabled={generateMutation.isPending} onClick={handleGenerate}>
                  {generateMutation.isPending ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Link2 className="h-3.5 w-3.5" />
                  )}
                  {t('cameras.streamLink.generate')}
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={confirmAction === 'regenerate'}
        onOpenChange={(o) => !o && setConfirmAction(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('cameras.streamLink.regenConfirmTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('cameras.streamLink.regenConfirmDesc')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={handleRegenerate}>{t('cameras.streamLink.regenerate')}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={confirmAction === 'revoke'}
        onOpenChange={(o) => !o && setConfirmAction(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('cameras.streamLink.revokeConfirmTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('cameras.streamLink.revokeConfirmDesc')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => revokeMutation.mutate()}
            >
              {t('cameras.streamLink.revoke')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function TtlSelect({
  ttl,
  setTtl,
  customHours,
  setCustomHours,
}: {
  ttl: string
  setTtl: (v: string) => void
  customHours: string
  setCustomHours: (v: string) => void
}) {
  const { t } = useTranslation()
  const preset = TTL_PRESETS.find((p) => p.value === ttl)
  return (
    <div className="flex min-w-0 flex-1 items-center gap-2">
      <Select value={ttl} onValueChange={setTtl}>
        <SelectTrigger className="w-full sm:w-[180px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="3600">{t('cameras.streamLink.ttl1h')}</SelectItem>
          <SelectItem value="86400">{t('cameras.streamLink.ttl1d')}</SelectItem>
          <SelectItem value="604800">{t('cameras.streamLink.ttl7d')}</SelectItem>
          <SelectItem value="2592000">{t('cameras.streamLink.ttl30d')}</SelectItem>
          <SelectItem value="0">{t('cameras.streamLink.ttlPermanent')}</SelectItem>
          <SelectItem value="custom">{t('cameras.streamLink.ttlCustom')}</SelectItem>
        </SelectContent>
      </Select>
      {ttl === 'custom' && (
        <Input
          type="number"
          min={1}
          value={customHours}
          onChange={(e) => setCustomHours(e.target.value)}
          placeholder={t('cameras.streamLink.customHoursPlaceholder')}
          className="w-28"
        />
      )}
      {preset && preset.hours > 0 && (
        <span className="hidden text-xs text-muted-foreground sm:inline">
          {t('cameras.streamLink.ttlHours', { hours: preset.hours })}
        </span>
      )}
    </div>
  )
}
