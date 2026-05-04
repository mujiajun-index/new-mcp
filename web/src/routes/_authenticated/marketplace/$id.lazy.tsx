import { createLazyFileRoute } from '@tanstack/react-router'
import { MarketplaceDetailPage } from '@/features/marketplace/components/marketplace-detail-page'

export const Route = createLazyFileRoute('/_authenticated/marketplace/$id')({
  component: MarketplaceDetailPage,
})
