import { createLazyFileRoute } from '@tanstack/react-router'
import { AdminBillingPage } from '@/features/admin/billing/components/admin-billing-page'

export const Route = createLazyFileRoute('/_authenticated/admin/billing')({
  component: AdminBillingPage,
})
