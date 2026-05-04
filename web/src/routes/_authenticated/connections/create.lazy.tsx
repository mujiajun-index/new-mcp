import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/connections/create')({
  component: () => <PlaceholderPage title="nav.connections" subtitle="添加连接" icon="Cloud" />,
})
