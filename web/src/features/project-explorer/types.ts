import type { FileTreeRowDecoration } from '@pierre/trees'
import type { ReactNode } from 'react'
import type { ProjectFileExplorerNode } from './model'

export interface ProjectExplorerNodeAction {
  id: string
  label: string
  icon?: ReactNode
  disabled?: boolean
  onSelect: () => void
}

export interface ProjectExplorerNodeContext {
  node: ProjectFileExplorerNode
  paths: string[]
}

/** Optional presentation behavior owned by a consuming project surface. */
export interface ProjectExplorerExtensions {
  deleteRecovery?: 'version-history' | 'none'
  getNodeActions?: (context: ProjectExplorerNodeContext) => readonly ProjectExplorerNodeAction[]
  getRowDecoration?: (node: ProjectFileExplorerNode) => FileTreeRowDecoration | null
}
