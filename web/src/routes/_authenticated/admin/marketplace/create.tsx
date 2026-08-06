import { createFileRoute, redirect } from '@tanstack/react-router'

// 上架通过 /admin/marketplace 列表页内的弹窗完成,此独立路由已废弃:
// 命中即重定向回列表页,避免落入空白占位页。
export const Route = createFileRoute('/_authenticated/admin/marketplace/create')({
  beforeLoad: () => {
    throw redirect({ to: '/admin/marketplace' })
  },
})
