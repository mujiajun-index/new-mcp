import { createLazyFileRoute } from '@tanstack/react-router'
import { VisionListPage } from '@/features/vision/components/vision-list-page'

export const Route = createLazyFileRoute('/_authenticated/vision/')({
  component: () => <VisionListPage />,
})
