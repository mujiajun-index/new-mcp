import * as React from "react"
import { cn } from "@/lib/utils"

// Progress: 纯 CSS 实现(无 radix 依赖),保留 shadcn 的 data-slot 约定——
// 父级可用 `[&_[data-slot=progress-indicator]]:bg-emerald-500` 等覆盖指示条颜色
// (与 reference/new-api/users-columns.tsx 的写法一致)。
const Progress = React.forwardRef<
  HTMLDivElement,
  React.ComponentPropsWithoutRef<"div"> & { value?: number }
>(({ className, value = 0, ...props }, ref) => {
  const pct = Math.min(100, Math.max(0, value))
  return (
    <div
      ref={ref}
      role="progressbar"
      aria-valuenow={pct}
      aria-valuemin={0}
      aria-valuemax={100}
      className={cn(
        "relative h-2 w-full overflow-hidden rounded-full bg-secondary",
        className
      )}
      {...props}
    >
      <div
        data-slot="progress-indicator"
        className="h-full bg-primary transition-all"
        style={{ width: `${pct}%` }}
      />
    </div>
  )
})
Progress.displayName = "Progress"

export { Progress }