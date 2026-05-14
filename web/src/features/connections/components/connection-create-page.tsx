import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation } from '@tanstack/react-query'
import { createConnection } from '../api'
import { getApiKeys } from '@/features/api-keys/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ArrowLeft, Loader2 } from 'lucide-react'

const cloudTypes = [
  { value: 'xiaozhi' as const, label: '小智', desc: '连接小智 AI 平台' },
  { value: 'custom' as const, label: '自定义 WSS', desc: '自定义 WebSocket 服务端' },
  { value: 'ssh' as const, label: 'SSH', desc: 'SSH 隧道连接' },
]

export function ConnectionCreatePage() {
  const navigate = useNavigate()
  const [form, setForm] = useState({
    name: '',
    cloud_type: 'xiaozhi' as 'xiaozhi' | 'custom' | 'ssh',
    wss_url: '',
    api_key_id: 0,
    auto_connect: true,
    expose_mode: 'smart' as 'smart' | 'direct',
    // SSH fields
    host: '',
    port: '22',
    username: '',
    password: '',
  })

  const { data: keysData } = useQuery({
    queryKey: ['api-keys'],
    queryFn: () => getApiKeys(),
  })

  const apiKeys = (keysData?.data || []).filter((k: { status: number }) => k.status === 1)

  const canSubmit =
    form.name.trim() !== '' &&
    form.api_key_id !== 0 &&
    (form.cloud_type === 'ssh'
      ? form.host.trim() !== ''
      : form.wss_url.trim() !== '')

  const createMutation = useMutation({
    mutationFn: () => {
      const config: Record<string, unknown> = {}
      if (form.cloud_type === 'ssh') {
        config.host = form.host
        config.port = Number(form.port)
        config.username = form.username
        config.password = form.password
      }
      return createConnection({
        name: form.name,
        cloud_type: form.cloud_type,
        wss_url: form.cloud_type !== 'ssh' ? form.wss_url : undefined,
        api_key_id: form.api_key_id,
        auto_connect: form.auto_connect,
        expose_mode: form.expose_mode,
        cloud_config: form.cloud_type === 'ssh' ? config : undefined,
      })
    },
    onSuccess: () => {
      toast.success('连接创建成功')
      navigate({ to: '/connections' })
    },
  })

  return (
    <div className="p-6 lg:p-8 max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/connections' })}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-xl font-semibold">添加云端连接</h1>
      </div>

      <div className="space-y-4 rounded-xl border bg-card p-6">
        <div className="space-y-2">
          <Label htmlFor="name">连接名称 *</Label>
          <Input id="name" placeholder="我的连接" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        </div>

        <div className="space-y-2">
          <Label>云平台类型</Label>
          <div className="grid gap-2 sm:grid-cols-3">
            {cloudTypes.map((ct) => (
              <button
                key={ct.value}
                type="button"
                onClick={() => setForm({ ...form, cloud_type: ct.value })}
                className={`rounded-lg border p-3 text-left transition-all ${
                  form.cloud_type === ct.value ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
                }`}
              >
                <p className="text-sm font-medium">{ct.label}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">{ct.desc}</p>
              </button>
            ))}
          </div>
        </div>

        {form.cloud_type !== 'ssh' ? (
          <div className="space-y-2">
            <Label htmlFor="wss_url">WSS URL *</Label>
            <Input id="wss_url" placeholder="wss://example.com/ws" value={form.wss_url} onChange={(e) => setForm({ ...form, wss_url: e.target.value })} />
          </div>
        ) : (
          <>
            <div className="space-y-2">
              <Label>主机 *</Label>
              <Input placeholder="192.168.1.1" value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label>端口</Label>
                <Input value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label>用户名</Label>
                <Input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} />
              </div>
            </div>
            <div className="space-y-2">
              <Label>密码</Label>
              <Input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
            </div>
          </>
        )}

        <div className="space-y-2">
          <Label>绑定 API Key *</Label>
          {apiKeys.length === 0 ? (
            <p className="text-xs text-muted-foreground">请先创建 API Key 后再添加连接</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {apiKeys.map((k: { id: number; name: string; key_prefix: string }) => (
                <button
                  key={k.id}
                  type="button"
                  onClick={() => setForm({ ...form, api_key_id: k.id })}
                  className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                    form.api_key_id === k.id ? 'border-primary bg-primary/5' : 'hover:border-primary/30'
                  }`}
                >
                  {k.name} <span className="text-muted-foreground">({k.key_prefix}...)</span>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="space-y-2">
          <Label>暴露模式</Label>
          <div className="grid gap-2 sm:grid-cols-2">
            <button
              type="button"
              onClick={() => setForm({ ...form, expose_mode: 'smart' })}
              className={`rounded-lg border p-3 text-left transition-all ${
                form.expose_mode === 'smart' ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
              }`}
            >
              <p className="text-sm font-medium">智能模式</p>
              <p className="mt-0.5 text-xs text-muted-foreground">3 个元工具智能搜索</p>
            </button>
            <button
              type="button"
              onClick={() => setForm({ ...form, expose_mode: 'direct' })}
              className={`rounded-lg border p-3 text-left transition-all ${
                form.expose_mode === 'direct' ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
              }`}
            >
              <p className="text-sm font-medium">直接模式</p>
              <p className="mt-0.5 text-xs text-muted-foreground">暴露所有工具</p>
            </button>
          </div>
        </div>
      </div>

      <div className="flex justify-end">
        <Button
          onClick={() => createMutation.mutate()}
          disabled={!canSubmit || createMutation.isPending}
        >
          {createMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          创建连接
        </Button>
      </div>
    </div>
  )
}
