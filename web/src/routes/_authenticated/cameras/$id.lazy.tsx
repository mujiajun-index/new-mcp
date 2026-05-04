import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/cameras/$id')({
  component: () => <PlaceholderPage title="nav.cameras" subtitle="摄像头详情" icon="Camera" />,
})
