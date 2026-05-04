import { AxiosError } from 'axios'
import { toast } from 'sonner'

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
  toast.error('未知错误')
}
