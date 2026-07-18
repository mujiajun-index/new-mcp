import { createLazyFileRoute } from '@tanstack/react-router'
import { AdminMarketplaceDetailPage } from '@/features/admin/marketplace/components/admin-marketplace-detail-page'

export const Route = createLazyFileRoute('/_authenticated/admin/marketplace/$id')({
  component: AdminMarketplaceDetailPage,
})
