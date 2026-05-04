import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/settings')({
  component: () => <PlaceholderPage title="nav.settings" icon="Settings" />,
})
