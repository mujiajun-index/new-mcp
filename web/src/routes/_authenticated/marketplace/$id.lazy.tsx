import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/marketplace/$id')({
  component: () => <PlaceholderPage title="nav.marketplace" subtitle="市场服务详情" icon="Store" />,
})
