import { createLazyFileRoute } from '@tanstack/react-router'
import { SettingsPage } from '@/features/settings/components/settings-page'

export const Route = createLazyFileRoute('/_authenticated/settings')({
  component: SettingsPage,
})
