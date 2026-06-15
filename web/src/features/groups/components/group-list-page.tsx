import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { getGroups } from '../api'
import { Button } from '@/components/ui/button'
import type { GroupListItem } from '@/types'
import { Plus, FolderTree, Wrench, Zap } from 'lucide-react'

export function GroupListPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['groups'],
    queryFn: () => getGroups(),
  })

  const groups: GroupListItem[] = data?.data || []

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">分组管理</h1>
          <p className="mt-1 text-sm text-muted-foreground">将多个服务聚合为统一的 MCP 端点</p>
        </div>
        <Link to="/groups/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            创建分组
          </Button>
        </Link>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-16 text-sm text-muted-foreground">加载中...</div>
      ) : groups.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <FolderTree className="h-10 w-10 text-muted-foreground/30 mb-3" />
          <p className="text-sm text-muted-foreground">暂无分组</p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {groups.map((g) => (
            <Link key={g.id} to="/groups/$id" params={{ id: String(g.id) }}>
              <div className="group rounded-xl border bg-card p-5 transition-all hover:border-primary/20 hover:shadow-md hover:shadow-black/[0.03]">
                <div className="flex items-start justify-between">
                  <div>
                    <h3 className="font-semibold group-hover:text-primary transition-colors">
                      {g.display_name || g.name}
                    </h3>
                    <p className="mt-0.5 text-xs text-muted-foreground font-mono">{g.name}</p>
                  </div>
                  <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
                    g.expose_mode === 'direct' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' : 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'
                  }`}>
                    {g.expose_mode === 'direct' ? '直接' : '智能'}
                  </span>
                </div>
                {g.description && (
                  <p className="mt-2 text-sm text-muted-foreground line-clamp-2">{g.description}</p>
                )}
                <div className="mt-3 flex items-center gap-3 text-xs text-muted-foreground">
                  <span className="flex items-center gap-1"><Wrench className="h-3 w-3" />{g.tools_count} 工具</span>
                  <span className={`inline-flex items-center gap-1 ${g.status === 1 ? 'text-emerald-600' : 'text-zinc-500'}`}>
                    <span className={`h-1.5 w-1.5 rounded-full ${g.status === 1 ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                    {g.status === 1 ? '启用' : '禁用'}
                  </span>
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
