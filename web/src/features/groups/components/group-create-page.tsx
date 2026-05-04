import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { createGroup } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ArrowLeft, Loader2 } from 'lucide-react'

export function GroupCreatePage() {
  const navigate = useNavigate()
  const [form, setForm] = useState({
    name: '',
    display_name: '',
    description: '',
    endpoint_slug: '',
    expose_mode: 'direct' as 'direct' | 'smart',
  })

  const createMutation = useMutation({
    mutationFn: () => createGroup({
      name: form.name,
      display_name: form.display_name || undefined,
      description: form.description || undefined,
      endpoint_slug: form.endpoint_slug || form.name,
      expose_mode: form.expose_mode,
    }),
    onSuccess: (res) => {
      toast.success('分组创建成功')
      const id = res.data?.id
      navigate({ to: '/groups/$id', params: { id: String(id) } })
    },
  })

  return (
    <div className="p-6 lg:p-8 max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/groups' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-xl font-semibold">创建分组</h1>
      </div>

      <div className="space-y-4 rounded-xl border bg-card p-6">
        <div className="space-y-2">
          <Label htmlFor="name">分组标识 *</Label>
          <Input id="name" placeholder="my-group" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <p className="text-xs text-muted-foreground">唯一标识，创建后不可修改</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="display_name">显示名称</Label>
          <Input id="display_name" placeholder="我的分组" value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="endpoint_slug">端点 Slug</Label>
          <Input id="endpoint_slug" placeholder="my-group（留空则使用分组标识）" value={form.endpoint_slug} onChange={(e) => setForm({ ...form, endpoint_slug: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="description">描述</Label>
          <Input id="description" placeholder="分组用途说明" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label>暴露模式</Label>
          <div className="grid gap-3 sm:grid-cols-2">
            <button
              type="button"
              onClick={() => setForm({ ...form, expose_mode: 'direct' })}
              className={`rounded-lg border p-4 text-left transition-all ${
                form.expose_mode === 'direct' ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
              }`}
            >
              <p className="text-sm font-semibold">Direct 模式</p>
              <p className="mt-1 text-xs text-muted-foreground">直接暴露所有工具，适合工具少的场景</p>
            </button>
            <button
              type="button"
              onClick={() => setForm({ ...form, expose_mode: 'smart' })}
              className={`rounded-lg border p-4 text-left transition-all ${
                form.expose_mode === 'smart' ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
              }`}
            >
              <p className="text-sm font-semibold">Smart 模式</p>
              <p className="mt-1 text-xs text-muted-foreground">仅暴露 3 个元工具（搜索/查看/执行），适合工具多的场景</p>
            </button>
          </div>
        </div>
      </div>

      <div className="flex justify-end">
        <Button
          className="gap-2"
          onClick={() => createMutation.mutate()}
          disabled={!form.name.trim() || createMutation.isPending}
        >
          {createMutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
          创建分组
        </Button>
      </div>
    </div>
  )
}
