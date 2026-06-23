import { createFileRoute, redirect } from '@tanstack/react-router'
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
        if (res?.success && res?.data) {
          auth.setUser({
            id: res.data.id,
            username: res.data.username,
            role: res.data.role,
          })
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
