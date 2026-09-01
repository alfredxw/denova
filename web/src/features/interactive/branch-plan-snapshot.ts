import type { Snapshot, UpdateBranchPlanResult } from './types'

/**
 * Applies a completed plan save only when its story and branch are still
 * selected. This keeps a late response from replacing a newly selected
 * branch's snapshot.
 */
export function branchPlanSnapshotAfterUpdate(input: {
  currentStoryId: string
  currentBranchId: string
  snapshot: Snapshot | null
  updatedStoryId: string
  updatedBranchId: string
  result: UpdateBranchPlanResult
}): Snapshot | null {
  const { currentStoryId, currentBranchId, snapshot, updatedStoryId, updatedBranchId, result } = input
  if (
    currentStoryId !== updatedStoryId
    || currentBranchId !== updatedBranchId
    || snapshot?.story_id !== updatedStoryId
    || snapshot.branch_id !== updatedBranchId
  ) return null
  return {
    ...snapshot,
    branch_plan: result.branch_plan,
    context_revision: result.context_revision,
  }
}
