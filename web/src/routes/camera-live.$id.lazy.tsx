import { createLazyFileRoute } from '@tanstack/react-router'
import { CameraStreamPage } from '@/features/cameras/components/camera-stream-page'

export const Route = createLazyFileRoute('/camera-live/$id')({
  component: CameraStreamPage,
})
