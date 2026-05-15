import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation } from '@tanstack/react-query'
import { createCamera } from '../api'
import { getVisionConfigs } from '@/features/vision/api'
import type { VisionConfigListItem } from '@/features/vision/api'
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
import { toast } from 'sonner'
import { ArrowLeft, Loader2, AlertCircle } from 'lucide-react'

export function CameraCreatePage() {
  const navigate = useNavigate()
  const [form, setForm] = useState({
    name: '',
    description: '',
    vision_config_id: '' as string,
  })

  const { data: visionData, isLoading: visionLoading } = useQuery({
    queryKey: ['vision'],
    queryFn: () => getVisionConfigs(),
  })

  const visionConfigs: VisionConfigListItem[] = (visionData?.data || []).filter(
    (v: VisionConfigListItem) => v.status === 1
  )

  const canSubmit =
    form.name.trim() !== '' &&
    form.vision_config_id !== ''

  const createMutation = useMutation({
    mutationFn: () =>
      createCamera({
        name: form.name,
        description: form.description || undefined,
        vision_config_id: Number(form.vision_config_id),
      }),
    onSuccess: () => {
      toast.success('摄像头创建成功')
      navigate({ to: '/cameras' })
    },
  })

  return (
    <div className="p-6 lg:p-8 max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/cameras' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-xl font-semibold">新建摄像头</h1>
      </div>

      <div className="space-y-4 rounded-xl border bg-card p-6">
        <div className="space-y-2">
          <Label htmlFor="name">名称 *</Label>
          <Input
            id="name"
            placeholder="摄像头名称"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="description">描述</Label>
          <Input
            id="description"
            placeholder="摄像头描述（可选）"
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
          />
        </div>

        <div className="space-y-2">
          <Label>视觉配置 *</Label>
          {visionLoading ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground py-2">
              <Loader2 className="h-4 w-4 animate-spin" />加载中...
            </div>
          ) : visionConfigs.length === 0 ? (
            <div className="flex items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>暂无可用的视觉配置，请先创建并启用视觉配置。</span>
            </div>
          ) : (
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
          )}
        </div>
      </div>

      <div className="flex justify-end">
        <Button
          onClick={() => createMutation.mutate()}
          disabled={!canSubmit || createMutation.isPending || visionConfigs.length === 0}
        >
          {createMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          创建摄像头
        </Button>
      </div>
    </div>
  )
}
