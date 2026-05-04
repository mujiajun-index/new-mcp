import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from '@tanstack/react-router'
import { getConnection, deleteConnection, connectConnection, disconnectConnection } from '../api'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'
import { ArrowLeft, Trash2, Wifi, WifiOff, Loader2 } from 'lucide-react'

const statusColors: Record<string, string> = {
  connected: 'text-emerald-600 dark:text-emerald-400',
  disconnected: 'text-zinc-500',
  connecting: 'text-amber-600 dark:text-amber-400',
  error: 'text-red-600 dark:text-red-400',
}

const cloudTypeLabels: Record<string, string> = {
  xiaozhi: '小智', custom: '自定义 WSS', ssh: 'SSH',
}

export function ConnectionDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()
  const connId = Number(id)

  const { data, isLoading } = useQuery({
    queryKey: ['connection', id],
    queryFn: () => getConnection(connId),
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteConnection(connId),
    onSuccess: () => { toast.success('连接已删除'); navigate({ to: '/connections' }) },
  })

  const toggleMutation = useMutation({
    mutationFn: (action: 'connect' | 'disconnect') =>
      action === 'connect' ? connectConnection(connId) : disconnectConnection(connId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['connection', id] }),
  })

  const conn = data?.data

  if (isLoading) return <div className="flex items-center justify-center py-20 text-muted-foreground">加载中...</div>
  if (!conn) return <div className="flex items-center justify-center py-20 text-muted-foreground">连接不存在</div>

  const info = [
    { label: '名称', value: conn.name },
    { label: '平台类型', value: cloudTypeLabels[conn.cloud_type] || conn.cloud_type },
    { label: '连接状态', value: conn.connection_status },
    { label: '远程 ID', value: conn.remote_id || '-' },
    { label: '自动连接', value: conn.auto_connect ? '是' : '否' },
    { label: '最后连接', value: conn.last_connected_at || '-' },
    { label: '最后错误', value: conn.last_error || '-' },
  ]

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-4xl mx-auto">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/connections' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{conn.name}</h1>
            <p className={`mt-0.5 text-sm font-medium ${statusColors[conn.connection_status] || ''}`}>
              {conn.connection_status}
            </p>
          </div>
        </div>
        <div className="flex gap-2">
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

      <div className="rounded-xl border bg-card p-5">
        <div className="grid gap-3 sm:grid-cols-2">
          {info.map((item) => (
            <div key={item.label}>
              <p className="text-xs text-muted-foreground">{item.label}</p>
              <p className="mt-0.5 text-sm">{item.value}</p>
            </div>
          ))}
        </div>
      </div>

      {conn.wss_url && (
        <div className="rounded-xl border bg-card p-5">
          <p className="text-xs text-muted-foreground">WSS URL</p>
          <code className="mt-1 block text-sm break-all">{conn.wss_url}</code>
        </div>
      )}

      {conn.cloud_config && Object.keys(conn.cloud_config).length > 0 && (
        <div className="rounded-xl border bg-card p-5">
          <p className="text-xs text-muted-foreground mb-2">配置</p>
          <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto">
            {JSON.stringify(conn.cloud_config, null, 2)}
          </pre>
        </div>
      )}
    </div>
  )
}
