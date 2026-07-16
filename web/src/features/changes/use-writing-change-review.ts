import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReviewFeedbackSelection } from './agent/ReviewFeedbackTray'
import type { WorkspaceChangeComment } from './types'

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
  const [activeReviewThreadID, setActiveReviewThreadID] = useState('')
  const [reviewFeedback, setReviewFeedback] = useState<ReviewFeedbackSelection | null>(null)
  const selectedFileRef = useRef(selectedFile)
  const suppressedFeedbackRef = useRef(new Map<string, string>())

  useEffect(() => {
    if (selectedFileRef.current !== selectedFile) setActiveReviewThreadID('')
    selectedFileRef.current = selectedFile
  }, [selectedFile])

  useEffect(() => {
    setActiveReviewThreadID('')
    setReviewFeedback(null)
    suppressedFeedbackRef.current.clear()
  }, [contextKey, workspace])

  useEffect(() => {
    if (!ideActive) setActiveReviewThreadID('')
  }, [ideActive])

  const openChangeReview = useCallback(async (reviewThreadID: string) => {
    if (!reviewThreadID || !(await onBeforeOpen())) return false
    setActiveReviewThreadID(reviewThreadID)
    return true
  }, [onBeforeOpen])

  const closeChangeReview = useCallback(() => setActiveReviewThreadID(''), [])

  const selectReviewFeedback = useCallback((reviewThreadID: string, comments: WorkspaceChangeComment[]) => {
    const pending = comments.filter((comment) => (
      !comment.deleted
      && !comment.resolved
      && suppressedFeedbackRef.current.get(feedbackKey(reviewThreadID, comment.id)) !== feedbackVersion(comment)
    ))
    setReviewFeedback(pending.length ? { reviewThreadId: reviewThreadID, comments: pending } : null)
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
  const clearReviewFeedback = useCallback(() => setReviewFeedback((current) => {
    if (!current) return null
    for (const comment of current.comments) {
      suppressedFeedbackRef.current.set(feedbackKey(current.reviewThreadId, comment.id), feedbackVersion(comment))
    }
    return null
  }), [])

  return {
    activeReviewThreadID,
    reviewFeedback,
    openChangeReview,
    closeChangeReview,
    selectReviewFeedback,
    removeReviewFeedback,
    clearReviewFeedback,
  }
}

function feedbackVersion(comment: WorkspaceChangeComment): string {
  return `${comment.updated_at ?? comment.created_at ?? ''}\u0000${comment.body}`
}

function feedbackKey(reviewThreadID: string, commentID: string): string {
  return `${reviewThreadID}\u0000${commentID}`
}
