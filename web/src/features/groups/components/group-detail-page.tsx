import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from '@tanstack/react-router'
import { getGroup, deleteGroup, removeGroupService, getGroupTools, getGroupEndpoint, updateGroup } from '../api'
import { getServices } from '@/features/services/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'
import { ArrowLeft, Trash2, Copy, RefreshCw, Plus, X, Globe, Radio } from 'lucide-react'
import { useState } from 'react'

export function GroupDetailPage() {
  const navigate = useNavigate()
  const { id } = useParams({ strict: false }) as { id: string }
  const queryClient = useQueryClient()
  const groupId = Number(id)
  const [showAddService, setShowAddService] = useState(false)

  const { data: groupData, isLoading } = useQuery({
    queryKey: ['group', id],
    queryFn: () => getGroup(groupId),
  })

  const { data: toolsData } = useQuery({
    queryKey: ['group-tools', id],
    queryFn: () => getGroupTools(groupId),
  })

  const { data: endpointData } = useQuery({
    queryKey: ['group-endpoint', id],
    queryFn: () => getGroupEndpoint(groupId),
  })

  const { data: servicesData } = useQuery({
    queryKey: ['services'],
    queryFn: () => getServices(),
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteGroup(groupId),
    onSuccess: () => {
      toast.success('分组已删除')
      navigate({ to: '/groups' })
    },
  })

  const removeServiceMutation = useMutation({
    mutationFn: (serviceId: number) => removeGroupService(groupId, serviceId),
    onSuccess: () => {
      toast.success('服务已移除')
      queryClient.invalidateQueries({ queryKey: ['group', id] })
      queryClient.invalidateQueries({ queryKey: ['group-tools', id] })
    },
  })

  const toggleModeMutation = useMutation({
    mutationFn: (mode: 'direct' | 'smart') => updateGroup(groupId, { expose_mode: mode }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['group', id] })
      toast.success('模式已切换')
    },
  })

  const group = groupData?.data
  const tools = toolsData?.data || []
  const endpoint = endpointData?.data
  const allServices = servicesData?.data || []
  const existingIds = new Set((group?.services || []).map((s: { id: number }) => s.id))
  const availableServices = allServices.filter((s: { id: number }) => !existingIds.has(s.id))

  const copyToClipboard = (text: string) => {
    if (navigator.clipboard) {
      navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    toast.success('已复制到剪贴板')
  }

  if (isLoading) return <div className="flex items-center justify-center py-20 text-muted-foreground">加载中...</div>
  if (!group) return <div className="flex items-center justify-center py-20 text-muted-foreground">分组不存在</div>

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-6xl">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/groups' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">{group.display_name || group.name}</h1>
            <p className="mt-0.5 text-sm text-muted-foreground font-mono">{group.name}</p>
          </div>
        </div>
        <Button variant="outline" size="sm" className="text-destructive" onClick={() => { if (confirm('确定删除此分组？')) deleteMutation.mutate() }}>
          <Trash2 className="h-3.5 w-3.5 mr-1.5" />删除
        </Button>
      </div>

      {/* Mode switch */}
      <div className="rounded-xl border bg-card p-4">
        <p className="text-xs text-muted-foreground mb-2">暴露模式</p>
        <div className="flex gap-2">
          {(['direct', 'smart'] as const).map((mode) => (
            <button
              key={mode}
              onClick={() => toggleModeMutation.mutate(mode)}
              className={`rounded-lg border px-4 py-2 text-sm font-medium transition-all ${
                group.expose_mode === mode ? 'border-primary bg-primary/5' : 'hover:border-primary/30'
              }`}
            >
              {mode === 'direct' ? 'Direct' : 'Smart'}
            </button>
          ))}
        </div>
      </div>

      {/* Endpoint info */}
      {endpoint && (
        <div className="rounded-xl border bg-card p-5 space-y-3">
          <h2 className="text-sm font-semibold">端点信息</h2>
          {endpoint.streamable_http_url && (
            <div className="flex items-center gap-2">
              <Globe className="h-4 w-4 text-muted-foreground shrink-0" />
              <code className="flex-1 rounded bg-muted px-2 py-1 text-xs truncate">{endpoint.streamable_http_url}</code>
              <Button variant="ghost" size="icon" className="h-7 w-7 shrink-0" onClick={() => copyToClipboard(endpoint.streamable_http_url)}>
                <Copy className="h-3.5 w-3.5" />
              </Button>
            </div>
          )}
          {endpoint.websocket_url && (
            <div className="flex items-center gap-2">
              <Radio className="h-4 w-4 text-muted-foreground shrink-0" />
              <code className="flex-1 rounded bg-muted px-2 py-1 text-xs truncate">{endpoint.websocket_url}</code>
              <Button variant="ghost" size="icon" className="h-7 w-7 shrink-0" onClick={() => copyToClipboard(endpoint.websocket_url)}>
                <Copy className="h-3.5 w-3.5" />
              </Button>
            </div>
          )}
        </div>
      )}

      {/* Services */}
      <div className="rounded-xl border bg-card p-5">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-semibold">已添加服务 ({group.services?.length || 0})</h2>
          <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setShowAddService(!showAddService)}>
            <Plus className="h-3.5 w-3.5" />添加服务
          </Button>
        </div>

        {showAddService && availableServices.length > 0 && (
          <div className="mb-4 rounded-lg border bg-muted/30 p-3 space-y-2">
            <p className="text-xs text-muted-foreground">选择要添加的服务：</p>
            <div className="flex flex-wrap gap-2">
              {availableServices.map((s: { id: number; name: string; display_name: string }) => (
                <Button
                  key={s.id}
                  variant="outline"
                  size="sm"
                  onClick={async () => {
                    const { addGroupServices } = await import('../api')
                    await addGroupServices(groupId, [s.id])
                    toast.success(`已添加 ${s.display_name || s.name}`)
                    queryClient.invalidateQueries({ queryKey: ['group', id] })
                    queryClient.invalidateQueries({ queryKey: ['group-tools', id] })
                  }}
                >
                  {s.display_name || s.name}
                </Button>
              ))}
            </div>
          </div>
        )}

        {(!group.services || group.services.length === 0) ? (
          <p className="text-sm text-muted-foreground text-center py-6">暂无服务，点击上方"添加服务"</p>
        ) : (
          <div className="space-y-2">
            {group.services.map((s: { id: number; name: string; enabled: boolean; tools_count: number }) => (
              <div key={s.id} className="flex items-center justify-between rounded-lg border px-3 py-2">
                <div className="flex items-center gap-2">
                  <span className={`h-2 w-2 rounded-full ${s.enabled ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                  <span className="text-sm font-medium">{s.name}</span>
                  <span className="text-xs text-muted-foreground">{s.tools_count} 工具</span>
                </div>
                <Button variant="ghost" size="sm" className="text-destructive h-7" onClick={() => removeServiceMutation.mutate(s.id)}>
                  <X className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Aggregated tools */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="mb-3 text-sm font-semibold">聚合工具列表 ({tools.length})</h2>
        {tools.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-6">添加服务并刷新后显示工具</p>
        ) : (
          <div className="space-y-2">
            {tools.map((tool: { name: string; original_name: string; service_name: string; description: string; enabled: boolean }) => (
              <div key={tool.name} className="flex items-start gap-3 rounded-lg border p-3">
                <span className={`mt-0.5 h-2 w-2 shrink-0 rounded-full ${tool.enabled ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium font-mono">{tool.name}</p>
                  <p className="text-xs text-muted-foreground">
                    来自 {tool.service_name} · 原名: {tool.original_name}
                  </p>
                  {tool.description && <p className="mt-1 text-xs text-muted-foreground">{tool.description}</p>}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
