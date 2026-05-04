import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/admin/marketplace/$id')({
  component: () => <PlaceholderPage title="nav.adminMarketplace" subtitle="编辑服务" icon="Store" />,
})
