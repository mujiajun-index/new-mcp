import type { CSSProperties } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getMarketplaceTags } from './api'

// useTagColors 标签名→颜色映射(启用标签;未配置颜色的标签不在映射内,渲染时回退 muted 灰)。
export function useTagColors(): Record<string, string> {
  const { data } = useQuery({ queryKey: ['marketplace-tags'], queryFn: getMarketplaceTags })
  const map: Record<string, string> = {}
  for (const tag of (data?.data ?? []) as { name: string; color?: string }[]) {
    if (tag.color) map[tag.name] = tag.color
  }
  return map
}

// tagStyle 配色标签的内联样式:10% 透明度同色打底、原色文字(后端保证 color 为 #RRGGBB)。
export function tagStyle(color?: string): CSSProperties | undefined {
  return color ? { backgroundColor: `${color}1A`, color } : undefined
}
