import { describe, expect, it } from 'vitest'
import type { WorkspaceChangeGroup, WorkspaceChangeSet } from '../types'
import { projectReviewGroupFiles } from './review-group-projection'

describe('projectReviewGroupFiles', () => {
  it('folds each historical run into one cumulative file diff without merging its history boundary', () => {
    const files = projectReviewGroupFiles({
      id: 'group-1',
      review_thread_id: 'thread-1',
      created_at: '2026-07-16T00:00:00Z',
      review_status: 'pending',
      apply_state: 'applied',
      change_sets: [
        changeSet({ id: 'set-1', sequence: 1, before_content: 'zero', after_content: 'one', base_revision: 'r0', revision: 'r1' }),
        changeSet({ id: 'set-2', sequence: 2, before_content: 'one', after_content: 'two', base_revision: 'r1', revision: 'r2' }),
      ],
    } satisfies WorkspaceChangeGroup)

    expect(files).toEqual([
      expect.objectContaining({
        path: 'chapters/ch01.md',
        before_content: 'zero',
        after_content: 'two',
        base_revision: 'r0',
        revision: 'r2',
        base_change_set_id: 'set-1',
        latest_change_set_id: 'set-2',
        change_set_ids: ['set-1', 'set-2'],
        continuity: 'continuous',
      }),
    ])
  })

  it('shows only the latest contiguous segment when a historical run contains a revision gap', () => {
    const files = projectReviewGroupFiles({
      id: 'group-1',
      created_at: '2026-07-16T00:00:00Z',
      review_status: 'pending',
      apply_state: 'applied',
      change_sets: [
        changeSet({ id: 'set-1', sequence: 1, before_content: 'zero', after_content: 'one', base_revision: 'r0', revision: 'r1' }),
        changeSet({ id: 'set-2', sequence: 2, before_content: 'external', after_content: 'two', base_revision: 'external-r', revision: 'r2' }),
      ],
    } satisfies WorkspaceChangeGroup)

    expect(files[0]).toMatchObject({
      before_content: 'external',
      after_content: 'two',
      base_change_set_id: 'set-2',
      continuity: 'conflicted',
      omitted_iteration_count: 1,
    })
  })
})

function changeSet(overrides: Partial<WorkspaceChangeSet>): WorkspaceChangeSet {
  return {
    id: 'set',
    sequence: 1,
    group_id: 'group-1',
    path: 'chapters/ch01.md',
    base_revision: 'before',
    revision: 'after',
    before_content: 'before',
    after_content: 'after',
    before_exists: true,
    after_exists: true,
    edits: [{ id: 'edit-1', review_status: 'pending' }],
    review_status: 'pending',
    apply_state: 'applied',
    created_at: '2026-07-16T00:00:00Z',
    ...overrides,
  }
}
