import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/admin/marketplace/create')({
  component: () => <PlaceholderPage title="nav.adminMarketplace" subtitle="上架服务" icon="Store" />,
})
