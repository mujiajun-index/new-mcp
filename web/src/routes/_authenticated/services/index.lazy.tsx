import { createLazyFileRoute } from '@tanstack/react-router'
import { ServiceListPage } from '@/features/services/components/service-list-page'

export const Route = createLazyFileRoute('/_authenticated/services/')({
  component: ServiceListPage,
})
