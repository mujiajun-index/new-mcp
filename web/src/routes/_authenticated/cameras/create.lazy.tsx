import { createLazyFileRoute } from '@tanstack/react-router'
import { CameraCreatePage } from '@/features/cameras/components/camera-create-page'

export const Route = createLazyFileRoute('/_authenticated/cameras/create')({
  component: () => <CameraCreatePage />,
})
