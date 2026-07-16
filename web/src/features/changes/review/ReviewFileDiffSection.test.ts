import { describe, expect, it } from 'vitest'
import { reviewEditorHeight } from './ReviewFileDiffSection'

describe('reviewEditorHeight', () => {
  it('does not cap a long expanded file at the old 720px viewport height', () => {
    const before = Array.from({ length: 120 }, (_, index) => `before ${index}`).join('\n')
    const after = Array.from({ length: 120 }, (_, index) => `after ${index}`).join('\n')

    expect(reviewEditorHeight({ before_content: before, after_content: after }, 'unified')).toBeGreaterThan(720)
    expect(reviewEditorHeight({ before_content: before, after_content: after }, 'split')).toBeGreaterThan(720)
  })
})
