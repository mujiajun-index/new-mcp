import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { Loader2 } from 'lucide-react'
import { useSystemConfigStore } from '@/stores/system-config-store'

export function SignInPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const search = useSearch({ strict: false }) as { redirect?: string }
  const { auth } = useAuthStore()
  const { config } = useSystemConfigStore()
  const [loading, setLoading] = useState(false)
  const [form, setForm] = useState({ username: '', password: '' })

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      const res = await api.post('/auth/login', form)
      const { data } = res.data
      if (data?.token) {
        localStorage.setItem('newmcp-token', data.token)
        auth.setUser({ id: data.id, username: data.username, role: data.role })
        toast.success(t('common.success'))
        navigate({ to: search.redirect || '/dashboard' })
      }
    } catch {
      // handled by interceptor
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-svh">
      {/* Left panel — visual */}
      <div className="hidden lg:flex lg:w-1/2 relative overflow-hidden bg-primary">
        <div className="absolute inset-0 bg-gradient-to-br from-primary via-primary/90 to-primary/80" />
        {/* Decorative elements */}
        <div className="absolute inset-0">
          <div className="absolute top-1/4 left-1/4 h-64 w-64 rounded-full bg-ring/10 blur-3xl" />
          <div className="absolute bottom-1/4 right-1/4 h-48 w-48 rounded-full bg-white/5 blur-3xl" />
          {/* Grid overlay */}
          <div className="absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.03)_1px,transparent_1px)] bg-[size:48px_48px]" />
        </div>
        <div className="relative z-10 flex flex-col justify-between p-12 text-primary-foreground">
          <Link to="/" className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl overflow-hidden">
              <img src="/favicon.svg" alt="Logo" className="h-10 w-10" />
            </div>
            <span className="text-xl font-semibold">{config.systemName}</span>
          </Link>
          <div>
            <h2 className="text-3xl font-bold leading-tight whitespace-pre-line">
              {t('auth.signInHeroTitle')}
            </h2>
            <p className="mt-4 max-w-sm text-sm leading-relaxed text-primary-foreground/70">
              {t('auth.signInHeroDesc')}
            </p>
          </div>
          <p className="text-xs text-primary-foreground/40">{config.footer || 'MCP Protocol Gateway'}</p>
        </div>
      </div>

      {/* Right panel — form */}
      <div className="flex flex-1 items-center justify-center p-8">
        <div className="w-full max-w-sm">
          <div className="mb-8">
            <Link to="/" className="flex items-center gap-2 lg:hidden mb-8">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg overflow-hidden">
                <img src="/favicon.svg" alt="Logo" className="h-8 w-8" />
              </div>
              <span className="text-lg font-semibold">{config.systemName}</span>
            </Link>
            <h1 className="text-2xl font-semibold tracking-tight">{t('auth.welcomeBack')}</h1>
            <p className="mt-2 text-sm text-muted-foreground">{t('auth.signInDesc')}</p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">{t('auth.usernameOrEmail')}</Label>
              <Input
                id="username"
                placeholder={t('auth.usernameOrEmail')}
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                required
              />
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="password">{t('auth.password')}</Label>
              </div>
              <Input
                id="password"
                type="password"
                placeholder={t('auth.password')}
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                required
              />
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('auth.signIn')}
            </Button>
          </form>

          <p className="mt-6 text-center text-sm text-muted-foreground">
            {t('auth.noAccount')}{' '}
            <Link to="/sign-up" className="font-medium text-primary hover:underline">
              {t('auth.goSignUp')}
            </Link>
          </p>
        </div>
      </div>

      {/* Footer — mobile only (desktop shows it inside the left visual panel) */}
      <footer className="absolute bottom-6 left-0 right-0 flex items-center justify-center gap-2 px-8 text-center text-xs text-muted-foreground lg:hidden">
        <span>{config.systemName}</span>
        <span className="text-muted-foreground/40">·</span>
        <span>{config.footer || 'MCP Protocol Gateway'}</span>
      </footer>
    </div>
  )
}
