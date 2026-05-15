import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getVisionConfigs,
  deleteVisionConfig,
  enableVisionConfig,
  disableVisionConfig,
} from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Eye, Plus, Trash2, Loader2, Power, PowerOff,
} from 'lucide-react'
import { toast } from 'sonner'
import type { VisionConfigListItem } from '../api'

function EnabledBadge({ enabled }: { enabled: boolean }) {
  if (enabled) {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
        已启用
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-zinc-500/10 px-2 py-0.5 text-xs font-medium text-zinc-500">
      已禁用
    </span>
  )
}

const providerLabels: Record<string, string> = {
  openai: 'OpenAI',
  custom: 'Custom',
  glm: 'GLM',
  qwen: 'Qwen',
  ollama: 'Ollama',
}

export function VisionListPage() {
  const queryClient = useQueryClient()
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['vision'],
    queryFn: getVisionConfigs,
  })

  const deleteMutation = useMutation({
    mutationFn: deleteVisionConfig,
    onSuccess: () => {
      toast.success('视觉配置已删除')
      queryClient.invalidateQueries({ queryKey: ['vision'] })
    },
  })

  const enableMutation = useMutation({
    mutationFn: enableVisionConfig,
    onSuccess: () => {
      toast.success('已启用')
      queryClient.invalidateQueries({ queryKey: ['vision'] })
    },
    onError: () => {
      toast.error('启用失败')
    },
  })

  const disableMutation = useMutation({
    mutationFn: disableVisionConfig,
    onSuccess: () => {
      toast.success('已禁用')
      queryClient.invalidateQueries({ queryKey: ['vision'] })
    },
    onError: () => {
      toast.error('禁用失败')
    },
  })

  const configs: VisionConfigListItem[] = data?.data || []

  const filtered = keyword.trim()
    ? configs.filter(
        (c) =>
          c.name.toLowerCase().includes(keyword.toLowerCase()) ||
          c.provider.toLowerCase().includes(keyword.toLowerCase()) ||
          c.model_name.toLowerCase().includes(keyword.toLowerCase())
      )
    : configs

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    setKeyword(searchInput)
  }

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-6xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">视觉配置</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            管理视觉模型接入，为 MCP 服务提供图像理解能力
          </p>
        </div>
        <Link to="/vision/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            新建视觉配置
          </Button>
        </Link>
      </div>

      {/* Search */}
      <form onSubmit={handleSearch} className="relative max-w-sm">
        <Eye className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="搜索名称、Provider 或模型..."
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          className="pl-9"
        />
      </form>

      {/* Table */}
      <div className="rounded-xl border bg-card overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">
            <Loader2 className="h-5 w-5 animate-spin mr-2" />
            加载中...
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Eye className="h-10 w-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">暂无视觉配置</p>
            <p className="text-xs text-muted-foreground/60 mt-1">
              点击"新建视觉配置"添加第一个视觉模型
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">名称</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Provider</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">模型</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">状态</th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((c) => {
                  const isEnabled = c.auto_register
                  return (
                    <tr
                      key={c.id}
                      className="border-b last:border-0 hover:bg-muted/30 transition-colors"
                    >
                      <td className="px-4 py-3">
                        <Link
                          to="/vision/$id"
                          params={{ id: String(c.id) }}
                          className="font-medium hover:text-primary transition-colors"
                        >
                          {c.name}
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-xs text-muted-foreground">
                          {providerLabels[c.provider] || c.provider}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className="font-mono text-xs">{c.model_name}</span>
                      </td>
                      <td className="px-4 py-3">
                        <EnabledBadge enabled={isEnabled} />
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="gap-1"
                            disabled={
                              enableMutation.isPending || disableMutation.isPending
                            }
                            onClick={() => {
                              if (isEnabled) {
                                disableMutation.mutate(c.id)
                              } else {
                                enableMutation.mutate(c.id)
                              }
                            }}
                            title={isEnabled ? '禁用' : '启用'}
                          >
                            {isEnabled ? (
                              <PowerOff className="h-3.5 w-3.5" />
                            ) : (
                              <Power className="h-3.5 w-3.5" />
                            )}
                            {isEnabled ? '禁用' : '启用'}
                          </Button>
                          <Link to="/vision/$id" params={{ id: String(c.id) }}>
                            <Button variant="ghost" size="sm">
                              详情
                            </Button>
                          </Link>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-destructive hover:text-destructive"
                            onClick={() => {
                              if (confirm(`确定删除视觉配置 "${c.name}"？`)) {
                                deleteMutation.mutate(c.id)
                              }
                            }}
                          >
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
