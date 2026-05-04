import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getApiKeys, createApiKey, deleteApiKey } from '../api'
import { getGroups } from '@/features/groups/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { Plus, Trash2, Copy, Key, X, Check } from 'lucide-react'

export function ApiKeyPage() {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [newKey, setNewKey] = useState<string | null>(null)
  const [form, setForm] = useState({ name: '', groups: '' })

  const { data: keysData, isLoading } = useQuery({
    queryKey: ['api-keys'],
    queryFn: () => getApiKeys(),
  })

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: () => getGroups(),
  })

  const createMutation = useMutation({
    mutationFn: () => createApiKey({
      name: form.name,
      groups: form.groups ? form.groups.split(',').map((s) => s.trim()) : [],
    }),
    onSuccess: (res) => {
      toast.success('API Key 创建成功')
      setNewKey(res.data?.key || null)
      setShowCreate(false)
      setForm({ name: '', groups: '' })
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteApiKey,
    onSuccess: () => {
      toast.success('API Key 已删除')
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  const keys = keysData?.data || []
  const groups = groupsData?.data || []

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-6xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">API 密钥</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理用于 MCP 端点访问的 API Key</p>
        </div>
        <Button className="gap-2" onClick={() => setShowCreate(true)}>
          <Plus className="h-4 w-4" />创建密钥
        </Button>
      </div>

      {/* New key display */}
      {newKey && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-5">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-amber-600 dark:text-amber-400">请立即复制 API Key</p>
              <p className="mt-1 text-xs text-muted-foreground">此密钥仅显示一次，关闭后将无法再次查看</p>
            </div>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setNewKey(null)}>
              <X className="h-4 w-4" />
            </Button>
          </div>
          <div className="mt-3 flex items-center gap-2">
            <code className="flex-1 rounded-lg bg-muted px-3 py-2 text-sm font-mono break-all">{newKey}</code>
            <Button variant="outline" size="sm" onClick={() => { navigator.clipboard.writeText(newKey); toast.success('已复制') }}>
              <Copy className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}

      {/* Create dialog */}
      {showCreate && (
        <div className="rounded-xl border bg-card p-5 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">创建新密钥</h2>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setShowCreate(false)}>
              <X className="h-4 w-4" />
            </Button>
          </div>
          <div className="space-y-2">
            <Label>名称</Label>
            <Input placeholder="my-key" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="space-y-2">
            <Label>关联分组 (逗号分隔的 endpoint_slug)</Label>
            <Input placeholder="group1, group2" value={form.groups} onChange={(e) => setForm({ ...form, groups: e.target.value })} />
            {groups.length > 0 && (
              <div className="flex flex-wrap gap-1.5 mt-1">
                {groups.map((g: { id: number; name: string; endpoint_slug?: string }) => (
                  <button
                    key={g.id}
                    type="button"
                    className="rounded bg-muted px-2 py-0.5 text-xs hover:bg-muted/80"
                    onClick={() => {
                      const slug = g.endpoint_slug || g.name
                      const current = form.groups ? form.groups.split(',').map((s) => s.trim()) : []
                      if (!current.includes(slug)) {
                        setForm({ ...form, groups: [...current, slug].join(', ') })
                      }
                    }}
                  >
                    {g.name}
                  </button>
                ))}
              </div>
            )}
          </div>
          <Button onClick={() => createMutation.mutate()} disabled={!form.name.trim() || createMutation.isPending}>
            创建
          </Button>
        </div>
      )}

      {/* List */}
      <div className="rounded-xl border bg-card overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">加载中...</div>
        ) : keys.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Key className="h-10 w-10 text-muted-foreground/30 mb-3" />
            <p className="text-sm text-muted-foreground">暂无 API 密钥</p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">名称</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">前缀</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">分组</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">状态</th>
                <th className="px-4 py-3 text-right font-medium text-muted-foreground">操作</th>
              </tr>
            </thead>
            <tbody>
              {keys.map((key: { id: number; name: string; key_prefix: string; groups: string[]; status: number }) => (
                <tr key={key.id} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-3 font-medium">{key.name}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{key.key_prefix}...</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {key.groups?.map((g) => (
                        <span key={g} className="rounded bg-muted px-1.5 py-0.5 text-[10px]">{g}</span>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex h-2 w-2 rounded-full ${key.status === 1 ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button variant="ghost" size="sm" className="text-destructive" onClick={() => {
                      if (confirm(`确定删除密钥 "${key.name}"？`)) deleteMutation.mutate(key.id)
                    }}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
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
