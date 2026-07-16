import { createLazyFileRoute } from '@tanstack/react-router'
import { RedemptionsPage } from '@/features/redemption-codes/components/redemptions-page'

export const Route = createLazyFileRoute('/_authenticated/admin/redemption-codes')({
  component: RedemptionsPage,
})
