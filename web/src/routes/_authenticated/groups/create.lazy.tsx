import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/groups/create')({
  component: () => <PlaceholderPage title="nav.groups" subtitle="创建分组" icon="FolderTree" />,
})
