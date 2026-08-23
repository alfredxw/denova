import { describe, expect, it } from 'vitest'
import type { ReviewThreadFile, WorkspaceChangeComment } from '../types'
import { resolveCommentThreads, reviewAnchorFromLineSelection, reviewCommentTarget } from './use-review-diff-annotations'

describe('reviewCommentTarget', () => {
  it('binds cumulative before and after comments to their owning change sets', () => {
    const file = reviewFile()
    expect(reviewCommentTarget(file, 'before')).toEqual({ group_id: 'group-1', change_set_id: 'set-1' })
    expect(reviewCommentTarget(file, 'after')).toEqual({ group_id: 'group-2', change_set_id: 'set-2' })
  })
})

describe('reviewAnchorFromLineSelection', () => {
  it('persists selected lines as UTF-8 byte ranges', () => {
    const file = reviewFile()
    const result = reviewAnchorFromLineSelection(file, { start: 2, end: 3, side: 'additions' })

    expect(result).toMatchObject({
      side: 'after',
      lineNumber: 3,
      anchor: {
        side: 'after',
        encoding: 'utf8-bytes-v1',
        revision: 'after-revision',
        quote: '第二行\nthird',
      },
    })
    expect(result.anchor.start).toBe(new TextEncoder().encode('first\n').length)
    expect(result.anchor.end).toBe(new TextEncoder().encode('first\n第二行\nthird').length)
  })

  it('anchors cross-column selections to the starting side only', () => {
    const file = reviewFile()
    const result = reviewAnchorFromLineSelection(file, { start: 1, end: 3, side: 'deletions', endSide: 'additions' })
    expect(result).toMatchObject({ side: 'before', lineNumber: 1, anchor: { quote: 'old' } })
  })
})

describe('resolveCommentThreads', () => {
  it('relocates an anchor only when its quote is unique', () => {
    const file = reviewFile()
    const relocated: WorkspaceChangeComment = {
      id: 'comment-1',
      group_id: 'group-2',
      change_set_id: 'set-2',
      body: 'Check this',
      anchor: { side: 'after', encoding: 'utf8-bytes-v1', revision: 'old-revision', start: 0, end: 5, quote: 'third' },
    }
    expect(resolveCommentThreads(file, [relocated])).toMatchObject({
      threads: [{ side: 'after', comments: [{ id: 'comment-1' }] }],
      outdated: [],
    })

    const duplicate = { ...file, after_content: 'third\nthird\n' }
    expect(resolveCommentThreads(duplicate, [relocated])).toMatchObject({ threads: [], outdated: [{ id: 'comment-1' }] })
  })
})

function reviewFile(): ReviewThreadFile {
  return {
    path: 'chapters/ch01.md',
    before_content: 'old\n',
    after_content: 'first\n第二行\nthird\n',
    base_revision: 'before-revision',
    revision: 'after-revision',
    base_group_id: 'group-1',
    base_change_set_id: 'set-1',
    latest_group_id: 'group-2',
    latest_change_set_id: 'set-2',
    group_ids: ['group-1', 'group-2'],
    change_set_ids: ['set-1', 'set-2'],
    pending_edit_ids: ['edit-1'],
    review_status: 'pending',
    apply_state: 'applied',
    continuity: 'continuous',
  }
}
