import { createLazyFileRoute } from '@tanstack/react-router'
import { VisionDetailPage } from '@/features/vision/components/vision-detail-page'

export const Route = createLazyFileRoute('/_authenticated/vision/$id')({
  component: () => <VisionDetailPage />,
})
