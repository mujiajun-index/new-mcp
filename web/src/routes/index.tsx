import { useEffect, useState } from 'react'
import { createFileRoute, Link, useNavigate, useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'
import { useTheme } from '@/context/theme-provider'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogFooter,
  AlertDialogTitle, AlertDialogDescription, AlertDialogAction, AlertDialogCancel,
} from '@/components/ui/alert-dialog'
import { Server, GitBranch, Cloud, Shield, ArrowRight, Zap, Moon, Sun, Monitor, Languages, LogOut, User, Check, Copy, Link2 } from 'lucide-react'

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
  const [showSignOutDialog, setShowSignOutDialog] = useState(false)

  const { config: systemConfig, fetchPublicSettings } = useSystemConfigStore()

  useEffect(() => {
    fetchPublicSettings()
  }, [fetchPublicSettings])

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
              <>
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
                  <DropdownMenuItem onClick={() => setShowSignOutDialog(true)} className="text-destructive focus:text-destructive">
                    <LogOut className="mr-2 h-4 w-4" />
                    {t('auth.signOut')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>

              <AlertDialog open={showSignOutDialog} onOpenChange={setShowSignOutDialog}>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>{t('auth.confirmSignOut')}</AlertDialogTitle>
                    <AlertDialogDescription>{t('auth.confirmSignOutDesc')}</AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
                    <AlertDialogAction onClick={handleSignOut} className="bg-destructive text-white hover:bg-destructive/90">
                      {t('auth.signOut')}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
              </>
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

      {/* Endpoints */}
      <EndpointsSection baseUrl={systemConfig.serverAddress} />

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

function EndpointsSection({ baseUrl }: { baseUrl: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState<string | null>(null)

  const base = baseUrl.replace(/\/$/, '')
  const endpoints = [
    {
      path: '/mcp',
      mode: t('landing.directMode'),
      desc: t('landing.directModeDesc'),
      accent: 'from-sky-500/20 to-blue-500/20',
      textColor: 'text-sky-600 dark:text-sky-400',
      borderColor: 'border-sky-500/20',
      glowColor: 'bg-sky-500/5',
    },
    {
      path: '/smart/mcp',
      mode: t('landing.smartMode'),
      desc: t('landing.smartModeDesc'),
      accent: 'from-violet-500/20 to-purple-500/20',
      textColor: 'text-violet-600 dark:text-violet-400',
      borderColor: 'border-violet-500/20',
      glowColor: 'bg-violet-500/5',
    },
  ]

  const handleCopy = async (url: string) => {
    let success = false
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(url)
        success = true
      } else {
        // Fallback for non-secure contexts (http://), where Clipboard API is unavailable
        const textarea = document.createElement('textarea')
        textarea.value = url
        textarea.style.position = 'fixed'
        textarea.style.top = '0'
        textarea.style.left = '0'
        textarea.style.opacity = '0'
        document.body.appendChild(textarea)
        textarea.focus()
        textarea.select()
        success = document.execCommand('copy')
        document.body.removeChild(textarea)
      }
    } catch {
      success = false
    }
    if (success) {
      setCopied(url)
      setTimeout(() => setCopied(null), 2000)
    }
  }

  return (
    <section className="relative overflow-hidden border-t px-6 py-24">
      {/* Background decoration */}
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute top-1/2 left-1/4 -translate-y-1/2 h-[500px] w-[500px] rounded-full bg-ring/3 blur-3xl" />
        <div className="absolute top-1/2 right-1/4 -translate-y-1/2 h-[500px] w-[500px] rounded-full bg-ring/3 blur-3xl" />
      </div>

      <div className="relative z-10 mx-auto max-w-4xl">
        <div className="mb-12 text-center">
          <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">
            {t('landing.endpointsTitle')}
          </h2>
          <p className="mx-auto mt-4 max-w-xl text-base text-muted-foreground">
            {t('landing.endpointsDesc')}
          </p>
        </div>

        <div className="grid gap-6 sm:grid-cols-2">
          {endpoints.map((ep, i) => {
            const url = `${base}${ep.path}`
            const isCopied = copied === url
            return (
              <div
                key={ep.path}
                className={cn(
                  'group relative rounded-2xl border bg-card/80 p-6 backdrop-blur transition-all duration-300',
                  'hover:border-ring/20 hover:shadow-lg hover:shadow-ring/5',
                  ep.borderColor,
                )}
                style={{ animationDelay: `${i * 120}ms` }}
              >
                <div className={cn('absolute -inset-px rounded-2xl opacity-0 transition-opacity duration-500 group-hover:opacity-100', ep.glowColor)} />
                <div className="relative z-10">
                  <div className="mb-4 flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <div className={cn('flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br', ep.accent)}>
                        <Link2 className="h-4 w-4" />
                      </div>
                      <span className={cn('text-sm font-semibold', ep.textColor)}>{ep.mode}</span>
                    </div>
                    <span className="rounded-full bg-muted px-2.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                      POST
                    </span>
                  </div>

                  <p className="mb-4 text-sm text-muted-foreground">{ep.desc}</p>

                  <div className="flex items-center gap-2">
                    <div className="flex-1 overflow-hidden rounded-lg border bg-muted/50 px-3 py-2">
                      <code className="block truncate text-sm font-mono text-foreground">{url}</code>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      className={cn(
                        'h-9 w-9 shrink-0 cursor-pointer transition-colors',
                        isCopied && 'text-emerald-600 dark:text-emerald-400',
                      )}
                      onClick={() => handleCopy(url)}
                      title={isCopied ? t('common.copied') : t('common.copy')}
                    >
                      {isCopied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                    </Button>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}

function cn(...classes: (string | undefined | false)[]) {
  return classes.filter(Boolean).join(' ')
}
