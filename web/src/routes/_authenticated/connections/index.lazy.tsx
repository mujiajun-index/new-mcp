import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/connections/')({
  component: () => <PlaceholderPage title="nav.connections" icon="Cloud" />,
})
