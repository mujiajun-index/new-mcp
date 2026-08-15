import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getAdminUsers, createAdminUser, updateAdminUser, getAdminUserDetail, adjustUserQuota, deleteAdminUser } from '@/features/admin/api'
import type { AdminUserDetail } from '@/types'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from '@/components/ui/dialog'
import { Tooltip, TooltipContent, TooltipTrigger, TooltipProvider } from '@/components/ui/tooltip'
import { Progress } from '@/components/ui/progress'
import { MobileListCard } from '@/components/ui/mobile-list-card'
import { useIsMobile } from '@/hooks/use-mobile'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { cn } from '@/lib/utils'
import { formatQuotaCurrency } from '@/lib/billing'
import { toast } from 'sonner'
import { Plus, Pencil, Search, ChevronLeft, ChevronRight, X, Eye, Scale, Trash2 } from 'lucide-react'

export function AdminUsersPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useIsMobile()
  const groupOptions = useSystemConfigStore((s) => s.config.userGroupOptions)
  const quotaPerUnit = useSystemConfigStore((s) => s.config.quotaPerUnit)
  const displayCurrency = useSystemConfigStore((s) => s.config.displayCurrency)
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [roleFilter, setRoleFilter] = useState<string>('all')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const pageSize = 20

  // 货币符号 + 调额快捷金额(按 quotaPerUnit 换算成单位额度)
  const currencySymbol = displayCurrency === 'USD' ? '$' : displayCurrency === 'EUR' ? '€' : '¥'
  const presetAmounts = [1, 5, 10, 50, 100, 500]
  const fmtMoney = (q: number) => formatQuotaCurrency(q, quotaPerUnit, displayCurrency)

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

  // 调额对话框(D13)
  const [quotaUser, setQuotaUser] = useState<any>(null)
  const [quotaForm, setQuotaForm] = useState<{ mode: 'add' | 'sub' | 'set'; value: string; remark: string }>({
    mode: 'add', value: '', remark: '',
  })
  const [quotaPreset, setQuotaPreset] = useState('')

  // 删除确认对话框
  const [deleteUser, setDeleteUser] = useState<any>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-users', page, keyword, roleFilter, statusFilter],
    queryFn: () => getAdminUsers({
      page, page_size: pageSize, keyword,
      role: roleFilter !== 'all' ? roleFilter : undefined,
      status: statusFilter !== 'all' ? parseInt(statusFilter) : undefined,
    }),
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

  const adjustQuotaMutation = useMutation({
    mutationFn: (data: { id: number; body: any }) => adjustUserQuota(data.id, data.body),
    onSuccess: (res) => {
      toast.success(t('admin.users.adjustSuccess', { quota: fmtMoney(res?.data?.new_quota ?? 0) }))
      setQuotaUser(null)
      setQuotaForm({ mode: 'add', value: '', remark: '' })
      setQuotaPreset('')
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
    onError: (err: any) => toast.error(err?.response?.data?.message || t('admin.users.adjustFailed')),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteAdminUser(id),
    onSuccess: () => {
      toast.success(t('admin.users.deleteSuccess'))
      setDeleteUser(null)
      queryClient.invalidateQueries({ queryKey: ['admin-users'] })
    },
    onError: (err: any) => toast.error(err?.response?.data?.message || t('admin.users.deleteFailed')),
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

  const startAdjustQuota = (user: any) => {
    setQuotaUser(user)
    setQuotaForm({ mode: 'add', value: '', remark: '' })
    setQuotaPreset('')
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

  // Remaining quota color: ≤10% rose, ≤30% amber, else emerald (matches reference/new-api users-columns).
  const quotaColor = (pct: number) =>
    pct <= 10
      ? '[&_[data-slot=progress-indicator]]:bg-rose-500'
      : pct <= 30
        ? '[&_[data-slot=progress-indicator]]:bg-amber-500'
        : '[&_[data-slot=progress-indicator]]:bg-emerald-500'

  // quotaCell: remain=user.quota, used=user.used_quota, total=used+remain。
  // 按 quotaPerUnit 换算成货币单位展示(对齐 reference/new-api formatQuota);
  // tooltip 同时给出货币与原始 quota 整数,保留审计精度。进度条按 remain/total 着色。
  const quotaCell = (u: { quota: number; used_quota: number }, width = 'w-[150px]') => {
    const remain = u.quota ?? 0
    const used = u.used_quota ?? 0
    const total = used + remain
    const pct = total > 0 ? (remain / total) * 100 : 0
    if (total === 0) {
      return <Badge variant="secondary">{t('admin.users.noQuota')}</Badge>
    }
    return (
      <TooltipProvider delayDuration={0}>
        <Tooltip>
          <TooltipTrigger asChild>
            <div className={`${width} cursor-help space-y-1`}>
              <div className="flex justify-between text-xs">
                <span className="font-medium tabular-nums">{fmtMoney(remain)}</span>
                <span className="text-muted-foreground tabular-nums">{fmtMoney(total)}</span>
              </div>
              <Progress value={pct} className={cn('h-1.5', quotaColor(pct))} />
            </div>
          </TooltipTrigger>
          <TooltipContent>
            <div className="space-y-1 text-xs">
              <div>{t('admin.users.usedQuota')}: {fmtMoney(used)} ({used.toLocaleString()})</div>
              <div>{t('admin.users.remainingQuota')}: {fmtMoney(remain)} ({remain.toLocaleString()})</div>
              <div>{t('admin.users.totalQuota')}: {fmtMoney(total)} ({total.toLocaleString()})</div>
              <div>{t('admin.users.percentage')}: {pct.toFixed(1)}%</div>
            </div>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
  }

  // 超级管理员（id=1）的角色与状态不可修改：编辑时角色以只读徽章展示、状态下拉禁用。
  // 普通管理员在列表中看不到超级管理员，故此处只影响超级管理员编辑自己的情形。
  const isProtectedTarget = editingUser?.id === 1 || editingUser?.role === 'super_admin'

  // 分组下拉的候选项：来自系统设置的「用户分组选项值」，若当前表单值不在列表中则补上，
  // 避免已有值在 UI 上被清空（编辑旧数据或新建时默认值被移除的情形）。
  const selectableGroups = (() => {
    const list = [...groupOptions]
    const current = form.group || editingUser?.group || ''
    if (current && !list.includes(current)) {
      list.push(current)
    }
    return list
  })()

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

      {/* Search & filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 min-w-[200px] max-w-xs">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('admin.users.searchPlaceholder')}
            value={keyword}
            onChange={e => { setKeyword(e.target.value); setPage(1) }}
            className="pl-9 h-9"
          />
        </div>
        <Select value={roleFilter} onValueChange={(v) => { setRoleFilter(v); setPage(1) }}>
          <SelectTrigger className="h-9 w-[140px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('admin.users.filterAllRoles')}</SelectItem>
            <SelectItem value="user">{t('admin.users.badgeUser')}</SelectItem>
            <SelectItem value="admin">{t('admin.users.badgeAdmin')}</SelectItem>
            <SelectItem value="super_admin">{t('admin.users.badgeSuperAdmin')}</SelectItem>
          </SelectContent>
        </Select>
        <Select value={statusFilter} onValueChange={(v) => { setStatusFilter(v); setPage(1) }}>
          <SelectTrigger className="h-9 w-[140px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('admin.users.filterAllStatuses')}</SelectItem>
            <SelectItem value="1">{t('admin.users.badgeEnabled')}</SelectItem>
            <SelectItem value="2">{t('admin.users.badgeDisabled')}</SelectItem>
          </SelectContent>
        </Select>
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
            {!editingUser && (
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('admin.users.quota')}</label>
                <Input type="number" placeholder="0" value={form.quota} onChange={e => setForm({ ...form, quota: e.target.value })} />
              </div>
            )}
            <div className="space-y-2">
              <label className="text-sm font-medium">{t('admin.users.groups')}</label>
              <Select value={form.group} onValueChange={v => setForm({ ...form, group: v })}>
                <SelectTrigger className="h-9"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {selectableGroups.map(g => (
                    <SelectItem key={g} value={g}>{g}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
            <div className="space-y-3">
              <p className="text-xs text-muted-foreground">{t('admin.users.quota')}</p>
              <div>{quotaCell(detailUser, 'w-[200px]')}</div>
            </div>
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
                  { label: t('admin.users.table.quota'), value: quotaCell(user, 'w-[180px]') },
                  { label: t('admin.users.table.calls'), value: <span className="tabular-nums">{user.request_count}</span> },
                  { label: t('admin.users.email'), value: user.email || '-' },
                  { label: t('admin.users.groups'), value: user.group || '-' },
                ]}
                actions={
                  <div className="flex items-center gap-1">
                    <Button variant="ghost" size="sm" onClick={() => startDetail(user)}>
                      <Eye className="h-3.5 w-3.5" />{t('admin.users.detail')}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => startAdjustQuota(user)}>
                      <Scale className="h-3.5 w-3.5" />{t('admin.users.adjustQuota')}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => startEdit(user)}>
                      <Pencil className="h-3.5 w-3.5" />{t('common.edit')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive"
                      disabled={user.id === 1 || user.role === 'super_admin'}
                      onClick={() => setDeleteUser(user)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />{t('admin.users.delete')}
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
                  <TableCell>{quotaCell(user)}</TableCell>
                  <TableCell className="text-sm tabular-nums">{user.request_count}</TableCell>
                  <TableCell>{statusLabel(user.status)}</TableCell>
                  <TableCell className="text-sm">{user.group}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => startDetail(user)} title={t('admin.users.detail')}>
                        <Eye className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => startAdjustQuota(user)} title={t('admin.users.adjustQuota')}>
                        <Scale className="h-3.5 w-3.5" />
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => startEdit(user)} title={t('common.edit')}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        disabled={user.id === 1 || user.role === 'super_admin'}
                        onClick={() => setDeleteUser(user)}
                        title={t('admin.users.delete')}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
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

      {/* Quota adjust dialog (D13) */}
      <Dialog open={!!quotaUser} onOpenChange={(v) => !v && setQuotaUser(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('admin.users.adjustQuotaTitle')}</DialogTitle>
            <DialogDescription>
              {quotaUser?.username} — {t('admin.users.currentQuota')}: {fmtMoney(quotaUser?.quota ?? 0)} · {t('admin.users.currentUsed')}: {fmtMoney(quotaUser?.used_quota ?? 0)}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t('admin.users.adjustMode')}</Label>
              <Select value={quotaForm.mode} onValueChange={(v) => setQuotaForm({ ...quotaForm, mode: v as 'add' | 'sub' | 'set' })}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="add">{t('admin.users.modeAdd')}</SelectItem>
                  <SelectItem value="sub">{t('admin.users.modeSub')}</SelectItem>
                  <SelectItem value="set">{t('admin.users.modeSet')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {quotaForm.mode === 'add' && (
              <div className="space-y-2">
                <Label>{t('admin.users.adjustPreset')}</Label>
                <Select
                  value={quotaPreset}
                  onValueChange={(v) => {
                    setQuotaPreset(v)
                    const amt = parseInt(v, 10)
                    if (!Number.isNaN(amt)) {
                      setQuotaForm((f) => ({ ...f, value: String(amt * quotaPerUnit) }))
                    }
                  }}
                >
                  <SelectTrigger><SelectValue placeholder={t('admin.users.adjustPresetPlaceholder')} /></SelectTrigger>
                  <SelectContent>
                    {presetAmounts.map((amt) => (
                      <SelectItem key={amt} value={String(amt)}>
                        {currencySymbol}{amt} = {(amt * quotaPerUnit).toLocaleString()}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  {t('admin.users.adjustPresetHint', { unit: quotaPerUnit.toLocaleString() })}
                </p>
              </div>
            )}
            <div className="space-y-2">
              <Label>{t('admin.users.adjustValue')} <span className="text-destructive">*</span></Label>
              <Input
                type="number"
                placeholder={t('admin.users.adjustValue')}
                value={quotaForm.value}
                onChange={(e) => setQuotaForm({ ...quotaForm, value: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>{t('admin.users.adjustRemark')}</Label>
              <Input
                placeholder={t('admin.users.adjustRemarkPlaceholder')}
                value={quotaForm.remark}
                onChange={(e) => setQuotaForm({ ...quotaForm, remark: e.target.value })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setQuotaUser(null)}>{t('common.cancel')}</Button>
            <Button
              disabled={adjustQuotaMutation.isPending || !quotaForm.value || parseInt(quotaForm.value) <= 0}
              onClick={() => adjustQuotaMutation.mutate({
                id: quotaUser.id,
                body: {
                  mode: quotaForm.mode,
                  value: parseInt(quotaForm.value),
                  remark: quotaForm.remark || undefined,
                },
              })}
            >
              {t('admin.users.adjustConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirm dialog */}
      <Dialog open={!!deleteUser} onOpenChange={(v) => !v && setDeleteUser(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('admin.users.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {t('admin.users.deleteDesc', { username: deleteUser?.username })}
            </DialogDescription>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">{t('admin.users.deleteWarning')}</p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteUser(null)}>{t('common.cancel')}</Button>
            <Button
              variant="destructive"
              disabled={deleteMutation.isPending}
              onClick={() => deleteUser && deleteMutation.mutate(deleteUser.id)}
            >
              {t('admin.users.deleteConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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
