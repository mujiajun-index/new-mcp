import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
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
import { Plus, Trash2, Video, Eye, Loader2, CirclePower, Phone } from 'lucide-react'
import { toast } from 'sonner'
import { useState } from 'react'

export function CameraListPage() {
  const queryClient = useQueryClient()
  const [deletingId, setDeletingId] = useState<number | null>(null)

  // 视频页跳转：携带 id + 当前会话 token，可在手机/平板等任意终端打开
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
      toast.success('摄像头已删除')
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
      setDeletingId(null)
    },
  })

  const toggleMutation = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: 'enable' | 'disable' }) => {
      return action === 'enable' ? enableCamera(id) : disableCamera(id)
    },
    onSuccess: (_data, variables) => {
      toast.success(variables.action === 'enable' ? '摄像头已启用' : '摄像头已禁用')
      queryClient.invalidateQueries({ queryKey: ['cameras'] })
    },
  })

  const cameras: CameraListItem[] = data?.data || []

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">摄像头管理</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理摄像头设备与视频流配置</p>
        </div>
        <Link to="/cameras/create">
          <Button className="gap-2"><Plus className="h-4 w-4" />新建摄像头</Button>
        </Link>
      </div>

      <div className="rounded-xl border bg-card overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">加载中...</div>
        ) : cameras.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Video className="h-10 w-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">暂无摄像头</p>
            <p className="text-xs text-muted-foreground/60 mt-1">点击"新建摄像头"添加第一个设备</p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">名称</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">关联视觉配置</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">状态</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">流状态</th>
                <th className="px-4 py-3 text-right font-medium text-muted-foreground">操作</th>
              </tr>
            </thead>
            <tbody>
              {cameras.map((camera) => (
                <tr key={camera.id} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-3 font-medium">{camera.name}</td>
                  <td className="px-4 py-3 text-muted-foreground">{camera.vision_config_name || '-'}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex items-center gap-1.5 text-sm ${
                      camera.auto_register ? 'text-emerald-600 dark:text-emerald-400' : 'text-zinc-500'
                    }`}>
                      <span className={`h-2 w-2 rounded-full ${camera.auto_register ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                      {camera.auto_register ? '已启用' : '已禁用'}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span className="inline-flex items-center gap-1.5">
                      <span className={`h-2 w-2 rounded-full ${camera.streaming ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                      <span className="text-sm">{camera.streaming ? '推流中' : '未推流'}</span>
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Link to="/cameras/$id" params={{ id: String(camera.id) }}>
                        <Button variant="ghost" size="sm" className="gap-1">
                          <Eye className="h-3.5 w-3.5" />详情
                        </Button>
                      </Link>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="gap-1"
                        onClick={() => openLive(camera.id)}
                        title="在新标签打开视频页（手机/桌面自适应，可复制链接到其他终端）"
                      >
                        <Phone className="h-3.5 w-3.5" />视频
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
                        {camera.auto_register ? '禁用' : '启用'}
                      </Button>
                      <AlertDialog open={deletingId === camera.id} onOpenChange={(open) => !open && setDeletingId(null)}>
                        <AlertDialogTrigger asChild>
                          <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setDeletingId(camera.id)}>
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>确认删除</AlertDialogTitle>
                            <AlertDialogDescription>
                              确定要删除摄像头 "{camera.name}" 吗？此操作不可撤销。
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>取消</AlertDialogCancel>
                            <AlertDialogAction
                              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                              onClick={() => deleteMutation.mutate(camera.id)}
                            >
                              删除
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
        )}
      </div>
    </div>
  )
}
