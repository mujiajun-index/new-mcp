import { Outlet } from '@tanstack/react-router'
import { AppSidebar } from './app-sidebar'
import { Header } from './header'

export function AuthenticatedLayout() {
  return (
    <div className="flex h-svh overflow-hidden">
      <AppSidebar />
      <div className="flex flex-1 flex-col overflow-hidden">
        <Header />
        <main className="flex-1 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
