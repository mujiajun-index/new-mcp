import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'
import { User, Mail, Key, Activity, Save } from 'lucide-react'

async function getProfile() {
  const res = await api.get('/auth/profile')
  return res.data
}

async function updateProfile(data: { display_name?: string; email?: string; avatar_url?: string }) {
  const res = await api.put('/auth/profile', data)
  return res.data
}

async function changePassword(data: { old_password: string; new_password: string }) {
  const res = await api.put('/auth/password', data)
  return res.data
}

export function SettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: profileData, isLoading } = useQuery({
    queryKey: ['profile'],
    queryFn: getProfile,
  })

  const profile = profileData?.data

  const [profileForm, setProfileForm] = useState({ display_name: '', email: '' })
  const [passwordForm, setPasswordForm] = useState({ old_password: '', new_password: '', confirm: '' })
  const [profileLoaded, setProfileLoaded] = useState(false)

  // Initialize form when profile loads
  if (profile && !profileLoaded) {
    setProfileForm({ display_name: profile.display_name || '', email: profile.email || '' })
    setProfileLoaded(true)
  }

  const updateProfileMutation = useMutation({
    mutationFn: () => updateProfile({
      display_name: profileForm.display_name || undefined,
      email: profileForm.email || undefined,
    }),
    onSuccess: () => {
      toast.success('个人资料已更新')
      queryClient.invalidateQueries({ queryKey: ['profile'] })
    },
    onError: () => toast.error('更新失败'),
  })

  const changePasswordMutation = useMutation({
    mutationFn: () => changePassword({
      old_password: passwordForm.old_password,
      new_password: passwordForm.new_password,
    }),
    onSuccess: () => {
      toast.success('密码已修改')
      setPasswordForm({ old_password: '', new_password: '', confirm: '' })
    },
    onError: (err: any) => toast.error(err?.response?.data?.message || '密码修改失败'),
  })

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('nav.settings')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">管理您的账号信息和安全设置</p>
      </div>

      {isLoading ? (
        <div className="text-sm text-muted-foreground text-center py-16">加载中...</div>
      ) : (
        <>
          {/* Account Info */}
          <div className="rounded-xl border bg-card p-5 space-y-4">
            <div className="flex items-center gap-2">
              <User className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">账号信息</h2>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <p className="text-xs text-muted-foreground mb-1">用户名</p>
                <p className="text-sm font-medium">{profile?.username}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">角色</p>
                {profile?.role === 'admin' ? <Badge variant="default">管理员</Badge> : <Badge variant="secondary">用户</Badge>}
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">分组</p>
                <p className="text-sm">{profile?.group || 'default'}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">注册时间</p>
                <p className="text-sm">{profile?.created_at ? new Date(profile.created_at).toLocaleDateString('zh-CN') : '-'}</p>
              </div>
            </div>
          </div>

          {/* Usage Stats */}
          <div className="rounded-xl border bg-card p-5 space-y-4">
            <div className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">用量统计</h2>
            </div>
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="rounded-lg bg-muted/50 p-3">
                <p className="text-xs text-muted-foreground">剩余额度</p>
                <p className="mt-1 text-xl font-semibold tabular-nums">{profile?.quota ?? 0}</p>
              </div>
              <div className="rounded-lg bg-muted/50 p-3">
                <p className="text-xs text-muted-foreground">已用额度</p>
                <p className="mt-1 text-xl font-semibold tabular-nums">{profile?.used_quota ?? 0}</p>
              </div>
              <div className="rounded-lg bg-muted/50 p-3">
                <p className="text-xs text-muted-foreground">总调用次数</p>
                <p className="mt-1 text-xl font-semibold tabular-nums">{profile?.request_count ?? 0}</p>
              </div>
            </div>
          </div>

          {/* Edit Profile */}
          <div className="rounded-xl border bg-card p-5 space-y-4">
            <div className="flex items-center gap-2">
              <Mail className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">编辑资料</h2>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <label className="text-sm font-medium">显示名称</label>
                <Input
                  placeholder="display name"
                  value={profileForm.display_name}
                  onChange={e => setProfileForm({ ...profileForm, display_name: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">邮箱</label>
                <Input
                  type="email"
                  placeholder="email"
                  value={profileForm.email}
                  onChange={e => setProfileForm({ ...profileForm, email: e.target.value })}
                />
              </div>
            </div>
            <div className="flex justify-end">
              <Button className="gap-2" onClick={() => updateProfileMutation.mutate()} disabled={updateProfileMutation.isPending}>
                <Save className="h-4 w-4" />保存
              </Button>
            </div>
          </div>

          {/* Change Password */}
          <div className="rounded-xl border bg-card p-5 space-y-4">
            <div className="flex items-center gap-2">
              <Key className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">修改密码</h2>
            </div>
            <div className="space-y-3 max-w-sm">
              <div className="space-y-2">
                <label className="text-sm font-medium">当前密码</label>
                <Input type="password" value={passwordForm.old_password} onChange={e => setPasswordForm({ ...passwordForm, old_password: e.target.value })} />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">新密码</label>
                <Input type="password" value={passwordForm.new_password} onChange={e => setPasswordForm({ ...passwordForm, new_password: e.target.value })} />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">确认新密码</label>
                <Input type="password" value={passwordForm.confirm} onChange={e => setPasswordForm({ ...passwordForm, confirm: e.target.value })} />
              </div>
            </div>
            <div className="flex justify-end">
              <Button
                className="gap-2"
                disabled={!passwordForm.old_password || !passwordForm.new_password || passwordForm.new_password !== passwordForm.confirm || changePasswordMutation.isPending}
                onClick={() => changePasswordMutation.mutate()}
              >
                <Save className="h-4 w-4" />修改密码
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
