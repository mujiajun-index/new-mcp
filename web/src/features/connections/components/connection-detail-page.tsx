import { useState, useEffect } from 'react'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getConnection, deleteConnection, connectConnection, disconnectConnection, updateConnection } from '../api'
import { getApiKeys } from '@/features/api-keys/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ArrowLeft, Trash2, Wifi, WifiOff, Loader2, Pencil, X, Check } from 'lucide-react'

const statusColors: Record<string, string> = {
  connected: 'text-emerald-600 dark:text-emerald-400',
  disconnected: 'text-zinc-500',
  connecting: 'text-amber-600 dark:text-amber-400',
  error: 'text-red-600 dark:text-red-400',
}

const statusLabels: Record<string, string> = {
  connected: '已连接',
  disconnected: '已断开',
  connecting: '连接中',
  error: '错误',
}

const cloudTypeLabels: Record<string, string> = {
  xiaozhi: '小智', custom: '自定义 WSS', ssh: 'SSH',
}

export function ConnectionDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()
  const connId = Number(id)

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({ name: '', wss_url: '', api_key_id: 0 })

  const { data, isLoading } = useQuery({
    queryKey: ['connection', id],
    queryFn: () => getConnection(connId),
  })

  const { data: keysData } = useQuery({
    queryKey: ['api-keys'],
    queryFn: () => getApiKeys(),
  })

  const conn = data?.data
  const apiKeys = (keysData?.data || []).filter((k: { status: number }) => k.status === 1)

  useEffect(() => {
    if (conn && !editing) {
      setForm({
        name: conn.name || '',
        wss_url: conn.wss_url || '',
        api_key_id: conn.api_key_id || 0,
      })
    }
  }, [conn, editing])

  const deleteMutation = useMutation({
    mutationFn: () => deleteConnection(connId),
    onSuccess: () => { toast.success('连接已删除'); navigate({ to: '/connections' }) },
  })

  const toggleMutation = useMutation({
    mutationFn: (action: 'connect' | 'disconnect') =>
      action === 'connect' ? connectConnection(connId) : disconnectConnection(connId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['connection', id] }),
  })

  const updateMutation = useMutation({
    mutationFn: () => updateConnection(connId, {
      name: form.name,
      wss_url: form.wss_url,
      api_key_id: form.api_key_id,
    }),
    onSuccess: () => {
      toast.success('更新成功')
      setEditing(false)
      queryClient.invalidateQueries({ queryKey: ['connection', id] })
    },
    onError: () => {
      toast.error('更新失败')
    },
  })

  if (isLoading) return <div className="flex items-center justify-center py-20 text-muted-foreground">加载中...</div>
  if (!conn) return <div className="flex items-center justify-center py-20 text-muted-foreground">连接不存在</div>

  const boundKey = apiKeys.find((k: { id: number }) => k.id === (editing ? form.api_key_id : conn.api_key_id))

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-4xl mx-auto">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/connections' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{conn.name}</h1>
            <p className={`mt-0.5 text-sm font-medium ${statusColors[conn.connection_status] || ''}`}>
              {statusLabels[conn.connection_status] || conn.connection_status}
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
            onClick={() => toggleMutation.mutate(conn.connection_status === 'connected' ? 'disconnect' : 'connect')}
            disabled={toggleMutation.isPending}
          >
            {toggleMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin mr-1.5" /> :
              conn.connection_status === 'connected' ? <><WifiOff className="h-4 w-4 mr-1.5" />断开</> : <><Wifi className="h-4 w-4 mr-1.5" />连接</>
            }
          </Button>
          <Button variant="outline" size="sm" className="text-destructive" onClick={() => { if (confirm('确定删除？')) deleteMutation.mutate() }}>
            <Trash2 className="h-4 w-4" />
          </Button>
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
              <p className="text-sm">{conn.name}</p>
            )}
          </div>

          {/* Cloud type (read-only) */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">平台类型</Label>
            <p className="text-sm">{cloudTypeLabels[conn.cloud_type] || conn.cloud_type}</p>
          </div>

          {/* WSS URL */}
          {conn.cloud_type !== 'ssh' && (
            <div className="space-y-1.5 sm:col-span-2">
              <Label className="text-xs text-muted-foreground">WSS URL</Label>
              {editing ? (
                <Input value={form.wss_url} onChange={(e) => setForm({ ...form, wss_url: e.target.value })} placeholder="wss://example.com/ws" />
              ) : (
                <code className="block text-sm break-all rounded-md bg-muted/50 px-3 py-2">{conn.wss_url || '-'}</code>
              )}
            </div>
          )}

          {/* Bound API Key */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">绑定 API Key</Label>
            {editing ? (
              apiKeys.length === 0 ? (
                <p className="text-xs text-muted-foreground py-2">请先创建 API Key</p>
              ) : (
                <div className="flex flex-wrap gap-2 pt-1">
                  {apiKeys.map((k: { id: number; name: string; key_prefix: string }) => (
                    <button
                      key={k.id}
                      type="button"
                      onClick={() => setForm({ ...form, api_key_id: k.id })}
                      className={`rounded-lg border px-3 py-1.5 text-sm transition-all ${
                        form.api_key_id === k.id ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'hover:border-primary/30'
                      }`}
                    >
                      {k.name} <span className="text-muted-foreground">({k.key_prefix}...)</span>
                    </button>
                  ))}
                </div>
              )
            ) : (
              <p className="text-sm">{boundKey ? `${boundKey.name} (${boundKey.key_prefix}...)` : '-'}</p>
            )}
          </div>

          {/* Auto connect (read-only) */}
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">自动连接</Label>
            <p className="text-sm">{conn.auto_connect ? '是' : '否'}</p>
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
              disabled={!form.name.trim() || !form.wss_url.trim() || form.api_key_id === 0 || updateMutation.isPending}
            >
              {updateMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin mr-1.5" /> : <Check className="h-4 w-4 mr-1.5" />}
              保存
            </Button>
          </div>
        )}
      </div>

      {/* Connection status */}
      <div className="rounded-xl border bg-card p-5 space-y-3">
        <h2 className="text-sm font-medium text-muted-foreground">连接状态</h2>
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <p className="text-xs text-muted-foreground">状态</p>
            <p className={`mt-0.5 text-sm font-medium ${statusColors[conn.connection_status] || ''}`}>
              {statusLabels[conn.connection_status] || conn.connection_status}
            </p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">远程 ID</p>
            <p className="mt-0.5 text-sm font-mono">{conn.remote_id || '-'}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">最后连接</p>
            <p className="mt-0.5 text-sm">{conn.last_connected_at || '-'}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">最后错误</p>
            <p className="mt-0.5 text-sm">{conn.last_error || '-'}</p>
          </div>
        </div>
      </div>

      {/* SSH config */}
      {conn.cloud_config && Object.keys(conn.cloud_config).length > 0 && (
        <div className="rounded-xl border bg-card p-5 space-y-2">
          <h2 className="text-sm font-medium text-muted-foreground">配置</h2>
          <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto">
            {JSON.stringify(conn.cloud_config, null, 2)}
          </pre>
        </div>
      )}
    </div>
  )
}
