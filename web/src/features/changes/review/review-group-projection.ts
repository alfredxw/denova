import { lineDiffStats } from '../diff-stats'
import type { ReviewThreadFile, WorkspaceChangeGroup, WorkspaceChangeSet } from '../types'

/** Converts one historical Agent run into the same per-file projection as the cumulative thread view. */
export function projectReviewGroupFiles(group?: WorkspaceChangeGroup): ReviewThreadFile[] {
  if (!group) return []
  const byPath = new Map<string, WorkspaceChangeSet[]>()
  for (const changeSet of group.change_sets ?? []) {
    if (!changeSet.path) continue
    const changes = byPath.get(changeSet.path) ?? []
    changes.push(changeSet)
    byPath.set(changeSet.path, changes)
  }

  return [...byPath.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([path, unsortedChanges]) => projectFile(group, path, unsortedChanges))
}

function projectFile(group: WorkspaceChangeGroup, path: string, unsortedChanges: WorkspaceChangeSet[]): ReviewThreadFile {
  const changes = [...unsortedChanges].sort(compareChangeSets)
  let segmentStart = 0
  let continuity: ReviewThreadFile['continuity'] = 'continuous'
  for (let index = 1; index < changes.length; index += 1) {
    if (!isContinuous(changes[index - 1], changes[index])) {
      continuity = 'conflicted'
      segmentStart = index
    }
  }
  const first = changes[segmentStart]
  const last = changes[changes.length - 1]
  const beforeContent = first.before_content ?? ''
  const afterContent = last.after_content ?? ''
  const stats = lineDiffStats(beforeContent, afterContent)

  return {
    path,
    before_content: beforeContent,
    after_content: afterContent,
    base_revision: first.base_revision ?? '',
    revision: last.revision ?? '',
    base_group_id: first.group_id || group.id,
    base_change_set_id: first.id,
    latest_group_id: last.group_id || group.id,
    latest_change_set_id: last.id,
    group_ids: [group.id],
    change_set_ids: changes.map((changeSet) => changeSet.id),
    pending_edit_ids: changes.flatMap((changeSet) => (changeSet.edits ?? [])
      .filter((edit) => (edit.review_status ?? 'pending') === 'pending')
      .map((edit) => edit.id)),
    review_status: group.review_status,
    apply_state: group.apply_state,
    continuity,
    omitted_iteration_count: segmentStart || undefined,
    before_exists: first.before_exists,
    after_exists: last.after_exists,
    additions: stats.additions,
    deletions: stats.deletions,
  }
}

function compareChangeSets(left: WorkspaceChangeSet, right: WorkspaceChangeSet): number {
  const sequenceDelta = (left.sequence ?? 0) - (right.sequence ?? 0)
  if (sequenceDelta !== 0) return sequenceDelta
  const createdDelta = left.created_at.localeCompare(right.created_at)
  return createdDelta || left.id.localeCompare(right.id)
}

function isContinuous(previous: WorkspaceChangeSet, current: WorkspaceChangeSet): boolean {
  const existenceMatches = previous.after_exists === undefined
    || current.before_exists === undefined
    || previous.after_exists === current.before_exists
  const revisionMatches = !previous.revision || !current.base_revision || previous.revision === current.base_revision
  return existenceMatches && revisionMatches
}
