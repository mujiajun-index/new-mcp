import { createLazyFileRoute } from '@tanstack/react-router'
import { GroupCreatePage } from '@/features/groups/components/group-create-page'

export const Route = createLazyFileRoute('/_authenticated/groups/create')({
  component: GroupCreatePage,
})
