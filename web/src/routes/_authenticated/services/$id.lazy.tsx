import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/services/$id')({
  component: () => <PlaceholderPage title="nav.services" subtitle="服务详情" icon="Server" />,
})
