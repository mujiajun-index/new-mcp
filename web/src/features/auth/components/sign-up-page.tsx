import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from '@tanstack/react-router'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { Loader2 } from 'lucide-react'

export function SignUpPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { auth } = useAuthStore()
  const [loading, setLoading] = useState(false)
  const [form, setForm] = useState({ username: '', email: '', password: '', confirmPassword: '' })

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (form.password !== form.confirmPassword) {
      toast.error('两次输入的密码不一致')
      return
    }
    setLoading(true)
    try {
      const res = await api.post('/auth/register', {
        username: form.username,
        email: form.email,
        password: form.password,
      })
      const { data } = res.data
      if (data?.token) {
        localStorage.setItem('newmcp-token', data.token)
        auth.setUser({ id: data.id, username: data.username, role: data.role })
        toast.success('注册成功')
        navigate({ to: '/dashboard' })
      }
    } catch {
      // handled by interceptor
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-svh items-center justify-center p-8">
      <Link to="/" className="absolute top-6 left-8 flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl overflow-hidden">
          <img src="/favicon.svg" alt="Logo" className="h-10 w-10" />
        </div>
        <span className="text-xl font-semibold">NewMCP</span>
      </Link>
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center">
          <h1 className="text-2xl font-semibold tracking-tight">{t('auth.createAccount')}</h1>
          <p className="mt-2 text-sm text-muted-foreground">{t('auth.signUpDesc')}</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="username">{t('auth.username')}</Label>
            <Input
              id="username"
              placeholder={t('auth.username')}
              value={form.username}
              onChange={(e) => setForm({ ...form, username: e.target.value })}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="email">{t('auth.email')}</Label>
            <Input
              id="email"
              type="email"
              placeholder={t('auth.email')}
              value={form.email}
              onChange={(e) => setForm({ ...form, email: e.target.value })}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">{t('auth.password')}</Label>
            <Input
              id="password"
              type="password"
              placeholder={t('auth.password')}
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="confirmPassword">{t('auth.confirmPassword')}</Label>
            <Input
              id="confirmPassword"
              type="password"
              placeholder={t('auth.confirmPassword')}
              value={form.confirmPassword}
              onChange={(e) => setForm({ ...form, confirmPassword: e.target.value })}
              required
            />
          </div>
          <Button type="submit" className="w-full" disabled={loading}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {t('auth.signUp')}
          </Button>
        </form>

        <p className="mt-6 text-center text-sm text-muted-foreground">
          {t('auth.hasAccount')}{' '}
          <Link to="/sign-in" className="font-medium text-primary hover:underline">
            {t('auth.goSignIn')}
          </Link>
        </p>
      </div>
    </div>
  )
}
