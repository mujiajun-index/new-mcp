import { createFileRoute, redirect, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'
import { Server, GitBranch, Cloud, Shield, ArrowRight, Zap } from 'lucide-react'

export const Route = createFileRoute('/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (auth.user) {
      throw redirect({ to: '/dashboard' })
    }
  },
  component: LandingPage,
})

function LandingPage() {
  const { t } = useTranslation()

  const features = [
    { icon: Server, title: t('landing.feature1Title'), desc: t('landing.feature1Desc'), accent: 'from-sky-500/20 to-blue-500/20' },
    { icon: GitBranch, title: t('landing.feature2Title'), desc: t('landing.feature2Desc'), accent: 'from-violet-500/20 to-purple-500/20' },
    { icon: Cloud, title: t('landing.feature3Title'), desc: t('landing.feature3Desc'), accent: 'from-emerald-500/20 to-green-500/20' },
    { icon: Shield, title: t('landing.feature4Title'), desc: t('landing.feature4Desc'), accent: 'from-amber-500/20 to-orange-500/20' },
  ]

  return (
    <div className="min-h-svh bg-background">
      {/* Nav */}
      <nav className="fixed top-0 z-50 w-full border-b bg-background/60 backdrop-blur-xl">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary">
              <svg viewBox="0 0 32 32" className="h-5 w-5" fill="none">
                <path d="M6 8h4l4 8-4 8H6l4-8-4-8Z" fill="currentColor" className="text-primary-foreground" />
                <path d="M14 8h4l4 8-4 8h-4l4-8-4-8Z" fill="currentColor" className="text-primary-foreground/60" />
              </svg>
            </div>
            <span className="text-lg font-semibold tracking-tight">NewMCP</span>
          </div>
          <div className="flex items-center gap-3">
            <Link to="/sign-in">
              <Button variant="ghost" size="sm">{t('auth.signIn')}</Button>
            </Link>
            <Link to="/sign-up">
              <Button size="sm">{t('auth.signUp')}</Button>
            </Link>
          </div>
        </div>
      </nav>

      {/* Hero */}
      <section className="relative flex min-h-svh flex-col items-center justify-center overflow-hidden px-6 pt-14">
        {/* Background decoration */}
        <div className="pointer-events-none absolute inset-0 overflow-hidden">
          <div className="absolute -top-40 left-1/2 -translate-x-1/2 h-[600px] w-[600px] rounded-full bg-ring/5 blur-3xl" />
          <div className="absolute top-1/3 -left-20 h-[400px] w-[400px] rounded-full bg-sky-500/5 blur-3xl" />
          <div className="absolute top-1/2 -right-20 h-[400px] w-[400px] rounded-full bg-violet-500/5 blur-3xl" />
          {/* Grid pattern */}
          <div className="absolute inset-0 bg-[linear-gradient(rgba(0,0,0,0.02)_1px,transparent_1px),linear-gradient(90deg,rgba(0,0,0,0.02)_1px,transparent_1px)] bg-[size:64px_64px] dark:bg-[linear-gradient(rgba(255,255,255,0.02)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.02)_1px,transparent_1px)]" />
        </div>

        <div className="relative z-10 mx-auto max-w-3xl text-center">
          {/* Badge */}
          <div className="mb-6 inline-flex items-center gap-2 rounded-full border bg-background px-4 py-1.5 text-xs font-medium text-muted-foreground shadow-sm">
            <Zap className="h-3 w-3 text-ring" />
            MCP Protocol Gateway
          </div>

          <h1 className="text-4xl font-bold tracking-tight sm:text-5xl lg:text-6xl">
            <span className="bg-gradient-to-r from-foreground to-foreground/70 bg-clip-text">
              {t('landing.hero')}
            </span>
          </h1>

          <p className="mx-auto mt-6 max-w-2xl text-lg leading-relaxed text-muted-foreground">
            {t('landing.heroDesc')}
          </p>

          <div className="mt-10 flex items-center justify-center gap-4">
            <Link to="/sign-up">
              <Button size="lg" className="gap-2 rounded-full px-6">
                {t('landing.getStarted')}
                <ArrowRight className="h-4 w-4" />
              </Button>
            </Link>
            <Button variant="outline" size="lg" className="rounded-full px-6">
              {t('landing.viewDocs')}
            </Button>
          </div>
        </div>
      </section>

      {/* Features */}
      <section className="relative border-t bg-muted/30 px-6 py-24">
        <div className="mx-auto max-w-6xl">
          <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
            {features.map((f, i) => (
              <div
                key={i}
                className="group relative rounded-2xl border bg-card p-6 transition-all duration-300 hover:border-ring/20 hover:shadow-lg hover:shadow-ring/5"
              >
                <div className={cn('mb-4 flex h-11 w-11 items-center justify-center rounded-xl bg-gradient-to-br', f.accent)}>
                  <f.icon className="h-5 w-5" />
                </div>
                <h3 className="mb-2 font-semibold">{f.title}</h3>
                <p className="text-sm leading-relaxed text-muted-foreground">{f.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t px-6 py-8">
        <div className="mx-auto flex max-w-6xl items-center justify-between text-sm text-muted-foreground">
          <span>NewMCP</span>
          <span>MCP Protocol Gateway</span>
        </div>
      </footer>
    </div>
  )
}

function cn(...classes: (string | undefined | false)[]) {
  return classes.filter(Boolean).join(' ')
}
