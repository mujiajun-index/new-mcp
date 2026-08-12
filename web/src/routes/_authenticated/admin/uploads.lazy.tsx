import { createLazyFileRoute } from '@tanstack/react-router'
import { AdminUploadsPage } from '@/features/uploads/components/admin-uploads-page'

export const Route = createLazyFileRoute('/_authenticated/admin/uploads')({
  component: AdminUploadsPage,
})
