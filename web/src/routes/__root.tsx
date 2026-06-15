import { useEffect } from 'react'
import { createRootRouteWithContext, Outlet, redirect } from '@tanstack/react-router'
import { Toaster } from 'sonner'
import { checkSetupStatus } from '@/lib/setup-check'
import { useSystemConfigStore } from '@/stores/system-config-store'

interface RouterContext {
  queryClient: import('@tanstack/react-query').QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ location }) => {
    const pathname = location?.pathname || ''
    const needsSetup = await checkSetupStatus()

    if (needsSetup && pathname !== '/setup') {
      throw redirect({ to: '/setup' })
    }
    if (!needsSetup && pathname === '/setup') {
      throw redirect({ to: '/sign-in' })
    }
  },
  component: RootComponent,
})

function RootComponent() {
  const { config, fetchPublicSettings } = useSystemConfigStore()

  useEffect(() => {
    fetchPublicSettings()
  }, [fetchPublicSettings])

  useEffect(() => {
    document.title = config.systemName || 'NewMCP'
  }, [config.systemName])

  return (
    <>
      <Outlet />
      <Toaster position="top-right" richColors closeButton />
    </>
  )
}
