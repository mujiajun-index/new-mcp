import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/cameras/')({
  component: () => <PlaceholderPage title="nav.cameras" icon="Camera" />,
})
