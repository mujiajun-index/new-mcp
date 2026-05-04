import { createFileRoute, redirect, Outlet } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { AuthenticatedLayout } from '@/components/layout/authenticated-layout'

let sessionVerified = false

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: async ({ location }) => {
    const { auth } = useAuthStore.getState()

    if (!auth.user) {
      throw redirect({ to: '/sign-in', search: { redirect: location.href } })
    }

    if (!sessionVerified) {
      try {
        const { getSelf } = await import('@/lib/api')
        const res = await getSelf()
        if (res?.success !== false && res?.data) {
          auth.setUser(res.data)
          sessionVerified = true
        } else {
          auth.reset()
          sessionVerified = false
          throw redirect({ to: '/sign-in', search: { redirect: location.href } })
        }
      } catch {
        if (!auth.user) {
          throw redirect({ to: '/sign-in', search: { redirect: location.href } })
        }
        sessionVerified = true
      }
    }
  },
  component: AuthenticatedLayout,
})

export { sessionVerified }
