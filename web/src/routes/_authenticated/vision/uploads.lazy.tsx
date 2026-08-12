import { createLazyFileRoute } from '@tanstack/react-router'
import { UploadsPage } from '@/features/uploads/components/uploads-page'

export const Route = createLazyFileRoute('/_authenticated/vision/uploads')({
  component: UploadsPage,
})
