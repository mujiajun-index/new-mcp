import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'
import { User, Activity, Key, Save, UserCog } from 'lucide-react'
import { AccountBindingsCard } from './account-bindings-card'

async function getProfile() {
  const res = await api.get('/auth/profile')
  return res.data
}

async function updateProfile(data: { display_name?: string }) {
  const res = await api.put('/auth/profile', data)
  return res.data
}

async function changePassword(data: { old_password: string; new_password: string }) {
  const res = await api.put('/auth/password', data)
  return res.data
}

export function SettingsPage() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const { data: profileData, isLoading } = useQuery({
    queryKey: ['profile'],
    queryFn: getProfile,
  })

  const profile = profileData?.data

  const [profileForm, setProfileForm] = useState({ display_name: '' })
  const [passwordForm, setPasswordForm] = useState({ old_password: '', new_password: '', confirm: '' })
  const [profileLoaded, setProfileLoaded] = useState(false)

  // Initialize form when profile loads
  if (profile && !profileLoaded) {
    setProfileForm({ display_name: profile.display_name || '' })
    setProfileLoaded(true)
  }

  const refreshProfile = () => queryClient.invalidateQueries({ queryKey: ['profile'] })

  const updateProfileMutation = useMutation({
    mutationFn: () => updateProfile({ display_name: profileForm.display_name || undefined }),
    onSuccess: () => {
      toast.success(t('settings.profileUpdated'))
      queryClient.invalidateQueries({ queryKey: ['profile'] })
    },
    // Error toasts are surfaced by the axios response interceptor.
  })

  const changePasswordMutation = useMutation({
    mutationFn: () => changePassword({
      old_password: passwordForm.old_password,
      new_password: passwordForm.new_password,
    }),
    onSuccess: () => {
      toast.success(t('settings.passwordChanged'))
      setPasswordForm({ old_password: '', new_password: '', confirm: '' })
    },
    onError: (err: any) => toast.error(err?.response?.data?.message || t('settings.passwordChangeFailed')),
  })

  const locale = i18n.language?.startsWith('zh') ? 'zh-CN' : 'en-US'

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t('nav.settings')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('settings.pageSubtitle')}</p>
      </div>

      {isLoading ? (
        <div className="text-sm text-muted-foreground text-center py-16">{t('common.loading')}</div>
      ) : (
        <>
          {/* Account Info */}
          <div className="rounded-xl border bg-card p-5 space-y-4">
            <div className="flex items-center gap-2">
              <User className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">{t('settings.accountInfo')}</h2>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <p className="text-xs text-muted-foreground mb-1">{t('settings.accountInfoUsername')}</p>
                <p className="text-sm font-medium">{profile?.username}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">{t('settings.accountInfoRole')}</p>
                {profile?.role === 'super_admin' ? <Badge variant="default">{t('settings.roleSuperAdmin')}</Badge> : profile?.role === 'admin' ? <Badge variant="default">{t('settings.roleAdmin')}</Badge> : <Badge variant="secondary">{t('settings.roleUser')}</Badge>}
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">{t('settings.accountInfoGroup')}</p>
                <p className="text-sm">{profile?.group || 'default'}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-1">{t('settings.accountInfoCreatedAt')}</p>
                <p className="text-sm">{profile?.created_at ? new Date(profile.created_at).toLocaleDateString(locale) : '-'}</p>
              </div>
            </div>
          </div>

          {/* Usage Stats */}
          <div className="rounded-xl border bg-card p-5 space-y-4">
            <div className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">{t('settings.usageStats')}</h2>
            </div>
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="rounded-lg bg-muted/50 p-3">
                <p className="text-xs text-muted-foreground">{t('settings.quotaRemaining')}</p>
                <p className="mt-1 text-xl font-semibold tabular-nums">{profile?.quota ?? 0}</p>
              </div>
              <div className="rounded-lg bg-muted/50 p-3">
                <p className="text-xs text-muted-foreground">{t('settings.quotaUsed')}</p>
                <p className="mt-1 text-xl font-semibold tabular-nums">{profile?.used_quota ?? 0}</p>
              </div>
              <div className="rounded-lg bg-muted/50 p-3">
                <p className="text-xs text-muted-foreground">{t('settings.totalCalls')}</p>
                <p className="mt-1 text-xl font-semibold tabular-nums">{profile?.request_count ?? 0}</p>
              </div>
            </div>
          </div>

          {/* Edit Profile (display name; email is bound via the Account Bindings card) */}
          <div className="rounded-xl border bg-card p-5 space-y-4">
            <div className="flex items-center gap-2">
              <UserCog className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">{t('settings.editProfile')}</h2>
            </div>
            <div className="space-y-2 max-w-sm">
              <label className="text-sm font-medium">{t('admin.users.displayName')}</label>
              <Input
                placeholder="display name"
                value={profileForm.display_name}
                onChange={e => setProfileForm({ ...profileForm, display_name: e.target.value })}
              />
            </div>
            <div className="flex justify-end">
              <Button className="gap-2" onClick={() => updateProfileMutation.mutate()} disabled={updateProfileMutation.isPending}>
                <Save className="h-4 w-4" />{t('common.save')}
              </Button>
            </div>
          </div>

          {/* Account Bindings (email + future OAuth providers) */}
          <AccountBindingsCard profile={profile} onUpdate={refreshProfile} />

          {/* Change Password */}
          <div className="rounded-xl border bg-card p-5 space-y-4">
            <div className="flex items-center gap-2">
              <Key className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">{t('settings.changePassword')}</h2>
            </div>
            <div className="space-y-3 max-w-sm">
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.currentPassword')}</label>
                <Input type="password" value={passwordForm.old_password} onChange={e => setPasswordForm({ ...passwordForm, old_password: e.target.value })} />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.newPassword')}</label>
                <Input type="password" value={passwordForm.new_password} onChange={e => setPasswordForm({ ...passwordForm, new_password: e.target.value })} />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">{t('settings.confirmNewPassword')}</label>
                <Input type="password" value={passwordForm.confirm} onChange={e => setPasswordForm({ ...passwordForm, confirm: e.target.value })} />
              </div>
            </div>
            <div className="flex justify-end">
              <Button
                className="gap-2"
                disabled={!passwordForm.old_password || !passwordForm.new_password || passwordForm.new_password !== passwordForm.confirm || changePasswordMutation.isPending}
                onClick={() => changePasswordMutation.mutate()}
              >
                <Save className="h-4 w-4" />{t('settings.changePassword')}
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
