import { createLazyFileRoute } from '@tanstack/react-router'
import { CameraDetailPage } from '@/features/cameras/components/camera-detail-page'

export const Route = createLazyFileRoute('/_authenticated/cameras/$id')({
  component: () => <CameraDetailPage />,
})
