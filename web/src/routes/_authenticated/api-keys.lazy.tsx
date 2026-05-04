import { createLazyFileRoute } from '@tanstack/react-router'
import { ApiKeyPage } from '@/features/api-keys/components/api-key-page'

export const Route = createLazyFileRoute('/_authenticated/api-keys')({
  component: ApiKeyPage,
})
