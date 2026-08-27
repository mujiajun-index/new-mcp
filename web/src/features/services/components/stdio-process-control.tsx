import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { controlServiceProcess } from '../api'
import type { ProcessControlAction } from '@/types'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger,
} from '@/components/ui/dialog'
import { Loader2, Play, Power, RotateCw, Square } from 'lucide-react'

// stdio 进程操作:电源图标按钮弹框,内含 启动/停止/重启。running 只控制各按钮的
// 可用态;操作成功后失效 services-overview / service-process 两个轮询键,立即回读
// 真实状态。失败提示由 axios 拦截器统一弹出,这里不重复 toast。
export function StdioProcessControl({
  serviceId, running,
}: {
  serviceId: number
  running: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)

  const controlMutation = useMutation({
    mutationFn: (action: ProcessControlAction) => controlServiceProcess(serviceId, action),
    onSuccess: (_res, action) => {
      const successKey: Record<ProcessControlAction, string> = {
        start: 'services.processStartSuccess',
        stop: 'services.processStopSuccess',
        restart: 'services.processRestartSuccess',
      }
      toast.success(t(successKey[action]))
      setOpen(false)
      queryClient.invalidateQueries({ queryKey: ['services-overview'] })
      queryClient.invalidateQueries({ queryKey: ['service-process'] })
    },
  })

  const actions: Array<{
    key: ProcessControlAction
    label: string
    icon: React.ComponentType<{ className?: string }>
    disabled: boolean
    danger?: boolean
  }> = [
    { key: 'start', label: t('services.processStart'), icon: Play, disabled: running },
    { key: 'stop', label: t('services.processStop'), icon: Square, disabled: !running, danger: true },
    { key: 'restart', label: t('services.processRestart'), icon: RotateCw, disabled: !running },
  ]

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!controlMutation.isPending) setOpen(v) }}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0 text-muted-foreground"
          title={t('services.processControl')}
          aria-label={t('services.processControl')}
        >
          <Power className="h-4 w-4" />
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>{t('services.processControl')}</DialogTitle>
          <DialogDescription>{t('services.processControlHint')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          {actions.map(({ key, label, icon: Icon, disabled, danger }) => (
            <Button
              key={key}
              variant="outline"
              className={`w-full justify-start gap-2 ${danger ? 'text-destructive hover:text-destructive' : ''}`}
              disabled={disabled || controlMutation.isPending}
              onClick={() => controlMutation.mutate(key)}
            >
              {controlMutation.isPending && controlMutation.variables === key
                ? <Loader2 className="h-4 w-4 animate-spin" />
                : <Icon className="h-4 w-4" />}
              {label}
            </Button>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
