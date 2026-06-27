import { createFileRoute, redirect, Outlet } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { isAdminRole } from '@/lib/roles'

export const Route = createFileRoute('/_authenticated/admin')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!isAdminRole(auth.user?.role)) {
      throw redirect({ to: '/dashboard' })
    }
  },
  component: () => <Outlet />,
})
