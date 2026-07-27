import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDocumentReview } from './use-document-review'

const apiMocks = vi.hoisted(() => ({
  getDocumentReview: vi.fn(),
  createDocumentComment: vi.fn(),
  updateDocumentComment: vi.fn(),
  deleteDocumentComment: vi.fn(),
}))

vi.mock('./api', () => apiMocks)

const anchor = {
  kind: 'text-range' as const,
  encoding: 'utf8-bytes-v1' as const,
  revision: 'sha256:body',
  start: 0,
  end: 6,
  quote: '正文',
  display_quote: '正文',
}

describe('useDocumentReview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getDocumentReview.mockResolvedValue({ id: '', comments: [] })
  })

  it('hides a submitted lore comment from both the Agent queue and editor, then restores failed submissions', async () => {
    const comment = { id: 'comment-1', thread_id: 'thread-1', target: { kind: 'lore_item' as const, id: 'hero', field: 'content' as const }, body: '修改这里', anchor, created_at: '', updated_at: '' }
    const thread = { id: 'thread-1', comments: [comment] }
    apiMocks.createDocumentComment.mockResolvedValue({ workspace: '/book', reviewThread: thread, comment })
    const showAgent = vi.fn()
    const { result } = renderHook(() => useDocumentReview({ workspace: '/book', agentVisible: false, onShowAgent: showAgent }))
    await waitFor(() => expect(apiMocks.getDocumentReview).toHaveBeenCalledWith('/book'))

    await act(async () => {
      await result.current.addComment({ target: comment.target, body: comment.body, anchor })
    })
    expect(showAgent).toHaveBeenCalledTimes(1)
    expect(result.current.feedback).toMatchObject({ source: 'document', reviewThreadId: 'thread-1', comments: [{ id: 'comment-1' }] })
    expect(result.current.visibleComments).toEqual([comment])

    const selection = result.current.feedback!
    act(() => result.current.submitFeedback(selection))
    expect(result.current.feedback).toBeNull()
    expect(result.current.visibleComments).toEqual([])
    expect(result.current.thread.comments).toEqual([comment])
    act(() => result.current.restoreFeedback(selection))
    expect(result.current.feedback?.comments[0].id).toBe('comment-1')
    expect(result.current.visibleComments).toEqual([comment])
  })
})
