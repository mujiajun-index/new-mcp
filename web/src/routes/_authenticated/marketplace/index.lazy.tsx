import { createLazyFileRoute } from '@tanstack/react-router'
import { MarketplaceListPage } from '@/features/marketplace/components/marketplace-list-page'

export const Route = createLazyFileRoute('/_authenticated/marketplace/')({
  component: MarketplaceListPage,
})
