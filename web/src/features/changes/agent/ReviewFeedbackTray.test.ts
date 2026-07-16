import { describe, expect, it } from 'vitest'
import { MAX_REVIEW_FEEDBACK_CONTEXT_BYTES, reviewFeedbackContextBytes } from './ReviewFeedbackTray'

describe('reviewFeedbackContextBytes', () => {
  it('accounts for UTF-8 and rejects payloads beyond the server context budget', () => {
    const bytes = reviewFeedbackContextBytes({
      reviewThreadId: 'thread-1',
      comments: [{
        id: 'comment-1',
        group_id: 'group-1',
        body: '😀'.repeat(70_000),
        review_path: 'chapters/ch01.md',
      }],
    })
    expect(bytes).toBeGreaterThan(MAX_REVIEW_FEEDBACK_CONTEXT_BYTES)
  })
})
