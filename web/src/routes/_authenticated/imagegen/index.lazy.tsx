import { createLazyFileRoute } from '@tanstack/react-router'
import { ImageGenListPage } from '@/features/imagegen/components/imagegen-list-page'
export const Route = createLazyFileRoute('/_authenticated/imagegen/')({
  component: ImageGenListPage,
})
