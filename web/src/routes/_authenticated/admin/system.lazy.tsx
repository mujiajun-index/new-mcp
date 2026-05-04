import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/admin/system')({
  component: () => <PlaceholderPage title="nav.adminSystem" icon="Wrench" />,
})
