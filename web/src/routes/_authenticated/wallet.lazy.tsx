import { createLazyFileRoute } from '@tanstack/react-router'
import { WalletPage } from '@/features/wallet/components/wallet-page'

export const Route = createLazyFileRoute('/_authenticated/wallet')({
  component: WalletPage,
})
