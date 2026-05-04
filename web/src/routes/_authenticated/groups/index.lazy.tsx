import { createLazyFileRoute } from '@tanstack/react-router'
import { GroupListPage } from '@/features/groups/components/group-list-page'

export const Route = createLazyFileRoute('/_authenticated/groups/')({
  component: GroupListPage,
})
