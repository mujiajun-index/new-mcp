import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/groups/')({
  component: () => <PlaceholderPage title="nav.groups" icon="FolderTree" />,
})
