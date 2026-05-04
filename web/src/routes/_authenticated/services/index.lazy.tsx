import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/services/')({
  component: () => <PlaceholderPage title="nav.services" icon="Server" />,
})
