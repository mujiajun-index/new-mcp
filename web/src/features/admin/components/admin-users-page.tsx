import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getAdminUsers, createAdminUser, updateAdminUser, getAdminUserDetail } from '@/features/admin/api'
import type { AdminUserDetail } from '@/types'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { toast } from 'sonner'
import { Plus, Pencil, Search, ChevronLeft, ChevronRight, X, Eye } from 'lucide-react'

export function AdminUsersPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const pageSize = 20

  const [showCreate, setShowCreate] = useState(false)
  const [editingUser, setEditingUser] = useState<any>(null)
  const [detailUser, setDetailUser] = useState<AdminUserDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
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
      toast.success(t('admin.users.createSuccess'))
      setShowCreate(false)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
    onError: (err: any) => toast.error(err?.response?.data?.message || t('admin.users.createFailed')),
  })

  const updateMutation = useMutation({
    mutationFn: (data: { id: number; body: any }) => updateAdminUser(data.id, data.body),
    onSuccess: () => {
      toast.success(t('admin.users.updateSuccess'))
      setEditingUser(null)
      resetForm()
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
    onError: (err: any) => toast.error(err?.response?.data?.message || t('admin.users.updateFailed')),
  })

  const startEdit = (user: any) => {
    setEditingUser(user)
    setDetailUser(null)
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
    setDetailUser(null)
    resetForm()
  }

  const startDetail = async (user: any) => {
    if (detailLoading) return
    setShowCreate(false)
    setEditingUser(null)
    setDetailLoading(true)
    try {
      const res = await getAdminUserDetail(user.id)
      setDetailUser(res?.data ?? null)
    } catch {
      // 错误由 axios 拦截器统一提示
    } finally {
      setDetailLoading(false)
    }
  }

  const fmtTime = (s?: string) => (s ? new Date(s).toLocaleString() : t('admin.users.never'))

  const roleLabel = (role: string) => {
    switch (role) {
      case 'super_admin': return <Badge variant="default">{t('admin.users.badgeSuperAdmin')}</Badge>
      case 'admin': return <Badge variant="default">{t('admin.users.badgeAdmin')}</Badge>
      default: return <Badge variant="secondary">{t('admin.users.badgeUser')}</Badge>
    }
  }

  const statusLabel = (status: number) => {
    switch (status) {
      case 1: return <Badge variant="success">{t('admin.users.badgeEnabled')}</Badge>
      default: return <Badge variant="destructive">{t('admin.users.badgeDisabled')}</Badge>
    }
  }

  // 超级管理员（id=1）的角色与状态不可修改：编辑时角色以只读徽章展示、状态下拉禁用。
  // 普通管理员在列表中看不到超级管理员，故此处只影响超级管理员编辑自己的情形。
  const isProtectedTarget = editingUser?.id === 1 || editingUser?.role === 'super_admin'

  return (
    <div className="space-y-6 p-4 sm:p-6 lg:p-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('nav.adminUsers')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('admin.users.subtitle')}</p>
        </div>
        <Button className="gap-2" onClick={startCreate}>
          <Plus className="h-4 w-4" />{t('admin.users.create')}
        </Button>
      </div>

      {/* Search */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('admin.users.searchPlaceholder')}
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
            <h2 className="text-sm font-semibold">{editingUser ? t('admin.users.editTitle') : t('admin.users.createTitle')}</h2>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => { setShowCreate(false); setEditingUser(null); resetForm() }}>
              <X className="h-4 w-4" />
            </Button>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('admin.users.username')}</label>
              {editingUser ? (
                <Input value={editingUser.username || ''} disabled readOnly />
              ) : (
                <Input placeholder="username" value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} />
              )}
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{editingUser ? t('admin.users.resetPassword') : t('admin.users.password')}</label>
              <Input type="password" placeholder={editingUser ? t('admin.users.keepPasswordUnchanged') : 'password'} value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('admin.users.displayName')}</label>
              <Input placeholder="display name" value={form.display_name} onChange={e => setForm({ ...form, display_name: e.target.value })} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('admin.users.email')}</label>
              <Input type="email" placeholder="email" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('admin.users.role')}</label>
              {isProtectedTarget ? (
                <div className="h-9 flex items-center"><Badge variant="default">{t('admin.users.badgeSuperAdmin')}</Badge></div>
              ) : (
                <Select value={form.role} onValueChange={v => setForm({ ...form, role: v })}>
                  <SelectTrigger className="h-9"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">{t('admin.users.user')}</SelectItem>
                    <SelectItem value="admin">{t('admin.users.admin')}</SelectItem>
                  </SelectContent>
                </Select>
              )}
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('admin.users.quota')}</label>
              <Input type="number" placeholder="0" value={form.quota} onChange={e => setForm({ ...form, quota: e.target.value })} />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('admin.users.groups')}</label>
              <Input placeholder="default" value={form.group} onChange={e => setForm({ ...form, group: e.target.value })} />
            </div>
            {editingUser && (
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('admin.users.status')}</label>
                <Select value={String(form.status)} onValueChange={v => setForm({ ...form, status: parseInt(v) })} disabled={isProtectedTarget}>
                  <SelectTrigger className="h-9"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="1">{t('admin.users.badgeEnabled')}</SelectItem>
                    <SelectItem value="2">{t('admin.users.badgeDisabled')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('admin.users.remark')}</label>
              <Input placeholder="remark" value={form.remark} onChange={e => setForm({ ...form, remark: e.target.value })} />
            </div>
          </div>

          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => { setShowCreate(false); setEditingUser(null); resetForm() }}>{t('common.cancel')}</Button>
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
              {editingUser ? t('common.save') : t('common.create')}
            </Button>
          </div>
        </div>
      )}

      {/* Detail panel (read-only, admin-only audit fields) */}
      {detailUser && (
        <div className="rounded-xl border bg-card p-5 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">{t('admin.users.detailTitle')}</h2>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setDetailUser(null)}>
              <X className="h-4 w-4" />
            </Button>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <DetailField label={t('admin.users.username')} value={detailUser.username} />
            <DetailField label={t('admin.users.displayName')} value={detailUser.display_name || '-'} />
            <DetailField label={t('admin.users.email')} value={detailUser.email || '-'} />
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">{t('admin.users.role')}</p>
              <div>{roleLabel(detailUser.role)}</div>
            </div>
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">{t('admin.users.status')}</p>
              <div>{statusLabel(detailUser.status)}</div>
            </div>
            <DetailField label={t('admin.users.groups')} value={detailUser.group || '-'} />
            <DetailField label={t('admin.users.quota')} value={`${detailUser.used_quota} / ${detailUser.quota}`} />
            <DetailField label={t('admin.users.table.calls')} value={String(detailUser.request_count)} />
            <DetailField label={t('admin.users.remark')} value={detailUser.remark || '-'} />
            <DetailField label={t('admin.users.registerTime')} value={fmtTime(detailUser.created_at)} />
            <DetailField label={t('admin.users.registerIp')} value={detailUser.register_ip || '-'} />
            <DetailField label={t('admin.users.lastLoginAt')} value={fmtTime(detailUser.last_login_at)} />
            <DetailField label={t('admin.users.lastLoginIp')} value={detailUser.last_login_ip || '-'} />
          </div>
        </div>
      )}

      {/* Table */}
      <div className="rounded-xl border bg-card">
        {isLoading ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.loading')}</div>
        ) : users.length === 0 ? (
          <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">{t('common.noData')}</div>
        ) : isMobile ? (
          <div className="divide-y">
            {users.map((user: any) => (
              <MobileListCard
                key={user.id}
                title={
                  <div className="flex flex-col">
                    <span className="font-medium">{user.username}</span>
                    {user.display_name && (
                      <span className="text-xs text-muted-foreground">{user.display_name}</span>
                    )}
                  </div>
                }
                badge={
                  <div className="flex flex-col items-end gap-1">
                    {roleLabel(user.role)}
                    {statusLabel(user.status)}
                  </div>
                }
                meta={[
                  { label: t('admin.users.table.quota'), value: <span className="tabular-nums">{user.used_quota}/{user.quota}</span> },
                  { label: t('admin.users.table.calls'), value: <span className="tabular-nums">{user.request_count}</span> },
                  { label: t('admin.users.email'), value: user.email || '-' },
                  { label: t('admin.users.groups'), value: user.group || '-' },
                ]}
                actions={
                  <div className="flex items-center gap-1">
                    <Button variant="ghost" size="sm" onClick={() => startDetail(user)}>
                      <Eye className="h-3.5 w-3.5" />{t('admin.users.detail')}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => startEdit(user)}>
                      <Pencil className="h-3.5 w-3.5" />{t('common.edit')}
                    </Button>
                  </div>
                }
              />
            ))}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>{t('admin.users.table.username')}</TableHead>
                <TableHead>{t('admin.users.table.displayName')}</TableHead>
                <TableHead>{t('admin.users.table.email')}</TableHead>
                <TableHead>{t('admin.users.table.role')}</TableHead>
                <TableHead>{t('admin.users.table.quota')}</TableHead>
                <TableHead>{t('admin.users.table.used')}</TableHead>
                <TableHead>{t('admin.users.table.calls')}</TableHead>
                <TableHead>{t('admin.users.table.status')}</TableHead>
                <TableHead>{t('admin.users.table.groups')}</TableHead>
                <TableHead className="text-right">{t('admin.users.table.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user: any) => (
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
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => startDetail(user)} title={t('admin.users.detail')}>
                        <Eye className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => startEdit(user)} title={t('common.edit')}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      {/* Pagination */}
      {pagination && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">{t('admin.users.total', { count: pagination.total })}</p>
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

function DetailField({ label, value }: { label: string; value: string }) {
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-sm font-medium break-all">{value}</p>
    </div>
  )
}
