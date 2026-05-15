import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from '@tanstack/react-router'
import { getService, deleteService, testService, refreshTools } from '../api'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'
import {
  ArrowLeft, Trash2, Zap, RefreshCw, Server,
  Terminal, Globe, Wifi, Radio, Plug,
} from 'lucide-react'
import type { McpTool } from '@/types'

const transportLabels: Record<string, string> = {
  'stdio': 'Stdio', 'sse': 'SSE', 'streamable-http': 'Streamable HTTP',
  'websocket': 'WebSocket', 'passive-ws': 'Passive WS', 'virtual': '虚拟',
}

const sourceLabels: Record<string, { label: string; color: string }> = {
  'vision': { label: '视觉', color: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300' },
  'camera': { label: '摄像头', color: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' },
}

export function ServiceDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['service', id],
    queryFn: () => getService(Number(id)),
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteService(Number(id)),
    onSuccess: () => {
      toast.success('服务已删除')
      navigate({ to: '/services' })
    },
  })

  const testMutation = useMutation({
    mutationFn: () => testService(Number(id)),
    onSuccess: (res) => {
      if (res.data?.connected) {
        toast.success(`连接成功 · ${res.data.tools_count} 个工具 · ${res.data.latency_ms}ms`)
      } else {
        toast.error(`连接失败: ${res.data?.error || '未知错误'}`)
      }
    },
  })

  const refreshMutation = useMutation({
    mutationFn: () => refreshTools(Number(id)),
    onSuccess: (res) => {
      toast.success(`已刷新，发现 ${res.data?.tools_count || 0} 个工具`)
      queryClient.invalidateQueries({ queryKey: ['service', id] })
    },
  })

  const service = data?.data

  if (isLoading) {
    return <div className="flex items-center justify-center py-20 text-muted-foreground">加载中...</div>
  }

  if (!service) {
    return <div className="flex items-center justify-center py-20 text-muted-foreground">服务不存在</div>
  }

  const tools: McpTool[] = service.tools_cache || []
  const isVirtual = service.transport_type === 'virtual'
  const virtualSource = isVirtual ? sourceLabels[service.source] : null

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-6xl">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/services' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{service.display_name || service.name}</h1>
            <div className="mt-0.5 flex items-center gap-2">
              <p className="text-sm text-muted-foreground">{service.name}</p>
              {virtualSource && (
                <span className={`inline-flex items-center rounded-md px-1.5 py-0.5 text-[10px] font-medium ${virtualSource.color}`}>
                  {virtualSource.label}
                </span>
              )}
            </div>
            {service.description && <p className="mt-2 text-sm text-muted-foreground">{service.description}</p>}
          </div>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => testMutation.mutate()} disabled={testMutation.isPending || isVirtual} title={isVirtual ? '虚拟服务不支持测试连接' : undefined}>
            <Zap className="h-3.5 w-3.5" />
            {testMutation.isPending ? '测试中...' : '测试连接'}
          </Button>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => refreshMutation.mutate()} disabled={refreshMutation.isPending || isVirtual} title={isVirtual ? '虚拟服务不支持刷新工具' : undefined}>
            <RefreshCw className={`h-3.5 w-3.5 ${refreshMutation.isPending ? 'animate-spin' : ''}`} />
            刷新工具
          </Button>
          {service.transport_type !== 'virtual' && (
          <Button variant="outline" size="sm" className="text-destructive hover:text-destructive" onClick={() => {
            if (confirm('确定删除此服务？')) deleteMutation.mutate()
          }}>
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
          )}
        </div>
      </div>

      {/* Info cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {[
          { label: '传输类型', value: transportLabels[service.transport_type] || service.transport_type },
          { label: '健康状态', value: service.health_status || 'unknown' },
          { label: '工具数', value: String(tools.length) },
          { label: '协议版本', value: service.protocol_version || '-' },
        ].map((item) => (
          <div key={item.label} className="rounded-xl border bg-card p-4">
            <p className="text-xs text-muted-foreground">{item.label}</p>
            <p className="mt-1 text-sm font-medium">{item.value}</p>
          </div>
        ))}
      </div>

      {/* Server Info */}
      {service.server_info && Object.keys(service.server_info).length > 0 && (
        <div className="rounded-xl border bg-card p-5">
          <h2 className="mb-3 text-sm font-semibold">服务器信息</h2>
          <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto">
            {JSON.stringify(service.server_info, null, 2)}
          </pre>
        </div>
      )}

      {/* Config */}
      {service.config && Object.keys(service.config).length > 0 && (
        <div className="rounded-xl border bg-card p-5">
          <h2 className="mb-3 text-sm font-semibold">连接配置</h2>
          <pre className="rounded-lg bg-muted/50 p-3 text-xs overflow-auto">
            {JSON.stringify(service.config, null, 2)}
          </pre>
        </div>
      )}

      {/* Tools */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="mb-3 text-sm font-semibold">工具列表 ({tools.length})</h2>
        {tools.length === 0 ? (
          <div className="flex flex-col items-center py-8 text-center">
            <Server className="h-8 w-8 text-muted-foreground/30 mb-2" />
            <p className="text-sm text-muted-foreground">暂无工具</p>
            <p className="text-xs text-muted-foreground/60 mt-1">点击"刷新工具"重新获取</p>
          </div>
        ) : (
          <div className="space-y-2">
            {tools.map((tool) => (
              <div key={tool.name} className="rounded-lg border p-3">
                <p className="text-sm font-medium font-mono">{tool.name}</p>
                {tool.description && (
                  <p className="mt-0.5 text-xs text-muted-foreground">{tool.description}</p>
                )}
                {tool.inputSchema?.properties != null && (
                  <div className="mt-2 flex flex-wrap gap-1">
                    {Object.keys(tool.inputSchema.properties as Record<string, unknown>).map((param) => (
                      <span key={param} className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
                        {param}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
