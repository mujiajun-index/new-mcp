import { AxiosError } from 'axios'
import { toast } from 'sonner'
import i18n from '@/i18n/config'

export function handleServerError(error: unknown) {
  if (error instanceof AxiosError) {
    const message = error.response?.data?.message || error.message
    toast.error(message)
    return
  }
  if (error instanceof Error) {
    toast.error(error.message)
    return
  }
  toast.error(i18n.t('common.unknownError'))
}
