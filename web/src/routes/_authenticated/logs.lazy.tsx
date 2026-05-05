import { createLazyFileRoute } from '@tanstack/react-router'
import { UserLogsPage } from '@/features/logs/components/user-logs-page'

export const Route = createLazyFileRoute('/_authenticated/logs')({
  component: UserLogsPage,
})
