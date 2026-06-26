import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/admin/marketplace/create')({
  component: () => <PlaceholderPage title="nav.adminMarketplace" subtitle="admin.marketplace.subtitleCreate" icon="Store" />,
})
