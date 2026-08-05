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
  const { result } = renderHook(() => useDocumentReview({ projectId: 'book-1', workspace: '/book', agentVisible: false, onShowAgent: showAgent }))
  await waitFor(() => expect(apiMocks.getDocumentReview).toHaveBeenCalledWith('book-1'))

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

  it('removes consumed comments from both the Agent queue and lore editor for the active workspace', async () => {
    const comment = { id: 'comment-1', thread_id: 'thread-1', target: { kind: 'lore_item' as const, id: 'hero', field: 'content' as const }, body: '修改这里', anchor, created_at: '', updated_at: '' }
    apiMocks.getDocumentReview
      .mockResolvedValueOnce({ id: 'thread-1', comments: [comment] })
      .mockResolvedValueOnce({ id: '', comments: [] })

  const { result } = renderHook(() => useDocumentReview({ projectId: 'book-1', workspace: '/book', agentVisible: true, onShowAgent: vi.fn() }))
    await waitFor(() => expect(result.current.feedback?.comments).toEqual([comment]))
    expect(result.current.visibleComments).toEqual([comment])

    act(() => {
      window.dispatchEvent(new CustomEvent('nova:workspace-change', {
        detail: { workspace: '/another-book', action: 'review_feedback_consumed' },
      }))
    })
    expect(apiMocks.getDocumentReview).toHaveBeenCalledTimes(1)

    act(() => {
      window.dispatchEvent(new CustomEvent('nova:workspace-change', {
        detail: { workspace: '/book', action: 'review_feedback_consumed' },
      }))
    })
    await waitFor(() => expect(result.current.feedback).toBeNull())
    expect(result.current.visibleComments).toEqual([])
    expect(apiMocks.getDocumentReview).toHaveBeenCalledTimes(2)
  })
})
