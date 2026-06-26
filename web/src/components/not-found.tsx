import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, Home } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

// Rendered by the router (defaultNotFoundComponent) whenever no route matches
// the current URL. Kept self-contained (own full-screen layout, brand mark and
// navigation) so it looks right whether it bubbles up from the root boundary or
// from a nested authenticated section.
export function NotFound() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.auth.user)
  const systemName = useSystemConfigStore((s) => s.config.systemName)

  return (
    <div className="relative flex min-h-svh flex-col items-center justify-center overflow-hidden px-6">
      {/* Soft ambient glow, mirroring the landing page's background treatment. */}
      <div className="pointer-events-none absolute inset-0 -z-10">
        <div className="absolute left-1/2 top-1/3 h-[440px] w-[440px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary/5 blur-3xl" />
      </div>

      <div className="flex animate-appear flex-col items-center gap-7 text-center">
        {/* Brand */}
        <div className="flex items-center gap-2">
          <img src="/favicon.svg" alt="Logo" className="h-7 w-7" />
          <span className="text-base font-semibold tracking-tight">
            {systemName}
          </span>
        </div>

        {/* 404 mark */}
        <h1 className="bg-gradient-to-b from-foreground to-foreground/30 bg-clip-text text-[120px] font-bold leading-none tracking-tighter text-transparent sm:text-[168px]">
          404
        </h1>

        <div className="space-y-2">
          <h2 className="text-xl font-semibold">{t('errors.notFoundTitle')}</h2>
          <p className="mx-auto max-w-md text-sm text-muted-foreground">
            {t('errors.notFoundDesc')}
          </p>
        </div>

        <div className="flex flex-wrap items-center justify-center gap-3">
          <Button onClick={() => navigate({ to: user ? '/dashboard' : '/' })}>
            <Home />
            {t('common.backHome')}
          </Button>
          <Button variant="outline" onClick={() => window.history.back()}>
            <ArrowLeft />
            {t('common.backPrev')}
          </Button>
        </div>
      </div>
    </div>
  )
}
