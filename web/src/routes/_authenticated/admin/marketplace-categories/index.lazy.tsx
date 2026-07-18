import { createLazyFileRoute } from '@tanstack/react-router'
import { AdminCategoriesPage } from '@/features/admin/marketplace-categories/components/categories-page'

export const Route = createLazyFileRoute('/_authenticated/admin/marketplace-categories/')({
  component: AdminCategoriesPage,
})
