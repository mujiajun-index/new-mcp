import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/connections/$id')({
  component: () => <PlaceholderPage title="nav.connections" subtitle="连接详情" icon="Cloud" />,
})
