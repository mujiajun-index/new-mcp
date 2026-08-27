import { createLazyFileRoute } from '@tanstack/react-router'
import { ServiceOverviewPage } from '@/features/services/components/service-overview-page'

export const Route = createLazyFileRoute('/_authenticated/services/overview')({
  component: ServiceOverviewPage,
})
