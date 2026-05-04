import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/vision/')({
  component: () => <PlaceholderPage title="nav.vision" icon="Eye" />,
})
