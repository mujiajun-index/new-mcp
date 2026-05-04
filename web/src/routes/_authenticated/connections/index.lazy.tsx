import { createLazyFileRoute } from '@tanstack/react-router'
import { ConnectionListPage } from '@/features/connections/components/connection-list-page'

export const Route = createLazyFileRoute('/_authenticated/connections/')({
  component: ConnectionListPage,
})
