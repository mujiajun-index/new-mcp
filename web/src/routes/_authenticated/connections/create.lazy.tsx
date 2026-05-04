import { createLazyFileRoute } from '@tanstack/react-router'
import { ConnectionCreatePage } from '@/features/connections/components/connection-create-page'

export const Route = createLazyFileRoute('/_authenticated/connections/create')({
  component: ConnectionCreatePage,
})
