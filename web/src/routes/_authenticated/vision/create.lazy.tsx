import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/vision/create')({
  component: () => <PlaceholderPage title="nav.vision" subtitle="新建视觉配置" icon="Eye" />,
})
