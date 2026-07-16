import { createLazyFileRoute } from '@tanstack/react-router'
import { AdminMarketplacePage } from '@/features/admin/marketplace/components/admin-marketplace-page'

export const Route = createLazyFileRoute('/_authenticated/admin/marketplace/')({
  component: AdminMarketplacePage,
})
