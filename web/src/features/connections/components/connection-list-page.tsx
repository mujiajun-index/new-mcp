import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getConnections, deleteConnection, connectConnection, disconnectConnection, updateConnection } from '../api'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { Plus, Trash2, Cloud, Wifi, WifiOff, Eye, Loader2, MoreHorizontal } from 'lucide-react'
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

type ConnectionItem = {
  id: number
  name: string
  cloud_type: string
  connection_status: string
  remote_id: string
  expose_mode: string
  status: number
}

export function ConnectionListPage() {
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
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
    onSuccess: (_data, variables) => {
      toast.success(variables.action === 'connect' ? '已连接' : '已断开')
      queryClient.invalidateQueries({ queryKey: ['connections'] })
    },
    onError: (err) => {
      toast.error(err.message || '操作失败')
    },
  })

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) => updateConnection(id, { status }),
    onSuccess: (_data, variables) => {
      toast.success(variables.status === 1 ? '已启用' : '已禁用')
      queryClient.invalidateQueries({ queryKey: ['connections'] })
    },
  })

  const connections: ConnectionItem[] = data?.data || []

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">云端连接</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理 MCP 服务的云端推送连接</p>
        </div>
        <Link to="/connections/create">
          <Button className="gap-2"><Plus className="h-4 w-4" />添加连接</Button>
        </Link>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">加载中...</div>
        ) : connections.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Cloud className="mb-3 h-10 w-10 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">暂无云端连接</p>
          </div>
        ) : isMobile ? (
          <div className="divide-y">
            {connections.map((c) => {
              const disabled = c.status !== 1
              return (
                <MobileListCard
                  key={c.id}
                  className={disabled ? 'opacity-50' : undefined}
                  title={c.name}
                  badge={
                    <span className="inline-flex items-center gap-1.5">
                      <span className={`h-2 w-2 rounded-full ${statusColors[c.connection_status] || 'bg-zinc-400'}`} />
                      {statusLabels[c.connection_status] || c.connection_status}
                    </span>
                  }
                  meta={[
                    { label: '平台', value: cloudTypeLabels[c.cloud_type] || c.cloud_type },
                    {
                      label: '暴露',
                      value: (
                        <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
                          c.expose_mode === 'direct' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' : 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'
                        }`}>
                          {c.expose_mode === 'direct' ? '直接' : '智能'}
                        </span>
                      ),
                    },
                    {
                      label: '启用',
                      value: (
                        <button
                          onClick={() => statusMutation.mutate({ id: c.id, status: disabled ? 1 : 2 })}
                          className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium transition-colors ${
                            disabled
                              ? 'bg-zinc-100 text-zinc-500 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-400'
                              : 'bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-900/30 dark:text-emerald-300'
                          }`}
                        >
                          {disabled ? '已禁用' : '已启用'}
                        </button>
                      ),
                    },
                    { label: '远程 ID', value: <span className="font-mono text-xs">{c.remote_id || '-'}</span> },
                  ]}
                  actions={
                    <>
                      <Button
                        variant="outline"
                        size="sm"
                        className="gap-1"
                        onClick={() => toggleMutation.mutate({
                          id: c.id,
                          action: c.connection_status === 'connected' ? 'disconnect' : 'connect',
                        })}
                        disabled={disabled || toggleMutation.isPending}
                      >
                        {toggleMutation.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : c.connection_status === 'connected' ? <WifiOff className="h-3.5 w-3.5" /> : <Wifi className="h-3.5 w-3.5" />}
                        {toggleMutation.isPending ? '处理中...' : c.connection_status === 'connected' ? '断开' : '连接'}
                      </Button>
                      <Link to="/connections/$id" params={{ id: String(c.id) }}>
                        <Button variant="ghost" size="sm" className="gap-1">
                          <Eye className="h-3.5 w-3.5" />详情
                        </Button>
                      </Link>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="h-8 w-8">
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            className="text-destructive focus:text-destructive"
                            onClick={() => {
                              if (confirm(`确定删除连接 "${c.name}"？`)) deleteMutation.mutate(c.id)
                            }}
                          >
                            <Trash2 className="mr-2 h-4 w-4" />
                            删除
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </>
                  }
                />
              )
            })}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">名称</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">平台</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">暴露模式</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">连接状态</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">启用</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">远程 ID</th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody>
                {connections.map((c) => {
                  const disabled = c.status !== 1
                  return (
                    <tr key={c.id} className={`border-b last:border-0 hover:bg-muted/30${disabled ? ' opacity-50' : ''}`}>
                      <td className="px-4 py-3 font-medium">{c.name}</td>
                      <td className="px-4 py-3">{cloudTypeLabels[c.cloud_type] || c.cloud_type}</td>
                      <td className="px-4 py-3">
                        <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
                          c.expose_mode === 'direct' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' : 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'
                        }`}>
                          {c.expose_mode === 'direct' ? '直接' : '智能'}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className="inline-flex items-center gap-1.5">
                          <span className={`h-2 w-2 rounded-full ${statusColors[c.connection_status] || 'bg-zinc-400'}`} />
                          {statusLabels[c.connection_status] || c.connection_status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <button
                          onClick={() => statusMutation.mutate({ id: c.id, status: disabled ? 1 : 2 })}
                          className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium transition-colors ${
                            disabled
                              ? 'bg-zinc-100 text-zinc-500 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-400'
                              : 'bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-900/30 dark:text-emerald-300'
                          }`}
                        >
                          {disabled ? '已禁用' : '已启用'}
                        </button>
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
                            disabled={disabled || toggleMutation.isPending}
                          >
                            {toggleMutation.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : c.connection_status === 'connected' ? <WifiOff className="h-3.5 w-3.5" /> : <Wifi className="h-3.5 w-3.5" />}
                            {toggleMutation.isPending ? '处理中...' : c.connection_status === 'connected' ? '断开' : '连接'}
                          </Button>
                          <Button variant="ghost" size="sm" className="text-destructive" onClick={() => {
                            if (confirm(`确定删除连接 "${c.name}"？`)) deleteMutation.mutate(c.id)
                          }}>
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
