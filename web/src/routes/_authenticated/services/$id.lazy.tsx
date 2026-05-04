import { createLazyFileRoute } from '@tanstack/react-router'
import { ServiceDetailPage } from '@/features/services/components/service-detail-page'

export const Route = createLazyFileRoute('/_authenticated/services/$id')({
  component: ServiceDetailPage,
})
