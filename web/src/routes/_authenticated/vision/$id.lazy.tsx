import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/vision/$id')({
  component: () => <PlaceholderPage title="nav.vision" subtitle="编辑视觉配置" icon="Eye" />,
})
