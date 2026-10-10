import { createLazyFileRoute } from '@tanstack/react-router'
import { ImageGenConfigPage } from '@/features/imagegen/components/imagegen-config-page'
export const Route = createLazyFileRoute('/_authenticated/imagegen/create')({
  component: () => <ImageGenConfigPage />,
})
