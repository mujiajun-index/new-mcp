import { createLazyFileRoute } from '@tanstack/react-router'
import { PricingPage } from '@/features/pricing/components/pricing-page'

export const Route = createLazyFileRoute('/_authenticated/pricing')({
  component: PricingPage,
})
