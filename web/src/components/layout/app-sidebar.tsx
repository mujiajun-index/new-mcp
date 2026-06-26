import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { cn } from '@/lib/utils'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tooltip, TooltipContent, TooltipTrigger, TooltipProvider } from '@/components/ui/tooltip'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { useIsMobile } from '@/hooks/use-mobile'
import {
  LayoutDashboard, Server, FolderTree, Cloud, Eye, Camera,
  Key, Store, Settings, Shield, Users, Wrench,
  ClipboardCheck, ChevronLeft, Activity,
} from 'lucide-react'
import { useState } from 'react'
import { useSystemConfigStore } from '@/stores/system-config-store'

interface NavItem {
  label: string
  icon: React.ComponentType<{ className?: string }>
  href: string
  adminOnly?: boolean
}

const mainNav: NavItem[] = [
  { label: 'nav.dashboard', icon: LayoutDashboard, href: '/dashboard' },
  { label: 'nav.services', icon: Server, href: '/services' },
  { label: 'nav.groups', icon: FolderTree, href: '/groups' },
  { label: 'nav.connections', icon: Cloud, href: '/connections' },
  { label: 'nav.vision', icon: Eye, href: '/vision' },
  { label: 'nav.cameras', icon: Camera, href: '/cameras' },
  { label: 'nav.apiKeys', icon: Key, href: '/api-keys' },
  { label: 'nav.logs', icon: Activity, href: '/logs' },
  { label: 'nav.marketplace', icon: Store, href: '/marketplace' },
  { label: 'nav.settings', icon: Settings, href: '/settings' },
]

const adminNav: NavItem[] = [
  { label: 'nav.adminUsers', icon: Users, href: '/admin/users', adminOnly: true },
  { label: 'nav.adminMarketplace', icon: Wrench, href: '/admin/marketplace', adminOnly: true },
  { label: 'nav.adminReviews', icon: ClipboardCheck, href: '/admin/reviews', adminOnly: true },
  { label: 'nav.adminSystem', icon: Shield, href: '/admin/system', adminOnly: true },
]

/**
 * Shared navigation body. Renders identically inside the desktop <aside> and
 * the mobile <Sheet>. `className` controls how it fills its container:
 * desktop passes `flex-1 min-h-0`, mobile passes `h-full`.
 */
function SidebarNav({
  collapsed,
  onNavigate,
  className,
}: {
  collapsed: boolean
  onNavigate?: () => void
  className?: string
}) {
  const { t } = useTranslation()
  const router = useRouterState()
  const currentPath = router.location.pathname
  const { auth } = useAuthStore()
  const { config } = useSystemConfigStore()
  const isAdmin = auth.user?.role === 'admin'

  const isActive = (href: string) => {
    if (href === '/dashboard') return currentPath === '/dashboard'
    return currentPath.startsWith(href)
  }

  const renderLinks = (items: NavItem[]) =>
    items.map((item) => {
      const active = isActive(item.href)
      const link = (
        <Link
          key={item.href}
          to={item.href}
          onClick={onNavigate}
          className={cn(
            'group flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-150',
            active
              ? 'bg-sidebar-accent text-sidebar-accent-foreground'
              : 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-foreground',
            collapsed && 'justify-center px-2'
          )}
        >
          <item.icon className={cn('h-4 w-4 shrink-0', active && 'text-ring')} />
          {!collapsed && <span>{t(item.label)}</span>}
        </Link>
      )

      if (collapsed) {
        return (
          <Tooltip key={item.href}>
            <TooltipTrigger asChild>{link}</TooltipTrigger>
            <TooltipContent side="right">{t(item.label)}</TooltipContent>
          </Tooltip>
        )
      }
      return link
    })

  return (
    <TooltipProvider delayDuration={0}>
      <div className={cn('flex flex-col', className)}>
        {/* Logo */}
        <Link
          to="/"
          onClick={onNavigate}
          className={cn(
            'flex h-14 items-center border-b border-sidebar-border px-4',
            collapsed ? 'justify-center' : 'gap-3'
          )}
        >
          <div className="flex h-8 w-8 items-center justify-center rounded-lg overflow-hidden">
            <img src="/favicon.svg" alt="Logo" className="h-8 w-8" />
          </div>
          {!collapsed && (
            <span className="text-base font-semibold tracking-tight">{config.systemName}</span>
          )}
        </Link>

        {/* Navigation */}
        <ScrollArea className="flex-1 py-3">
          <nav className="flex flex-col gap-1 px-3">
            {renderLinks(mainNav)}

            {isAdmin && (
              <>
                <div className={cn('my-2 h-px bg-sidebar-border', collapsed && 'mx-1')} />
                {!collapsed && (
                  <p className="mb-1 px-3 text-[11px] font-medium uppercase tracking-wider text-sidebar-foreground/40">
                    {t('nav.admin')}
                  </p>
                )}
                {renderLinks(adminNav)}
              </>
            )}
          </nav>
        </ScrollArea>
      </div>
    </TooltipProvider>
  )
}

export function AppSidebar({
  mobileOpen,
  onMobileOpenChange,
}: {
  mobileOpen: boolean
  onMobileOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const [collapsed, setCollapsed] = useState(false)

  if (isMobile) {
    return (
      <Sheet open={mobileOpen} onOpenChange={onMobileOpenChange}>
        <SheetContent side="left" showClose={false} className="w-[240px] gap-0 p-0">
          {/* Visually hidden a11y labels required by Radix Dialog. */}
          <SheetHeader className="sr-only">
            <SheetTitle>{t('auth.nav')}</SheetTitle>
            <SheetDescription>{t('auth.navDesc')}</SheetDescription>
          </SheetHeader>
          <SidebarNav
            collapsed={false}
            className="min-h-0 flex-1"
            onNavigate={() => onMobileOpenChange(false)}
          />

          {/* Close drawer (mobile equivalent of the desktop collapse toggle) */}
          <div className="border-t border-sidebar-border p-3">
            <button
              onClick={() => onMobileOpenChange(false)}
              className="flex h-8 w-full items-center gap-2 rounded-lg px-2 text-sidebar-foreground/50 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
            >
              <ChevronLeft className="h-4 w-4" />
              <span className="text-xs">{t('auth.collapseSidebar')}</span>
            </button>
          </div>
        </SheetContent>
      </Sheet>
    )
  }

  return (
    <aside
      className={cn(
        'hidden h-svh flex-col border-r border-sidebar-border bg-sidebar transition-[width] duration-200 ease-out lg:flex',
        collapsed ? 'w-[68px]' : 'w-[240px]'
      )}
    >
      <SidebarNav collapsed={collapsed} className="min-h-0 flex-1" />

      {/* Collapse toggle (desktop only) */}
      <div className="border-t border-sidebar-border p-3">
        <button
          onClick={() => setCollapsed(!collapsed)}
          className={cn(
            'flex h-8 w-full items-center rounded-lg text-sidebar-foreground/50 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground',
            collapsed ? 'justify-center' : 'gap-2 px-2'
          )}
        >
          <ChevronLeft className={cn('h-4 w-4 transition-transform', collapsed && 'rotate-180')} />
          {!collapsed && <span className="text-xs">{t('auth.collapseSidebar')}</span>}
        </button>
      </div>
    </aside>
  )
}
