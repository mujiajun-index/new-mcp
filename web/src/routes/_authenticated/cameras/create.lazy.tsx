import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/cameras/create')({
  component: () => <PlaceholderPage title="nav.cameras" subtitle="添加摄像头" icon="Camera" />,
})
