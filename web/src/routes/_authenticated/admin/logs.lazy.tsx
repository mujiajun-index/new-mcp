import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/admin/logs')({
  component: () => <PlaceholderPage title="nav.adminLogs" icon="FileText" />,
})
