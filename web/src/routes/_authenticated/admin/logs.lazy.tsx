import { createLazyFileRoute } from '@tanstack/react-router'
import { AdminLogsPage } from '@/features/admin/components/admin-logs-page'

export const Route = createLazyFileRoute('/_authenticated/admin/logs')({
  component: AdminLogsPage,
})
