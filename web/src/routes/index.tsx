import { createFileRoute, Link, useNavigate, useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { useTheme } from '@/context/theme-provider'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Server, GitBranch, Cloud, Shield, ArrowRight, Zap, Moon, Sun, Monitor, Languages, LogOut, User, Check } from 'lucide-react'

export const Route = createFileRoute('/')({
  component: LandingPage,
})

function LandingPage() {
  const { t, i18n } = useTranslation()
  const { auth } = useAuthStore()
  const navigate = useNavigate()
  const location = useLocation()
  const { theme, setTheme } = useTheme()
  const isLoggedIn = !!auth.user

  const navItems = [
    { label: t('nav.home'), to: '/' as const },
    { label: t('nav.dashboard'), to: '/dashboard' as const },
    { label: t('nav.marketplace'), to: '/marketplace' as const },
  ]

  const handleNavClick = (to: string) => {
    if (to !== '/' && !isLoggedIn) {
      navigate({ to: '/sign-in' })
      return
    }
    navigate({ to: to as never })
  }

  const handleSignOut = () => {
    auth.reset()
    navigate({ to: '/sign-in' })
  }

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
          <div className="flex items-center gap-6">
            <Link to="/" className="flex items-center gap-2.5">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg overflow-hidden">
                <img src="/favicon.svg" alt="Logo" className="h-8 w-8" />
              </div>
              <span className="text-lg font-semibold tracking-tight">NewMCP</span>
            </Link>
            <div className="flex items-center gap-1">
              {navItems.map((item) => {
                const isActive = item.to === '/'
                  ? location.pathname === '/'
                  : location.pathname.startsWith(item.to)
                if (item.to === '/') {
                  return (
                    <Link
                      key={item.to}
                      to={item.to}
                      className={cn(
                        'px-3 py-1.5 text-sm font-medium transition-colors hover:text-foreground',
                        isActive ? 'text-foreground' : 'text-muted-foreground',
                      )}
                    >
                      {item.label}
                    </Link>
                  )
                }
                return (
                  <button
                    key={item.to}
                    onClick={() => handleNavClick(item.to)}
                    className={cn(
                      'px-3 py-1.5 text-sm font-medium transition-colors hover:text-foreground',
                      isActive ? 'text-foreground' : 'text-muted-foreground',
                    )}
                  >
                    {item.label}
                  </button>
                )
              })}
            </div>
          </div>

          <div className="flex items-center gap-2">
            {/* Theme toggle */}
            <Button
              variant="ghost"
              size="icon"
              onClick={() => {
                const next = theme === 'light' ? 'dark' : theme === 'dark' ? 'system' : 'light'
                setTheme(next)
              }}
              className="h-8 w-8"
            >
              {theme === 'dark' ? (
                <Moon className="h-4 w-4" />
              ) : theme === 'light' ? (
                <Sun className="h-4 w-4" />
              ) : (
                <Monitor className="h-4 w-4" />
              )}
            </Button>

            {/* Language toggle */}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="h-8 w-8">
                  <Languages className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => i18n.changeLanguage('zh')}>
                  <span className="flex-1">中文</span>
                  {i18n.language === 'zh' && <Check className="h-4 w-4 text-primary" />}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => i18n.changeLanguage('en')}>
                  <span className="flex-1">English</span>
                  {i18n.language === 'en' && <Check className="h-4 w-4 text-primary" />}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>

            {/* Auth section */}
            {isLoggedIn ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" className="relative h-8 gap-2 rounded-full pl-2 pr-3">
                    <Avatar className="h-6 w-6">
                      <AvatarFallback className="bg-primary/10 text-primary text-[10px]">
                        {auth.user?.username?.charAt(0)?.toUpperCase() || 'U'}
                      </AvatarFallback>
                    </Avatar>
                    <span className="text-sm font-medium">{auth.user?.username}</span>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent className="w-48" align="end">
                  <DropdownMenuLabel className="font-normal">
                    <div className="flex flex-col gap-1">
                      <p className="text-sm font-medium">{auth.user?.username}</p>
                    </div>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={() => navigate({ to: '/settings' })}>
                    <User className="mr-2 h-4 w-4" />
                    {t('nav.settings')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={handleSignOut} className="text-destructive focus:text-destructive">
                    <LogOut className="mr-2 h-4 w-4" />
                    {t('auth.signOut')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <div className="flex items-center gap-3">
                <Link to="/sign-in">
                  <Button variant="ghost" size="sm">{t('auth.signIn')}</Button>
                </Link>
                <Link to="/sign-up">
                  <Button size="sm">{t('auth.signUp')}</Button>
                </Link>
              </div>
            )}
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
            {isLoggedIn ? (
              <Button size="lg" className="gap-2 rounded-full px-6" onClick={() => navigate({ to: '/dashboard' })}>
                {t('nav.dashboard')}
                <ArrowRight className="h-4 w-4" />
              </Button>
            ) : (
              <>
                <Link to="/sign-up">
                  <Button size="lg" className="gap-2 rounded-full px-6">
                    {t('landing.getStarted')}
                    <ArrowRight className="h-4 w-4" />
                  </Button>
                </Link>
                <Button variant="outline" size="lg" className="rounded-full px-6">
                  {t('landing.viewDocs')}
                </Button>
              </>
            )}
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
