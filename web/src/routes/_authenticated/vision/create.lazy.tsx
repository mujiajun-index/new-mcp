import { createLazyFileRoute } from '@tanstack/react-router'
import { VisionCreatePage } from '@/features/vision/components/vision-create-page'

export const Route = createLazyFileRoute('/_authenticated/vision/create')({
  component: () => <VisionCreatePage />,
})
