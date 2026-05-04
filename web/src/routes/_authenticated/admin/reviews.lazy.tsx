import { createLazyFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '@/components/layout/placeholder-page'

export const Route = createLazyFileRoute('/_authenticated/admin/reviews')({
  component: () => <PlaceholderPage title="nav.adminReviews" icon="ClipboardCheck" />,
})
