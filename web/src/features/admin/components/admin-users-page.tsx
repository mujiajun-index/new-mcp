import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getAdminUsers, createAdminUser, updateAdminUser } from '@/features/admin/api'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from 'sonner'
import { Users, Plus, Pencil, Search, ChevronLeft, ChevronRight, X } from 'lucide-react'

export function AdminUsersPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const pageSize = 20

  const [showCreate, setShowCreate] = useState(false)
  const [editingUser, setEditingUser] = useState<any>(null)
  const [form, setForm] = useState({
    username: '',
    password: '',
    email: '',
    display_name: '',
    role: 'user',
    quota: '',
    group: 'default',
    remark: '',
    status: 1,
  })

  const { data, isLoading } = useQuery({
    queryKey: ['admin-users', page, keyword],
    queryFn: () => getAdminUsers({ page, page_size: pageSize, keyword }),
  })

  const users = data?.data ?? []
  const pagination = data?.pagination
  const totalPages = pagination?.total_pages ?? 1

  const resetForm = () => setForm({ username: '', password: '', email: '', display_name: '', role: 'user', quota: '', group: 'default', remark: '', status: 1 })

  const createMutation = useMutation({
    mutationFn: () => createAdminUser({
      username: form.username,
      password: form.password,
      email: form.email || undefined,
      display_name: form.display_name || undefined,
      role: form.role || undefined,
      quota: form.quota ? parseInt(form.quota) : undefined,
      group: form.group || undefined,
    }),
    onSuccess: () => {
      toast.success('用户创建成功')
      setShowCreate(false)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
    onError: (err: any) => toast.error(err?.response?.data?.message || '创建失败'),
  })

  const updateMutation = useMutation({
    mutationFn: (data: { id: number; body: any }) => updateAdminUser(data.id, data.body),
    onSuccess: () => {
      toast.success('更新成功')
      setEditingUser(null)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
    onError: () => toast.error('更新失败'),
  })

  const startEdit = (user: any) => {
    setEditingUser(user)
    setShowCreate(false)
    setForm({
      username: user.username || '',
      password: '',
      email: user.email || '',
      display_name: user.display_name || '',
      role: user.role || 'user',
      quota: String(user.quota ?? ''),
      group: user.group || 'default',
      remark: user.remark || '',
      status: user.status ?? 1,
    })
  }

  const startCreate = () => {
    setShowCreate(true)
    setEditingUser(null)
    resetForm()
  }

  const roleLabel = (role: string) => {
    switch (role) {
      case 'admin': return <Badge variant="default">管理员</Badge>
      default: return <Badge variant="secondary">用户</Badge>
    }
  }

  const statusLabel = (status: number) => {
    switch (status) {
      case 1: return <Badge variant="success">启用</Badge>
      default: return <Badge variant="destructive">禁用</Badge>
    }
  }

  return (
    <div className="p-6 lg:p-8 space-y-6 max-w-[1400px]">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.adminUsers')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理系统用户账号</p>
        </div>
        <Button className="gap-2" onClick={startCreate}>
          <Plus className="h-4 w-4" />创建用户
        </Button>
      </div>

      {/* Search */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="搜索用户名/邮箱..."
            value={keyword}
            onChange={e => { setKeyword(e.target.value); setPage(1) }}
            className="pl-9 h-9"
          />
        </div>
      </div>

      {/* Create / Edit form */}
      {(showCreate || editingUser) && (
        <div className="rounded-xl border bg-card p-5 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">{editingUser ? '编辑用户' : '创建新用户'}</h2>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => { setShowCreate(false); setEditingUser(null); resetForm() }}>
              <X className="h-4 w-4" />
            </Button>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {!editingUser && (
              <div className="space-y-2">
                <label className="text-sm font-medium">用户名</label>
                <Input placeholder="username" value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} />
              </div>
            )}
            <div className="space-y-2">
              <label className="text-sm font-medium">{editingUser ? '重置密码' : '密码'}</label>
              <Input type="password" placeholder={editingUser ? '留空不修改' : 'password'} value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">显示名称</label>
              <Input placeholder="display name" value={form.display_name} onChange={e => setForm({ ...form, display_name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">邮箱</label>
              <Input type="email" placeholder="email" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">角色</label>
              <Select value={form.role} onValueChange={v => setForm({ ...form, role: v })}>
                <SelectTrigger className="h-9"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="user">普通用户</SelectItem>
                  <SelectItem value="admin">管理员</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">额度</label>
              <Input type="number" placeholder="0" value={form.quota} onChange={e => setForm({ ...form, quota: e.target.value })} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">分组</label>
              <Input placeholder="default" value={form.group} onChange={e => setForm({ ...form, group: e.target.value })} />
            </div>
            {editingUser && (
              <div className="space-y-2">
                <label className="text-sm font-medium">状态</label>
                <Select value={String(form.status)} onValueChange={v => setForm({ ...form, status: parseInt(v) })}>
                  <SelectTrigger className="h-9"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="1">启用</SelectItem>
                    <SelectItem value="2">禁用</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="space-y-2">
              <label className="text-sm font-medium">备注</label>
              <Input placeholder="remark" value={form.remark} onChange={e => setForm({ ...form, remark: e.target.value })} />
            </div>
          </div>

          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => { setShowCreate(false); setEditingUser(null); resetForm() }}>取消</Button>
            <Button
              disabled={editingUser ? updateMutation.isPending : (!form.username.trim() || !form.password.trim())}
              onClick={() => {
                if (editingUser) {
                  const body: any = {
                    display_name: form.display_name || undefined,
                    email: form.email || undefined,
                    role: form.role,
                    quota: form.quota ? parseInt(form.quota) : undefined,
                    group: form.group || undefined,
                    remark: form.remark || undefined,
                    status: form.status,
                  }
                  if (form.password) body.password = form.password
                  updateMutation.mutate({ id: editingUser.id, body })
                } else {
                  createMutation.mutate()
                }
              }}
            >
              {editingUser ? '保存' : '创建'}
            </Button>
          </div>
        </div>
      )}

      {/* Table */}
      <div className="rounded-xl border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>用户名</TableHead>
              <TableHead>显示名</TableHead>
              <TableHead>邮箱</TableHead>
              <TableHead>角色</TableHead>
              <TableHead>额度</TableHead>
              <TableHead>已用</TableHead>
              <TableHead>调用</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>分组</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow><TableCell colSpan={11} className="h-32 text-center text-muted-foreground">加载中...</TableCell></TableRow>
            ) : users.length === 0 ? (
              <TableRow><TableCell colSpan={11} className="h-32 text-center text-muted-foreground">无数据</TableCell></TableRow>
            ) : (
              users.map((user: any) => (
                <TableRow key={user.id}>
                  <TableCell className="text-xs text-muted-foreground tabular-nums">{user.id}</TableCell>
                  <TableCell className="font-medium">{user.username}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{user.display_name || '-'}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{user.email || '-'}</TableCell>
                  <TableCell>{roleLabel(user.role)}</TableCell>
                  <TableCell className="text-sm tabular-nums">{user.quota}</TableCell>
                  <TableCell className="text-sm tabular-nums">{user.used_quota}</TableCell>
                  <TableCell className="text-sm tabular-nums">{user.request_count}</TableCell>
                  <TableCell>{statusLabel(user.status)}</TableCell>
                  <TableCell className="text-sm">{user.group}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="sm" onClick={() => startEdit(user)}>
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      {pagination && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">共 {pagination.total} 条</p>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <span className="text-sm tabular-nums">{page} / {totalPages}</span>
            <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
