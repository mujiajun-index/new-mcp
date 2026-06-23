import { useState } from 'react'
import { Outlet } from '@tanstack/react-router'
import { AppSidebar } from './app-sidebar'
import { Header } from './header'
import { useSystemConfigStore } from '@/stores/system-config-store'

export function AuthenticatedLayout() {
  // Shared source of truth for the mobile sidebar drawer: the header opens it,
  // the sidebar renders it and closes itself on navigation.
  const [mobileOpen, setMobileOpen] = useState(false)
  const { config } = useSystemConfigStore()

  return (
    <div className="flex h-svh overflow-hidden">
      <AppSidebar mobileOpen={mobileOpen} onMobileOpenChange={setMobileOpen} />
      <div className="flex flex-1 flex-col overflow-hidden">
        <Header onOpenMobileNav={() => setMobileOpen(true)} />
        <main className="flex-1 overflow-auto">
          <Outlet />
        </main>
        <footer className="shrink-0 border-t bg-background px-4 py-3 sm:px-6 lg:px-8">
          <div className="flex h-8 items-center justify-between gap-2 text-xs text-muted-foreground">
            <span className="truncate">{config.systemName}</span>
            <span className="truncate">{config.footer || 'MCP Protocol Gateway'}</span>
          </div>
        </footer>
      </div>
    </div>
  )
}
