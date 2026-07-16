import { describe, expect, it } from 'vitest'
import type { ReviewThreadFile } from '../types'
import { reviewCommentTarget } from './ReviewDiffEditor'

describe('reviewCommentTarget', () => {
  it('binds cumulative before and after comments to their owning change sets', () => {
    const file = {
      base_group_id: 'group-1',
      base_change_set_id: 'set-1',
      latest_group_id: 'group-2',
      latest_change_set_id: 'set-2',
    } as ReviewThreadFile

    expect(reviewCommentTarget(file, 'before')).toEqual({ group_id: 'group-1', change_set_id: 'set-1' })
    expect(reviewCommentTarget(file, 'after')).toEqual({ group_id: 'group-2', change_set_id: 'set-2' })
  })
})
