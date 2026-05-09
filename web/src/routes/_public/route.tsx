import { createFileRoute, redirect, Outlet } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_public')({
  beforeLoad: async () => {
    const { auth } = useAuthStore.getState()
    if (auth.user) {
      throw redirect({ to: '/dashboard' })
    }
  },
  component: PublicLayout,
})

function PublicLayout() {
  return (
    <div className="flex min-h-svh flex-col">
      <Outlet />
    </div>
  )
}
