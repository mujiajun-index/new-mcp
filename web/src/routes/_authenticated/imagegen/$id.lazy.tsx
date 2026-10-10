import { createLazyFileRoute } from '@tanstack/react-router'
import { ImageGenConfigPage } from '@/features/imagegen/components/imagegen-config-page'
export const Route = createLazyFileRoute('/_authenticated/imagegen/$id')({
  component: Detail,
})
function Detail() {
  const { id } = Route.useParams()
  return <ImageGenConfigPage id={Number(id)} />
}
