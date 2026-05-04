import { createLazyFileRoute } from '@tanstack/react-router'
import { ServiceCreatePage } from '@/features/services/components/service-create-page'

export const Route = createLazyFileRoute('/_authenticated/services/create')({
  component: ServiceCreatePage,
})
