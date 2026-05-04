import { createLazyFileRoute } from '@tanstack/react-router'
import { GroupDetailPage } from '@/features/groups/components/group-detail-page'

export const Route = createLazyFileRoute('/_authenticated/groups/$id')({
  component: GroupDetailPage,
})
