import { createLazyFileRoute } from '@tanstack/react-router'
import { ConnectionDetailPage } from '@/features/connections/components/connection-detail-page'

export const Route = createLazyFileRoute('/_authenticated/connections/$id')({
  component: ConnectionDetailPage,
})
