import dayjs from 'dayjs'

// datetime-local 不接收时区后缀，必须先转到浏览器当地时间再填入。
export function toDateTimeLocalValue(value: string): string {
  if (!value) return ''
  const date = dayjs(value)
  return date.isValid() ? date.format('YYYY-MM-DDTHH:mm') : ''
}
