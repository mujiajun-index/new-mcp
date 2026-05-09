import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { Loader2, Shield } from 'lucide-react'
import { toast } from 'sonner'
import { submitSetup } from './api'
import { markSetupDone } from '@/lib/setup-check'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function SetupPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [form, setForm] = useState({
    username: '',
    password: '',
    confirmPassword: '',
  })

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!form.username.trim()) {
      toast.error(t('setup.usernameRequired'))
      return
    }
    if (form.password.length < 8) {
      toast.error(t('setup.passwordMinLength'))
      return
    }
    if (form.password !== form.confirmPassword) {
      toast.error(t('setup.passwordMismatch'))
      return
    }

    setLoading(true)
    try {
      const res = await submitSetup({
        username: form.username.trim(),
        password: form.password,
        confirm_password: form.confirmPassword,
      })
      if (res.success) {
        markSetupDone()
        toast.success(t('setup.success'))
        setTimeout(() => navigate({ to: '/sign-in' }), 1000)
      }
    } catch {
      // handled by interceptor
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-svh">
      {/* Left panel */}
      <div className="hidden lg:flex lg:w-1/2 relative overflow-hidden bg-primary">
        <div className="absolute inset-0 bg-gradient-to-br from-primary via-primary/90 to-primary/80" />
        <div className="absolute inset-0">
          <div className="absolute top-1/4 left-1/4 h-64 w-64 rounded-full bg-ring/10 blur-3xl" />
          <div className="absolute bottom-1/4 right-1/4 h-48 w-48 rounded-full bg-white/5 blur-3xl" />
          <div className="absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.03)_1px,transparent_1px)] bg-[size:48px_48px]" />
        </div>
        <div className="relative z-10 flex flex-col justify-between p-12 text-primary-foreground">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl overflow-hidden">
              <img src="/favicon.svg" alt="Logo" className="h-10 w-10" />
            </div>
            <span className="text-xl font-semibold">NewMCP</span>
          </div>
          <div>
            <h2 className="text-3xl font-bold leading-tight">
              {t('setup.heroTitle')}
            </h2>
            <p className="mt-4 max-w-sm text-sm leading-relaxed text-primary-foreground/70">
              {t('setup.heroDesc')}
            </p>
          </div>
          <p className="text-xs text-primary-foreground/40">MCP Protocol Gateway</p>
        </div>
      </div>

      {/* Right panel — form */}
      <div className="flex flex-1 items-center justify-center p-8">
        <div className="w-full max-w-sm">
          <div className="mb-8">
            <div className="flex items-center gap-2 lg:hidden mb-8">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg overflow-hidden">
                <img src="/favicon.svg" alt="Logo" className="h-8 w-8" />
              </div>
              <span className="text-lg font-semibold">NewMCP</span>
            </div>
            <div className="flex items-center gap-3 mb-4">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                <Shield className="h-5 w-5 text-primary" />
              </div>
              <h1 className="text-2xl font-semibold tracking-tight">
                {t('setup.title')}
              </h1>
            </div>
            <p className="text-sm text-muted-foreground">
              {t('setup.description')}
            </p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">{t('setup.adminUsername')}</Label>
              <Input
                id="username"
                placeholder={t('setup.usernamePlaceholder')}
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                required
                autoComplete="username"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t('setup.password')}</Label>
              <Input
                id="password"
                type="password"
                placeholder={t('setup.passwordPlaceholder')}
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                required
                autoComplete="new-password"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirmPassword">{t('setup.confirmPassword')}</Label>
              <Input
                id="confirmPassword"
                type="password"
                placeholder={t('setup.confirmPasswordPlaceholder')}
                value={form.confirmPassword}
                onChange={(e) =>
                  setForm({ ...form, confirmPassword: e.target.value })
                }
                required
                autoComplete="new-password"
              />
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('setup.initialize')}
            </Button>
          </form>
        </div>
      </div>
    </div>
  )
}
