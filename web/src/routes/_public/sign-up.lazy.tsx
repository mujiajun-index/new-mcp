import { createLazyFileRoute } from '@tanstack/react-router'
import { SignUpPage } from '@/features/auth/components/sign-up-page'

export const Route = createLazyFileRoute('/_public/sign-up')({
  component: SignUpPage,
})
