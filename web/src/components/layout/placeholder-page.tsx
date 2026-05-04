import { useTranslation } from 'react-i18next'
import { useMatchRoute } from '@tanstack/react-router'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  Server, FolderTree, Cloud, Eye, Camera, Key, Store, Settings,
  Users, FileText, ClipboardCheck, Wrench, type LucideIcon,
} from 'lucide-react'

const iconMap: Record<string, LucideIcon> = {
  Server, FolderTree, Cloud, Eye, Camera, Key, Store, Settings,
  Users, FileText, ClipboardCheck, Wrench,
}

export function PlaceholderPage({ title, subtitle, icon }: { title: string; subtitle?: string; icon: string }) {
  const { t } = useTranslation()
  const Icon = iconMap[icon] || Server

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-8">
      <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-muted">
        <Icon className="h-8 w-8 text-muted-foreground" />
      </div>
      <h2 className="text-xl font-semibold">{t(title)}</h2>
      {subtitle && <p className="text-sm text-muted-foreground">{subtitle}</p>}
      <p className="text-xs text-muted-foreground/60 mt-2">即将上线</p>
    </div>
  )
}
