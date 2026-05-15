import { createLazyFileRoute } from '@tanstack/react-router'
import { CameraListPage } from '@/features/cameras/components/camera-list-page'

export const Route = createLazyFileRoute('/_authenticated/cameras/')({
  component: () => <CameraListPage />,
})
