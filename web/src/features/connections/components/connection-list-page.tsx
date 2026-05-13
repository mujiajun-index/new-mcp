import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getConnections, deleteConnection, connectConnection, disconnectConnection } from '../api'
import { Button } from '@/components/ui/button'
import { Plus, Trash2, Cloud, Wifi, WifiOff, Eye } from 'lucide-react'
import { toast } from 'sonner'

const statusColors: Record<string, string> = {
  connected: 'bg-emerald-500',
  disconnected: 'bg-zinc-400',
  connecting: 'bg-amber-500',
  error: 'bg-red-500',
}

const statusLabels: Record<string, string> = {
  connected: '已连接',
  disconnected: '已断开',
  connecting: '连接中',
  error: '错误',
}

const cloudTypeLabels: Record<string, string> = {
  xiaozhi: '小智',
  custom: '自定义 WSS',
  ssh: 'SSH',
}

export function ConnectionListPage() {
  const queryClient = useQueryClient()
  const { data, isLoading } = useQuery({
    queryKey: ['connections'],
    queryFn: () => getConnections(),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteConnection,
    onSuccess: () => {
      toast.success('连接已删除')
      queryClient.invalidateQueries({ queryKey: ['connections'] })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: 'connect' | 'disconnect' }) => {
      return action === 'connect' ? connectConnection(id) : disconnectConnection(id)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['connections'] })
    },
  })

  const connections = data?.data || []

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-6xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">云端连接</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理 MCP 服务的云端推送连接</p>
        </div>
        <Link to="/connections/create">
          <Button className="gap-2"><Plus className="h-4 w-4" />添加连接</Button>
        </Link>
      </div>

      <div className="rounded-xl border bg-card overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">加载中...</div>
        ) : connections.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Cloud className="h-10 w-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">暂无云端连接</p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">名称</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">平台</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">状态</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">远程 ID</th>
                <th className="px-4 py-3 text-right font-medium text-muted-foreground">操作</th>
              </tr>
            </thead>
            <tbody>
              {connections.map((c: { id: number; name: string; cloud_type: string; connection_status: string; remote_id: string }) => (
                <tr key={c.id} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-3 font-medium">{c.name}</td>
                  <td className="px-4 py-3">{cloudTypeLabels[c.cloud_type] || c.cloud_type}</td>
                  <td className="px-4 py-3">
                    <span className="inline-flex items-center gap-1.5">
                      <span className={`h-2 w-2 rounded-full ${statusColors[c.connection_status] || 'bg-zinc-400'}`} />
                      {statusLabels[c.connection_status] || c.connection_status}
                    </span>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{c.remote_id || '-'}</td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Link to="/connections/$id" params={{ id: String(c.id) }}>
                        <Button variant="ghost" size="sm" className="gap-1">
                          <Eye className="h-3.5 w-3.5" />详情
                        </Button>
                      </Link>
                      <Button
                        variant="outline"
                        size="sm"
                        className="gap-1"
                        onClick={() => toggleMutation.mutate({
                          id: c.id,
                          action: c.connection_status === 'connected' ? 'disconnect' : 'connect',
                        })}
                        disabled={toggleMutation.isPending}
                      >
                        {c.connection_status === 'connected' ? <WifiOff className="h-3.5 w-3.5" /> : <Wifi className="h-3.5 w-3.5" />}
                        {c.connection_status === 'connected' ? '断开' : '连接'}
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" onClick={() => {
                        if (confirm(`确定删除连接 "${c.name}"？`)) deleteMutation.mutate(c.id)
                      }}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
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
