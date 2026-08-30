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
  /** Optional state dot over the leading icon; label is exposed to assistive technology. */
  status?: {
    label: string
    tone?: 'default' | 'success' | 'warning' | 'danger' | 'muted'
  }
}

export interface ResourceDirectorySection {
  id: string
  label: string
  /** Optional full context shown when a compact section label is truncated. */
  description?: string
  icon?: LucideIcon
  items: ResourceDirectoryItem[]
  /** 提供时组头展示新建按钮 */
  onCreate?: () => void
  createLabel?: string
  /** 未设置时缺省策略为「空分组折叠」 */
  defaultCollapsed?: boolean
  /** 组头右侧附加内容（计数左侧之外，如 scope 路径、只读徽标） */
  headerMeta?: ReactNode
  /** 允许条目在当前分组内拖拽排序；需配合 ResourceDirectory.onReorderItems。 */
  reorderable?: boolean
}

/** Pinned fixed entries such as CREATOR.md, rendered below the search area. */
export interface ResourceDirectoryPinnedEntry {
  id: string
  label: string
  icon: LucideIcon
  summary?: string
}
