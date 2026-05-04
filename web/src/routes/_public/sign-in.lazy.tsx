import { createLazyFileRoute } from '@tanstack/react-router'
import { SignInPage } from '@/features/auth/components/sign-in-page'

export const Route = createLazyFileRoute('/_public/sign-in')({
  component: SignInPage,
})
