import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getServices, deleteService, updateService, testService } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { TransportType, ServiceListItem } from '@/types'
import {
  Plus, Search, Server, Trash2, RefreshCw, Zap, Loader2,
  Wifi, Terminal, Globe, Radio, Plug,
} from 'lucide-react'
import { toast } from 'sonner'

const transportIcons: Record<string, React.ComponentType<{ className?: string }>> = {
  'stdio': Terminal,
  'sse': Globe,
  'streamable-http': Wifi,
  'websocket': Radio,
  'passive-ws': Plug,
  'virtual': Zap,
}

const transportLabels: Record<string, string> = {
  'stdio': 'Stdio',
  'sse': 'SSE',
  'streamable-http': 'Streamable HTTP',
  'websocket': 'WebSocket',
  'passive-ws': 'Passive WS',
  'virtual': '虚拟',
}

function StatusBadge({ status }: { status: number }) {
  if (status === 1) return <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">启用</span>
  return <span className="inline-flex items-center gap-1 rounded-full bg-zinc-500/10 px-2 py-0.5 text-xs font-medium text-zinc-500">禁用</span>
}

function HealthBadge({ status }: { status: string }) {
  if (status === 'healthy') return <span className="inline-flex h-2 w-2 rounded-full bg-emerald-500" title="健康" />
  if (status === 'unhealthy') return <span className="inline-flex h-2 w-2 rounded-full bg-red-500" title="异常" />
  return <span className="inline-flex h-2 w-2 rounded-full bg-zinc-300 dark:bg-zinc-600" title="未知" />
}

export function ServiceListPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [transportFilter, setTransportFilter] = useState<string>('')
  const [searchInput, setSearchInput] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['services', keyword, transportFilter],
    queryFn: () => getServices({ keyword: keyword || undefined, transport_type: (transportFilter || undefined) as TransportType | undefined }),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteService,
    onSuccess: () => {
      toast.success('服务已删除')
      queryClient.invalidateQueries({ queryKey: ['services'] })
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) => updateService(id, { status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['services'] })
    },
  })

  const testMutation = useMutation({
    mutationFn: testService,
    onSuccess: (res) => {
      const result = res.data
      if (result?.connected) {
        toast.success(`连接成功，${result.tools_count ?? 0} 个工具，延迟 ${result.latency_ms ?? 0}ms`)
      } else {
        toast.error(`连接失败: ${result?.error || '未知错误'}`)
      }
      queryClient.invalidateQueries({ queryKey: ['services'] })
    },
    onError: () => {
      toast.error('测试请求失败')
    },
  })

  const services: ServiceListItem[] = data?.data || []

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    setKeyword(searchInput)
  }

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-6xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.services')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理所有 MCP 服务</p>
        </div>
        <Link to="/services/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            注册新服务
          </Button>
        </Link>
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <form onSubmit={handleSearch} className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="搜索服务名称..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </form>
        <div className="flex gap-2">
          <Button
            variant={transportFilter === '' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setTransportFilter('')}
          >
            全部
          </Button>
          {['stdio', 'sse', 'streamable-http', 'websocket', 'passive-ws'].map((t) => (
            <Button
              key={t}
              variant={transportFilter === t ? 'default' : 'outline'}
              size="sm"
              onClick={() => setTransportFilter(t)}
            >
              {transportLabels[t]}
            </Button>
          ))}
        </div>
      </div>

      {/* Table */}
      <div className="rounded-xl border bg-card overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">加载中...</div>
        ) : services.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Server className="h-10 w-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">暂无服务</p>
            <p className="text-xs text-muted-foreground/60 mt-1">点击"注册新服务"添加第一个 MCP 服务</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">服务名称</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">传输类型</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">健康</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">工具数</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">状态</th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody>
                {services.map((s) => {
                  const Icon = transportIcons[s.transport_type] || Globe
                  const isVirtual = s.transport_type === 'virtual'
                  return (
                    <tr key={s.id} className="border-b last:border-0 hover:bg-muted/30 transition-colors">
                      <td className="px-4 py-3">
                        <Link to="/services/$id" params={{ id: String(s.id) }} className="font-medium hover:text-primary transition-colors">
                          {s.display_name || s.name}
                        </Link>
                        {s.description && (
                          <p className="mt-0.5 text-xs text-muted-foreground line-clamp-1">{s.description}</p>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className="inline-flex items-center gap-1.5 text-xs">
                          <Icon className="h-3.5 w-3.5" />
                          {transportLabels[s.transport_type] || s.transport_type}
                        </span>
                      </td>
                      <td className="px-4 py-3"><HealthBadge status={s.health_status} /></td>
                      <td className="px-4 py-3 tabular-nums">{s.tools_count}</td>
                      <td className="px-4 py-3">
                        <button onClick={() => toggleMutation.mutate({ id: s.id, status: s.status === 1 ? 0 : 1 })}>
                          <StatusBadge status={s.status} />
                        </button>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          {!isVirtual && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="gap-1"
                            disabled={testMutation.isPending}
                            onClick={() => testMutation.mutate(s.id)}
                          >
                            {testMutation.isPending && testMutation.variables === s.id
                              ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                              : <Zap className="h-3.5 w-3.5" />}
                            测试
                          </Button>
                          )}
                          <Link to="/services/$id" params={{ id: String(s.id) }}>
                            <Button variant="ghost" size="sm">详情</Button>
                          </Link>
                          {!isVirtual && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => {
                              if (confirm(`确定删除服务 "${s.display_name || s.name}"？`)) {
                                deleteMutation.mutate(s.id)
                              }
                            }}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                          )}
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
