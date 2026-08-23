import { DiffFileNavigator } from '@/features/diff/DiffFileNavigator'
import { diffFileKind, type DiffFileNavigationItem } from '@/features/diff/types'
import type { ReviewThreadFile } from '../types'

interface ReviewFileNavigatorProps {
  files: ReviewThreadFile[]
  selectedPath: string
  onSelect: (path: string) => void
}

/** Adds Review status metadata to the shared changed-file navigator. */
export function ReviewFileNavigator({ files, ...props }: ReviewFileNavigatorProps) {
  return <DiffFileNavigator files={reviewFileNavigationItems(files)} {...props} />
}

export function reviewFileNavigationItems(files: readonly ReviewThreadFile[]): DiffFileNavigationItem[] {
  return files.map((file) => ({
    path: file.path,
    kind: diffFileKind(file),
    conflicted: file.continuity !== 'continuous' || file.apply_state === 'conflicted',
    accepted: file.review_status === 'accepted',
  }))
}
