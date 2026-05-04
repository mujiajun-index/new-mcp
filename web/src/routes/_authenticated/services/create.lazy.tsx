import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/services/create')({
  component: () => <PlaceholderPage title="nav.services" subtitle="创建新服务" icon="Server" />,
})
