import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReviewFeedbackComment, ReviewFeedbackSelection } from './agent/ReviewFeedbackTray'
import type { WorkspaceChangeComment } from './types'

export interface ChangeReviewScopeRequest {
  id: number
  threadID: string
  groupID: string
}

interface WritingChangeReviewOptions {
  workspace: string
  /** Session or other conversation identity that scopes transient feedback. */
  contextKey: string
  ideActive: boolean
  selectedFile: string | null
  agentVisible: boolean
  onBeforeOpen: () => boolean | Promise<boolean>
  onShowAgent: () => void
}

/** Coordinates the non-persistent Review surface with durable review comments. */
export function useWritingChangeReview({ workspace, contextKey, ideActive, selectedFile, agentVisible, onBeforeOpen, onShowAgent }: WritingChangeReviewOptions) {
  const [activeReviewRequest, setActiveReviewRequest] = useState<ChangeReviewScopeRequest | null>(null)
  const [reviewFeedback, setReviewFeedback] = useState<ReviewFeedbackSelection | null>(null)
  const [submittedReviewCommentIDs, setSubmittedReviewCommentIDs] = useState<ReadonlySet<string>>(() => new Set())
  const selectedFileRef = useRef(selectedFile)
  const suppressedFeedbackRef = useRef(new Map<string, string>())
  const reviewRequestIDRef = useRef(0)
  const activeReviewThreadID = activeReviewRequest?.threadID || ''

  useEffect(() => {
    if (selectedFileRef.current !== selectedFile) setActiveReviewRequest(null)
    selectedFileRef.current = selectedFile
  }, [selectedFile])

  useEffect(() => {
    setActiveReviewRequest(null)
    setReviewFeedback(null)
    setSubmittedReviewCommentIDs(new Set())
    suppressedFeedbackRef.current.clear()
  }, [contextKey, workspace])

  useEffect(() => {
    if (!ideActive) setActiveReviewRequest(null)
  }, [ideActive])

  const openChangeReview = useCallback(async (reviewThreadID: string, groupID = '') => {
    if (!reviewThreadID || !(await onBeforeOpen())) return false
    reviewRequestIDRef.current += 1
    setActiveReviewRequest({ id: reviewRequestIDRef.current, threadID: reviewThreadID, groupID })
    return true
  }, [onBeforeOpen])

  const closeChangeReview = useCallback(() => setActiveReviewRequest(null), [])

  const selectReviewFeedback = useCallback((reviewThreadID: string, comments: WorkspaceChangeComment[]) => {
    const pending = comments.filter((comment) => (
      !comment.deleted
      && !comment.resolved
      && suppressedFeedbackRef.current.get(feedbackKey(reviewThreadID, comment.id)) !== feedbackVersion(comment)
    ))
    setReviewFeedback(pending.length ? { source: 'workspace_change', reviewThreadId: reviewThreadID, comments: pending } : null)
    if (pending.length && !agentVisible) onShowAgent()
  }, [agentVisible, onShowAgent])

  const removeReviewFeedback = useCallback((commentID: string) => {
    setReviewFeedback((current) => {
      if (!current) return null
      const removed = current.comments.find((comment) => comment.id === commentID)
      if (removed) suppressedFeedbackRef.current.set(feedbackKey(current.reviewThreadId, removed.id), feedbackVersion(removed))
      const comments = current.comments.filter((comment) => comment.id !== commentID)
      return comments.length ? { ...current, comments } : null
    })
  }, [])
  const submitReviewFeedback = useCallback((feedback: ReviewFeedbackSelection) => {
    for (const comment of feedback.comments) {
      suppressedFeedbackRef.current.set(feedbackKey(feedback.reviewThreadId, comment.id), feedbackVersion(comment))
    }
    setSubmittedReviewCommentIDs((current) => new Set([...current, ...feedback.comments.map((comment) => comment.id)]))
    setReviewFeedback((current) => current?.reviewThreadId === feedback.reviewThreadId ? null : current)
  }, [])

  const restoreReviewFeedback = useCallback((feedback: ReviewFeedbackSelection) => {
    for (const comment of feedback.comments) {
      const key = feedbackKey(feedback.reviewThreadId, comment.id)
      if (suppressedFeedbackRef.current.get(key) === feedbackVersion(comment)) suppressedFeedbackRef.current.delete(key)
    }
    const restoredIDs = new Set(feedback.comments.map((comment) => comment.id))
    setSubmittedReviewCommentIDs((current) => new Set([...current].filter((id) => !restoredIDs.has(id))))
    setReviewFeedback(feedback.comments.length ? feedback : null)
  }, [])

  return {
    activeReviewThreadID,
    activeReviewRequest,
    reviewFeedback,
    submittedReviewCommentIDs,
    openChangeReview,
    closeChangeReview,
    selectReviewFeedback,
    removeReviewFeedback,
    submitReviewFeedback,
    restoreReviewFeedback,
  }
}

function feedbackVersion(comment: ReviewFeedbackComment): string {
  return `${comment.updated_at ?? comment.created_at ?? ''}\u0000${comment.body}`
}

function feedbackKey(reviewThreadID: string, commentID: string): string {
  return `${reviewThreadID}\u0000${commentID}`
}
