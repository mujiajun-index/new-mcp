import { createLazyFileRoute } from '@tanstack/react-router'
import { AdminUsersPage } from '@/features/admin/components/admin-users-page'

export const Route = createLazyFileRoute('/_authenticated/admin/users')({
  component: AdminUsersPage,
})
