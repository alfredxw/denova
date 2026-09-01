import { describe, expect, it } from 'vitest'
import type { Snapshot, UpdateBranchPlanResult } from './types'
import { branchPlanSnapshotAfterUpdate } from './branch-plan-snapshot'

const snapshot = {
  story_id: 'story-a',
  branch_id: 'main',
  context_revision: 3,
} as Snapshot

const result: UpdateBranchPlanResult = {
  branch_plan: {
    markdown: '## Direction\n\nCross the river.',
    revision: 'bpr-2',
  },
  context_revision: 4,
}

describe('branchPlanSnapshotAfterUpdate', () => {
  it('patches the still-selected branch', () => {
    expect(branchPlanSnapshotAfterUpdate({
      currentStoryId: 'story-a',
      currentBranchId: 'main',
      snapshot,
      updatedStoryId: 'story-a',
      updatedBranchId: 'main',
      result,
    })).toMatchObject({
      story_id: 'story-a',
      branch_id: 'main',
      branch_plan: result.branch_plan,
      context_revision: 4,
    })
  })

  it('ignores a late save response after branch selection changes', () => {
    expect(branchPlanSnapshotAfterUpdate({
      currentStoryId: 'story-a',
      currentBranchId: 'alternate',
      snapshot,
      updatedStoryId: 'story-a',
      updatedBranchId: 'main',
      result,
    })).toBeNull()
  })
})
