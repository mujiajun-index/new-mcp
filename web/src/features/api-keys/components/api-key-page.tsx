import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getApiKeys, createApiKey, updateApiKey, deleteApiKey } from '../api'
import { getGroups } from '@/features/groups/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'
import { Plus, Trash2, Copy, Key, X, Pencil, ToggleLeft, ToggleRight, Infinity } from 'lucide-react'
import type { ApiKeyListItem } from '@/types'

export function ApiKeyPage() {
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [editingKey, setEditingKey] = useState<ApiKeyListItem | null>(null)
  const [newKey, setNewKey] = useState<string | null>(null)
  const [form, setForm] = useState({
    name: '',
    groups: '',
    quota: '',
    unlimited_quota: true,
    allow_ips: '',
    expires_at: '',
  })

  const { data: keysData, isLoading } = useQuery({
    queryKey: ['api-keys'],
    queryFn: () => getApiKeys(),
  })

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: () => getGroups(),
  })

  const resetForm = () => setForm({ name: '', groups: '', quota: '', unlimited_quota: true, allow_ips: '', expires_at: '' })

  const createMutation = useMutation({
    mutationFn: () => createApiKey({
      name: form.name,
      groups: form.groups ? form.groups.split(',').map(s => s.trim()) : [],
      quota: form.unlimited_quota ? undefined : (parseInt(form.quota) || undefined),
      unlimited_quota: form.unlimited_quota,
      allow_ips: form.allow_ips || undefined,
      expires_at: form.expires_at || undefined,
    }),
    onSuccess: (res) => {
      toast.success('API Key 创建成功')
      setNewKey(res.data?.key || null)
      setShowCreate(false)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: () => toast.error('创建失败'),
  })

  const updateMutation = useMutation({
    mutationFn: (data: { id: number; body: any }) => updateApiKey(data.id, data.body),
    onSuccess: () => {
      toast.success('更新成功')
      setEditingKey(null)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: () => toast.error('更新失败'),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteApiKey,
    onSuccess: () => {
      toast.success('已删除')
      queryClient.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  const toggleStatus = (key: ApiKeyListItem) => {
    updateMutation.mutate({
      id: key.id,
      body: { status: key.status === 1 ? 2 : 1 },
    })
  }

  const startEdit = (key: ApiKeyListItem) => {
    setEditingKey(key)
    setForm({
      name: key.name,
      groups: key.groups?.join(', ') || '',
      quota: key.unlimited_quota ? '' : String(key.quota),
      unlimited_quota: key.unlimited_quota,
      allow_ips: key.allow_ips || '',
      expires_at: key.expires_at || '',
    })
  }

  const keys = keysData?.data || []
  const groups = groupsData?.data || []

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-6xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">API 密钥</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理用于 MCP 端点访问的 API Key</p>
        </div>
        <Button className="gap-2" onClick={() => { setShowCreate(true); setEditingKey(null); resetForm() }}>
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

      {/* Create / Edit form */}
      {(showCreate || editingKey) && (
        <div className="rounded-xl border bg-card p-5 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">{editingKey ? '编辑密钥' : '创建新密钥'}</h2>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => { setShowCreate(false); setEditingKey(null); resetForm() }}>
              <X className="h-4 w-4" />
            </Button>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>名称</Label>
              <Input placeholder="my-key" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>关联分组 (逗号分隔)</Label>
              <Input placeholder="group1, group2" value={form.groups} onChange={e => setForm({ ...form, groups: e.target.value })} />
              {groups.length > 0 && (
                <div className="flex flex-wrap gap-1.5 mt-1">
                  {groups.map((g: any) => (
                    <button
                      key={g.id}
                      type="button"
                      className="rounded bg-muted px-2 py-0.5 text-xs hover:bg-muted/80"
                      onClick={() => {
                        const slug = g.endpoint_slug || g.name
                        const current = form.groups ? form.groups.split(',').map(s => s.trim()) : []
                        if (!current.includes(slug)) {
                          setForm({ ...form, groups: [...current, slug].join(', ') })
                        }
                      }}
                    >
                      {g.display_name || g.name}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label>额度</Label>
              <div className="flex items-center gap-2">
                <Input
                  placeholder="不限"
                  type="number"
                  value={form.quota}
                  disabled={form.unlimited_quota}
                  onChange={e => setForm({ ...form, quota: e.target.value })}
                />
                <Button
                  type="button"
                  variant={form.unlimited_quota ? 'default' : 'outline'}
                  size="sm"
                  className="shrink-0 gap-1"
                  onClick={() => setForm({ ...form, unlimited_quota: !form.unlimited_quota })}
                >
                  <Infinity className="h-3.5 w-3.5" />
                  不限
                </Button>
              </div>
            </div>
            <div className="space-y-2">
              <Label>IP 白名单</Label>
              <Input placeholder="留空不限制" value={form.allow_ips} onChange={e => setForm({ ...form, allow_ips: e.target.value })} />
            </div>
            <div className="space-y-2">
              <Label>过期时间</Label>
              <Input
                type="datetime-local"
                value={form.expires_at ? form.expires_at.replace('Z', '').replace('T', 'T').slice(0, 16) : ''}
                onChange={e => {
                  const v = e.target.value
                  setForm({ ...form, expires_at: v ? new Date(v).toISOString() : '' })
                }}
              />
            </div>
          </div>

          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => { setShowCreate(false); setEditingKey(null); resetForm() }}>取消</Button>
            <Button
              disabled={!form.name.trim()}
              onClick={() => {
                if (editingKey) {
                  updateMutation.mutate({
                    id: editingKey.id,
                    body: {
                      name: form.name,
                      groups: form.groups ? form.groups.split(',').map(s => s.trim()) : [],
                      quota: form.unlimited_quota ? undefined : (parseInt(form.quota) || undefined),
                      unlimited_quota: form.unlimited_quota,
                      allow_ips: form.allow_ips || undefined,
                    },
                  })
                } else {
                  createMutation.mutate()
                }
              }}
            >
              {editingKey ? '保存' : '创建'}
            </Button>
          </div>
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
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">额度</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">状态</th>
                <th className="px-4 py-3 text-left font-medium text-muted-foreground">过期</th>
                <th className="px-4 py-3 text-right font-medium text-muted-foreground">操作</th>
              </tr>
            </thead>
            <tbody>
              {keys.map((key: ApiKeyListItem) => (
                <tr key={key.id} className="border-b last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-3 font-medium">{key.name}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-1.5">
                      <code className="text-xs font-mono text-muted-foreground">{key.key_prefix}...</code>
                      <button
                        className="text-muted-foreground hover:text-foreground transition-colors"
                        onClick={() => { navigator.clipboard.writeText(key.key_prefix + '***'); toast.success('前缀已复制') }}
                        title="复制前缀"
                      >
                        <Copy className="h-3 w-3" />
                      </button>
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {key.groups?.map(g => (
                        <span key={g} className="rounded bg-muted px-1.5 py-0.5 text-[10px]">{g}</span>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    {key.unlimited_quota ? (
                      <Badge variant="secondary" className="gap-1"><Infinity className="h-3 w-3" />不限</Badge>
                    ) : (
                      <span className="text-xs tabular-nums">{key.used_quota}/{key.quota}</span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <button onClick={() => toggleStatus(key)} className="transition-colors" title={key.status === 1 ? '点击禁用' : '点击启用'}>
                      {key.status === 1 ? (
                        <ToggleRight className="h-5 w-5 text-emerald-500" />
                      ) : (
                        <ToggleLeft className="h-5 w-5 text-zinc-400" />
                      )}
                    </button>
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">
                    {key.expires_at ? new Date(key.expires_at).toLocaleDateString('zh-CN') : '永不过期'}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => startEdit(key)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="sm" className="text-destructive" onClick={() => {
                        if (confirm(`确定删除密钥 "${key.name}"？`)) deleteMutation.mutate(key.id)
                      }}>
                        <Trash2 className="h-3.5 w-3.5" />
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
