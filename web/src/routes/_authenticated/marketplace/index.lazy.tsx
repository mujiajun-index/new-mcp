import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/marketplace/')({
  component: () => <PlaceholderPage title="nav.marketplace" icon="Store" />,
})
