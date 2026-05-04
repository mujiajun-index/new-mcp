import { createLazyFileRoute } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { AlertTriangle } from 'lucide-react'

export const Route = createLazyFileRoute('/500')({
  component: ErrorPage,
})

function ErrorPage() {
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4">
      <AlertTriangle className="h-16 w-16 text-destructive" />
      <h1 className="text-2xl font-semibold">500</h1>
      <p className="text-muted-foreground">服务器内部错误</p>
      <Button variant="outline" onClick={() => window.location.href = '/'}>
        返回首页
      </Button>
    </div>
  )
}
