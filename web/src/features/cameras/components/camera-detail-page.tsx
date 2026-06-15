import { useState, useEffect } from 'react'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getCamera, updateCamera, deleteCamera, enableCamera, disableCamera } from '../api'
import { getVisionConfigs } from '@/features/vision/api'
import type { VisionConfigListItem } from '@/features/vision/api'
import { CameraCapture } from './camera-capture'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { toast } from 'sonner'
import {
  ArrowLeft, Trash2, Pencil, X, Check, Loader2,
  CirclePower, Camera, Wrench,
} from 'lucide-react'

export function CameraDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()
  const cameraId = Number(id)

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({
    name: '',
    description: '',
    vision_config_id: '' as string,
    capture_name: '',
    capture_desc: '',
    analyze_name: '',
    analyze_desc: '',
  })
  const [streaming, setStreaming] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['cameras', id],
    queryFn: () => getCamera(cameraId),
  })

  const { data: visionData } = useQuery({
    queryKey: ['vision'],
    queryFn: () => getVisionConfigs(),
  })

  const camera = data?.data
  const visionConfigs: VisionConfigListItem[] = (visionData?.data || []).filter(
    (v: VisionConfigListItem) => v.status === 1
  )

  useEffect(() => {
    if (camera && !editing) {
      setForm({
        name: camera.name || '',
        description: camera.description || '',
        vision_config_id: camera.vision_config_id ? String(camera.vision_config_id) : '',
        capture_name: camera.capture_name || '',
        capture_desc: camera.capture_desc || '',
        analyze_name: camera.analyze_name || '',
        analyze_desc: camera.analyze_desc || '',
      })
      setStreaming(camera.streaming)
    }
  }, [camera, editing])

  const updateMutation = useMutation({
    mutationFn: () =>
      updateCamera(cameraId, {
        name: form.name,
        description: form.description,
        vision_config_id: form.vision_config_id ? Number(form.vision_config_id) : undefined,
        capture_name: form.capture_name,
        capture_desc: form.capture_desc,
        analyze_name: form.analyze_name,
        analyze_desc: form.analyze_desc,
      }),
    onSuccess: () => {
      toast.success('更新成功')
      setEditing(false)
      queryClient.invalidateQueries({ queryKey: ['cameras', id] })
    },
    onError: () => {
      toast.error('更新失败')
    },
  })

  const toggleMutation = useMutation({
    mutationFn: (action: 'enable' | 'disable') =>
      action === 'enable' ? enableCamera(cameraId) : disableCamera(cameraId),
    onSuccess: () => {
      toast.success('状态已更新')
      queryClient.invalidateQueries({ queryKey: ['cameras', id] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteCamera(cameraId),
    onSuccess: () => {
      toast.success('摄像头已删除')
      navigate({ to: '/cameras' })
    },
  })

  if (isLoading) return <div className="flex items-center justify-center py-20 text-muted-foreground">加载中...</div>
  if (!camera) return <div className="flex items-center justify-center py-20 text-muted-foreground">摄像头不存在</div>

  return (
    <div className="p-6 lg:p-8 space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/cameras' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{camera.name}</h1>
            <p className={`mt-0.5 text-sm font-medium flex items-center gap-1.5 ${
              camera.auto_register ? 'text-emerald-600 dark:text-emerald-400' : 'text-zinc-500'
            }`}>
              <span className={`h-2 w-2 rounded-full ${camera.auto_register ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
              {camera.auto_register ? '已启用' : '已禁用'}
            </p>
          </div>
        </div>
        <div className="flex gap-2">
          {!editing && (
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              <Pencil className="h-4 w-4 mr-1.5" />编辑
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={() => toggleMutation.mutate(camera.auto_register ? 'disable' : 'enable')}
            disabled={toggleMutation.isPending}
          >
            {toggleMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin mr-1.5" />
            ) : (
              <CirclePower className="h-4 w-4 mr-1.5" />
            )}
            {camera.auto_register ? '禁用' : '启用'}
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="outline" size="sm" className="text-destructive">
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
                  onClick={() => deleteMutation.mutate()}
                >
                  删除
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>

      {/* Basic config */}
      <div className="rounded-xl border bg-card p-5 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-muted-foreground">基本配置</h2>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          {/* Name */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">名称</Label>
            {editing ? (
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            ) : (
              <p className="text-sm">{camera.name}</p>
            )}
          </div>

          {/* Vision config */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">视觉配置</Label>
            {editing ? (
              <Select
                value={form.vision_config_id}
                onValueChange={(value) => setForm({ ...form, vision_config_id: value })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="选择视觉配置" />
                </SelectTrigger>
                <SelectContent>
                  {visionConfigs.map((vc) => (
                    <SelectItem key={vc.id} value={String(vc.id)}>
                      {vc.name} ({vc.model_name})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <p className="text-sm">{camera.vision_config_name || '-'}</p>
            )}
          </div>

          {/* Description */}
          <div className="space-y-1.5 sm:col-span-2">
            <Label className="text-xs text-muted-foreground">描述</Label>
            {editing ? (
              <Input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} placeholder="摄像头描述" />
            ) : (
              <p className="text-sm">{camera.description || '-'}</p>
            )}
          </div>
        </div>

        {/* Edit actions */}
        {editing && (
          <div className="flex justify-end gap-2 pt-2 border-t">
            <Button variant="outline" size="sm" onClick={() => setEditing(false)}>
              <X className="h-4 w-4 mr-1.5" />取消
            </Button>
            <Button
              size="sm"
              onClick={() => updateMutation.mutate()}
              disabled={!form.name.trim() || !form.vision_config_id || updateMutation.isPending}
            >
              {updateMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin mr-1.5" /> : <Check className="h-4 w-4 mr-1.5" />}
              保存
            </Button>
          </div>
        )}
      </div>

      {/* Tool config */}
      <div className="rounded-xl border bg-card p-5 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-muted-foreground flex items-center gap-1.5">
            <Wrench className="h-3.5 w-3.5" />
            工具配置
          </h2>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          {/* Capture tool card */}
          <div className="rounded-lg border p-4 space-y-3">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-100 dark:bg-blue-900/30">
                <Camera className="h-4 w-4 text-blue-600 dark:text-blue-400" />
              </div>
              <div>
                <p className="text-sm font-medium">截图工具</p>
                <p className="text-xs text-muted-foreground">capture</p>
              </div>
            </div>
            <div className="space-y-2">
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">工具名称</Label>
                {editing ? (
                  <Input
                    value={form.capture_name}
                    onChange={(e) => setForm({ ...form, capture_name: e.target.value })}
                    placeholder="截图工具名称"
                  />
                ) : (
                  <p className="text-sm">{camera.capture_name || '-'}</p>
                )}
              </div>
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">工具描述</Label>
                {editing ? (
                  <Input
                    value={form.capture_desc}
                    onChange={(e) => setForm({ ...form, capture_desc: e.target.value })}
                    placeholder="截图工具描述"
                  />
                ) : (
                  <p className="text-sm">{camera.capture_desc || '-'}</p>
                )}
              </div>
            </div>
          </div>

          {/* Analyze tool card */}
          <div className="rounded-lg border p-4 space-y-3">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-purple-100 dark:bg-purple-900/30">
                <Wrench className="h-4 w-4 text-purple-600 dark:text-purple-400" />
              </div>
              <div>
                <p className="text-sm font-medium">分析工具</p>
                <p className="text-xs text-muted-foreground">analyze</p>
              </div>
            </div>
            <div className="space-y-2">
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">工具名称</Label>
                {editing ? (
                  <Input
                    value={form.analyze_name}
                    onChange={(e) => setForm({ ...form, analyze_name: e.target.value })}
                    placeholder="分析工具名称"
                  />
                ) : (
                  <p className="text-sm">{camera.analyze_name || '-'}</p>
                )}
              </div>
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">工具描述</Label>
                {editing ? (
                  <Input
                    value={form.analyze_desc}
                    onChange={(e) => setForm({ ...form, analyze_desc: e.target.value })}
                    placeholder="分析工具描述"
                  />
                ) : (
                  <p className="text-sm">{camera.analyze_desc || '-'}</p>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Camera preview */}
      <div className="rounded-xl border bg-card p-5 space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-muted-foreground">摄像头预览</h2>
          <span className="inline-flex items-center gap-1.5 text-xs">
            <span className={`h-2 w-2 rounded-full ${streaming ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
            {streaming ? '推流中' : '未推流'}
          </span>
        </div>
        <CameraCapture cameraId={cameraId} onStreamingChange={setStreaming} />
      </div>
    </div>
  )
}
