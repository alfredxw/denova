import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

export interface ResourceDirectoryBadge {
  label: string
  title?: string
  tone?: 'default' | 'outline' | 'warning' | 'muted'
}

export interface ResourceDirectoryItem {
  id: string
  title: string
  /** 有 summary 时条目行渲染为双行 */
  summary?: string
  icon?: LucideIcon
  thumbnailUrl?: string | null
  badges?: ResourceDirectoryBadge[]
  /** 置灰展示（如已禁用的条目） */
  disabled?: boolean
  /** 额外参与默认搜索匹配的文本（默认匹配 title + summary + searchText） */
  searchText?: string
}

export interface ResourceDirectorySection {
  id: string
  label: string
  icon?: LucideIcon
  items: ResourceDirectoryItem[]
  /** 提供时组头展示新建按钮 */
  onCreate?: () => void
  createLabel?: string
  /** 未设置时缺省策略为「空分组折叠」 */
  defaultCollapsed?: boolean
  /** 组头右侧附加内容（计数左侧之外，如 scope 路径、只读徽标） */
  headerMeta?: ReactNode
}

/** 置顶固定条目（如 CREATOR.md、配置 Agent 等伪条目），渲染在搜索区下方 */
export interface ResourceDirectoryPinnedEntry {
  id: string
  label: string
  icon: LucideIcon
}
