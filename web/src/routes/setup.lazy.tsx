import { createLazyFileRoute } from '@tanstack/react-router'
import { SetupPage } from '@/features/setup/setup-page'

export const Route = createLazyFileRoute('/setup')({
  component: SetupPage,
})
