import { createLazyFileRoute } from '@tanstack/react-router'
import { AdminSettingsPage } from '@/features/admin/components/admin-settings-page'

export const Route = createLazyFileRoute('/_authenticated/admin/system')({
  component: AdminSettingsPage,
})
