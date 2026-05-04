import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/api-keys')({
  component: () => <PlaceholderPage title="nav.apiKeys" icon="Key" />,
})
