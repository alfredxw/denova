import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import { createDocumentComment, deleteDocumentComment, getDocumentReview, updateDocumentComment } from './api'
import type { CreateDocumentCommentRequest, DocumentReviewComment, DocumentReviewThread } from './types'

interface UseDocumentReviewOptions {
  workspace: string
  agentVisible: boolean
  onShowAgent: () => void
}

const EMPTY_THREAD: DocumentReviewThread = { id: '', comments: [] }

/** Owns author-created document comments and their one-shot Agent queue. */
export function useDocumentReview({ workspace, agentVisible, onShowAgent }: UseDocumentReviewOptions) {
  const { t } = useTranslation()
  const [thread, setThread] = useState<DocumentReviewThread>(EMPTY_THREAD)
  const [hiddenCommentIDs, setHiddenCommentIDs] = useState<ReadonlySet<string>>(() => new Set())
  const requestEpochRef = useRef(0)

  const refresh = useCallback(async () => {
    const epoch = ++requestEpochRef.current
    if (!workspace) {
      setThread(EMPTY_THREAD)
      return EMPTY_THREAD
    }
    try {
      const next = await getDocumentReview(workspace)
      if (requestEpochRef.current === epoch) {
        setThread(next)
        setHiddenCommentIDs((current) => new Set([...current].filter((id) => next.comments.some((comment) => comment.id === id))))
      }
      return next
    } catch (error) {
      console.error('加载正文审阅评论失败', { workspace, error })
      if (requestEpochRef.current === epoch) setThread(EMPTY_THREAD)
      return EMPTY_THREAD
    }
  }, [workspace])

  useEffect(() => {
    setThread(EMPTY_THREAD)
    setHiddenCommentIDs(new Set())
    void refresh()
    return () => { requestEpochRef.current += 1 }
  }, [refresh])

  useEffect(() => {
    const onWorkspaceChange = (event: Event) => {
      const detail = (event as CustomEvent<{ action?: string }>).detail
      if (detail?.action === 'review_feedback_consumed') void refresh()
    }
    window.addEventListener('nova:workspace-change', onWorkspaceChange)
    return () => window.removeEventListener('nova:workspace-change', onWorkspaceChange)
  }, [refresh])

  const addComment = useCallback(async (request: CreateDocumentCommentRequest) => {
    const result = await createDocumentComment(workspace, request)
    setThread(result.reviewThread)
    setHiddenCommentIDs((current) => {
      const next = new Set(current)
      next.delete(result.comment.id)
      return next
    })
    if (!agentVisible) onShowAgent()
    return result.comment
  }, [agentVisible, onShowAgent, workspace])

  const editComment = useCallback(async (comment: DocumentReviewComment, body: string) => {
    const result = await updateDocumentComment(workspace, comment.id, body)
    setThread(result.reviewThread)
    return result.comment
  }, [workspace])

  const removeComment = useCallback(async (comment: DocumentReviewComment) => {
    const result = await deleteDocumentComment(workspace, comment.id)
    setThread(result.reviewThread)
    setHiddenCommentIDs((current) => {
      const next = new Set(current)
      next.delete(comment.id)
      return next
    })
    return result.comment
  }, [workspace])

  const removeFeedback = useCallback((commentID: string) => {
    const comment = thread.comments.find((item) => item.id === commentID)
    if (!comment) return
    void removeComment(comment).catch((error) => {
      console.error('删除正文审阅评论失败', { workspace, commentID, error })
      toast.error(t('editor.review.deleteFailed'))
    })
  }, [removeComment, t, thread.comments, workspace])

  const feedback = useMemo<ReviewFeedbackSelection | null>(() => {
    if (!thread.id) return null
    const comments = thread.comments.filter((comment) => !hiddenCommentIDs.has(comment.id))
    return comments.length ? { source: 'document', reviewThreadId: thread.id, comments } : null
  }, [hiddenCommentIDs, thread])

  const submitFeedback = useCallback((selection: ReviewFeedbackSelection) => {
    if (selection.source !== 'document') return
    setHiddenCommentIDs((current) => new Set([...current, ...selection.comments.map((comment) => comment.id)]))
  }, [])

  const restoreFeedback = useCallback((selection: ReviewFeedbackSelection) => {
    if (selection.source !== 'document') return
    const restored = new Set(selection.comments.map((comment) => comment.id))
    setHiddenCommentIDs((current) => new Set([...current].filter((id) => !restored.has(id))))
  }, [])

  return {
    thread,
    feedback,
    refresh,
    addComment,
    editComment,
    removeComment,
    removeFeedback,
    submitFeedback,
    restoreFeedback,
  }
}
