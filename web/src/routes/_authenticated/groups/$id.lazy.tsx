import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/groups/$id')({
  component: () => <PlaceholderPage title="nav.groups" subtitle="分组详情" icon="FolderTree" />,
})
