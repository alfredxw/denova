import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useWritingChangeReview } from './use-writing-change-review'

describe('useWritingChangeReview', () => {
  it('opens a temporary review surface after flushing and closes it on file navigation', async () => {
    const beforeOpen = vi.fn().mockResolvedValue(true)
    const { result, rerender } = renderHook(
      ({ selectedFile }) => useWritingChangeReview({
        workspace: '/book',
        contextKey: 'session-1',
        ideActive: true,
        selectedFile,
        agentVisible: true,
        onBeforeOpen: beforeOpen,
        onShowAgent: vi.fn(),
      }),
      { initialProps: { selectedFile: 'chapters/a.md' as string | null } },
    )

    await act(async () => {
      expect(await result.current.openChangeReview('thread-1', 'group-2')).toBe(true)
    })
    expect(beforeOpen).toHaveBeenCalledTimes(1)
    expect(result.current.activeReviewThreadID).toBe('thread-1')
    expect(result.current.activeReviewRequest).toMatchObject({ threadID: 'thread-1', groupID: 'group-2' })

    rerender({ selectedFile: 'chapters/b.md' })
    expect(result.current.activeReviewThreadID).toBe('')
  })

  it('selects unresolved comments for the next Agent turn and opens Agent when hidden', () => {
    const showAgent = vi.fn()
    const { result } = renderHook(() => useWritingChangeReview({
      workspace: '/book',
      contextKey: 'session-1',
      ideActive: true,
      selectedFile: null,
      agentVisible: false,
      onBeforeOpen: () => true,
      onShowAgent: showAgent,
    }))

    act(() => result.current.selectReviewFeedback('thread-1', [
      { id: 'pending', group_id: 'group-1', body: 'revise this' },
      { id: 'resolved', group_id: 'group-1', body: 'done', resolved: true },
    ]))
    expect(result.current.reviewFeedback?.comments.map((comment) => comment.id)).toEqual(['pending'])
    expect(showAgent).toHaveBeenCalledTimes(1)

    act(() => result.current.removeReviewFeedback('pending'))
    expect(result.current.reviewFeedback).toBeNull()

    act(() => result.current.selectReviewFeedback('thread-1', [
      { id: 'pending', group_id: 'group-1', body: 'revise this' },
    ]))
    expect(result.current.reviewFeedback).toBeNull()

    act(() => result.current.selectReviewFeedback('thread-1', [
      { id: 'pending', group_id: 'group-1', body: 'revise this again', updated_at: 'later' },
    ]))
    expect(result.current.reviewFeedback?.comments.map((comment) => comment.id)).toEqual(['pending'])
  })

  it('clears temporary review state when the chat session changes', async () => {
    const { result, rerender } = renderHook(
      ({ contextKey }) => useWritingChangeReview({
        workspace: '/book',
        contextKey,
        ideActive: true,
        selectedFile: null,
        agentVisible: true,
        onBeforeOpen: () => true,
        onShowAgent: vi.fn(),
      }),
      { initialProps: { contextKey: 'session-1' } },
    )
    await act(async () => { await result.current.openChangeReview('thread-1') })
    act(() => result.current.selectReviewFeedback('thread-1', [
      { id: 'pending', group_id: 'group-1', body: 'revise this' },
    ]))

    rerender({ contextKey: 'session-2' })

    expect(result.current.activeReviewThreadID).toBe('')
    expect(result.current.reviewFeedback).toBeNull()
  })

  it('keeps submitted comments suppressed independently across review threads', () => {
    const { result } = renderHook(() => useWritingChangeReview({
      workspace: '/book',
      contextKey: 'session-1',
      ideActive: true,
      selectedFile: null,
      agentVisible: true,
      onBeforeOpen: () => true,
      onShowAgent: vi.fn(),
    }))
    const commentA = { id: 'comment-a', group_id: 'group-a', body: 'A' }
    const commentB = { id: 'comment-b', group_id: 'group-b', body: 'B' }

    act(() => result.current.selectReviewFeedback('thread-a', [commentA]))
    act(() => result.current.submitReviewFeedback({ reviewThreadId: 'thread-a', comments: [commentA] }))
    act(() => result.current.selectReviewFeedback('thread-b', [commentB]))
    expect(result.current.reviewFeedback?.comments.map((comment) => comment.id)).toEqual(['comment-b'])

    act(() => result.current.selectReviewFeedback('thread-a', [commentA]))
    expect(result.current.reviewFeedback).toBeNull()
  })

  it('optimistically consumes submitted comments and restores them when submission fails', () => {
    const { result } = renderHook(() => useWritingChangeReview({
      workspace: '/book',
      contextKey: 'session-1',
      ideActive: true,
      selectedFile: null,
      agentVisible: true,
      onBeforeOpen: () => true,
      onShowAgent: vi.fn(),
    }))
    const comment = { id: 'comment-1', group_id: 'group-1', body: 'revise this' }
    const feedback = { reviewThreadId: 'thread-1', comments: [comment] }

    act(() => result.current.selectReviewFeedback('thread-1', [comment]))
    act(() => result.current.submitReviewFeedback(feedback))
    expect(result.current.reviewFeedback).toBeNull()

    act(() => result.current.restoreReviewFeedback(feedback))
    expect(result.current.reviewFeedback?.comments.map((item) => item.id)).toEqual(['comment-1'])
  })
})
