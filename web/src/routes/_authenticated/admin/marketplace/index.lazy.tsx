import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/admin/marketplace/')({
  component: () => <PlaceholderPage title="nav.adminMarketplace" icon="Store" />,
})
