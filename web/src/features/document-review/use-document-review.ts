import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { ReviewFeedbackSelection } from '@/features/changes/agent/ReviewFeedbackTray'
import { isProjectChangeForProject, type WorkspaceChangeEvent } from '@/features/changes/types'
import { withErrorLogID } from '@/lib/api-client'
import { createDocumentComment, deleteDocumentComment, getDocumentReview, updateDocumentComment } from './api'
import type { CreateDocumentCommentRequest, DocumentReviewComment, DocumentReviewThread } from './types'

interface UseDocumentReviewOptions {
  projectId: string
  agentVisible: boolean
  onShowAgent: () => void
}

const EMPTY_THREAD: DocumentReviewThread = { id: '', comments: [] }
const DOCUMENT_REVIEW_UPDATED_EVENT = 'nova:document-review-updated'
let documentReviewSourceSequence = 0
const projectHiddenCommentIDs = new Map<string, ReadonlySet<string>>()

interface DocumentReviewUpdatedDetail {
  projectId: string
  source: number
  action: 'refresh' | 'submit' | 'restore'
  commentIDs?: string[]
}

/** Owns author-created text-resource comments and their one-shot Agent queue. */
export function useDocumentReview({ projectId, agentVisible, onShowAgent }: UseDocumentReviewOptions) {
  const { t } = useTranslation()
  const [thread, setThread] = useState<DocumentReviewThread>(EMPTY_THREAD)
  const [hiddenCommentIDs, setHiddenCommentIDs] = useState<ReadonlySet<string>>(() => new Set())
  const requestEpochRef = useRef(0)
  const [source] = useState(() => ++documentReviewSourceSequence)
  const updateHiddenCommentIDs = useCallback((update: (current: ReadonlySet<string>) => ReadonlySet<string>) => {
    setHiddenCommentIDs((current) => {
      const next = update(current)
      if (projectId) projectHiddenCommentIDs.set(projectId, next)
      return next
    })
  }, [projectId])

  const refresh = useCallback(async () => {
    const epoch = ++requestEpochRef.current
    if (!projectId) {
      setThread(EMPTY_THREAD)
      return EMPTY_THREAD
    }
    try {
      const next = await getDocumentReview(projectId)
      if (requestEpochRef.current === epoch) {
        setThread(next)
        updateHiddenCommentIDs((current) => new Set(
          [...current].filter((id) => next.comments.some((comment) => comment.id === id)),
        ))
      }
      return next
    } catch (error) {
      console.error('[features/document-review/use-document-review.ts] failed to load document review comments', {
        projectId,
        error,
      })
      if (requestEpochRef.current === epoch) setThread(EMPTY_THREAD)
      return EMPTY_THREAD
    }
  }, [projectId, updateHiddenCommentIDs])

  useEffect(() => {
    setThread(EMPTY_THREAD)
    setHiddenCommentIDs(new Set(projectHiddenCommentIDs.get(projectId) ?? []))
    void refresh()
    return () => { requestEpochRef.current += 1 }
  }, [refresh])

  useEffect(() => {
    const onDocumentReviewUpdated = (event: Event) => {
      const detail = (event as CustomEvent<DocumentReviewUpdatedDetail>).detail
      if (!detail || detail.projectId !== projectId || detail.source === source) return
      if (detail.action === 'submit') {
        updateHiddenCommentIDs((current) => new Set([...current, ...(detail.commentIDs ?? [])]))
        return
      }
      if (detail.action === 'restore') {
        const restored = new Set(detail.commentIDs ?? [])
        updateHiddenCommentIDs((current) => new Set([...current].filter((id) => !restored.has(id))))
        return
      }
      void refresh()
    }
    window.addEventListener(DOCUMENT_REVIEW_UPDATED_EVENT, onDocumentReviewUpdated)
    return () => window.removeEventListener(DOCUMENT_REVIEW_UPDATED_EVENT, onDocumentReviewUpdated)
  }, [projectId, refresh, source, updateHiddenCommentIDs])

  useEffect(() => {
    const onWorkspaceChange = (event: Event) => {
      const detail = (event as CustomEvent<WorkspaceChangeEvent>).detail
      if (detail?.action !== 'review_feedback_consumed' || !isProjectChangeForProject(detail, projectId)) return
      void refresh()
    }
    window.addEventListener('nova:workspace-change', onWorkspaceChange)
    return () => window.removeEventListener('nova:workspace-change', onWorkspaceChange)
  }, [projectId, refresh])

  const addComment = useCallback(async (request: CreateDocumentCommentRequest) => {
    const result = await createDocumentComment(projectId, request)
    setThread(result.reviewThread)
    updateHiddenCommentIDs((current) => {
      const next = new Set(current)
      next.delete(result.comment.id)
      return next
    })
    if (!agentVisible) onShowAgent()
    notifyDocumentReviewUpdated(projectId, source, 'refresh')
    return result.comment
  }, [agentVisible, onShowAgent, projectId, source, updateHiddenCommentIDs])

  const editComment = useCallback(async (comment: DocumentReviewComment, body: string) => {
    const result = await updateDocumentComment(projectId, comment.id, body)
    setThread(result.reviewThread)
    notifyDocumentReviewUpdated(projectId, source, 'refresh')
    return result.comment
  }, [projectId, source])

  const removeComment = useCallback(async (comment: DocumentReviewComment) => {
    const result = await deleteDocumentComment(projectId, comment.id)
    setThread(result.reviewThread)
    updateHiddenCommentIDs((current) => {
      const next = new Set(current)
      next.delete(comment.id)
      return next
    })
    notifyDocumentReviewUpdated(projectId, source, 'refresh')
    return result.comment
  }, [projectId, source, updateHiddenCommentIDs])

  const removeFeedback = useCallback((commentID: string) => {
    const comment = thread.comments.find((item) => item.id === commentID)
    if (!comment) return
    void removeComment(comment).catch((error) => {
      console.error('[features/document-review/use-document-review.ts] failed to delete document review comment', {
        projectId,
        commentID,
        error,
      })
      toast.error(withErrorLogID(t('editor.review.deleteFailed'), error))
    })
  }, [projectId, removeComment, t, thread.comments])

  const feedback = useMemo<ReviewFeedbackSelection | null>(() => {
    if (!thread.id) return null
    const comments = thread.comments.filter((comment) => !hiddenCommentIDs.has(comment.id))
    return comments.length ? { source: 'document', reviewThreadId: thread.id, comments } : null
  }, [hiddenCommentIDs, thread])

  const visibleComments = useMemo(
    () => thread.comments.filter((comment) => !hiddenCommentIDs.has(comment.id)),
    [hiddenCommentIDs, thread.comments],
  )

  const submitFeedback = useCallback((selection: ReviewFeedbackSelection) => {
    if (selection.source !== 'document') return
    const commentIDs = selection.comments.map((comment) => comment.id)
    updateHiddenCommentIDs((current) => new Set([...current, ...commentIDs]))
    notifyDocumentReviewUpdated(projectId, source, 'submit', commentIDs)
  }, [projectId, source, updateHiddenCommentIDs])

  const restoreFeedback = useCallback((selection: ReviewFeedbackSelection) => {
    if (selection.source !== 'document') return
    const restored = new Set(selection.comments.map((comment) => comment.id))
    updateHiddenCommentIDs((current) => new Set([...current].filter((id) => !restored.has(id))))
    notifyDocumentReviewUpdated(projectId, source, 'restore', [...restored])
  }, [projectId, source, updateHiddenCommentIDs])

  return {
    thread,
    visibleComments,
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

function notifyDocumentReviewUpdated(
  projectId: string,
  source: number,
  action: DocumentReviewUpdatedDetail['action'],
  commentIDs?: string[],
) {
  if (!projectId) return
  window.dispatchEvent(new CustomEvent<DocumentReviewUpdatedDetail>(DOCUMENT_REVIEW_UPDATED_EVENT, {
    detail: { projectId, source, action, commentIDs },
  }))
}
